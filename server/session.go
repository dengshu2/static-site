package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 管理端登录：输入一次 Token，换成 30 天的会话 Cookie。
//
// Cookie 只有管理端域名能读写（HttpOnly、host-only，HTTPS 下带 __Host- 前缀），
// 上传内容在另一个域名，脚本拿不到它。但 deploy 与 pages 属于同一个站点
// （同一个注册域名），SameSite 挡不住从上传页面发起的请求，所以凭 Cookie
// 发起的写操作还必须带上管理端自己的 Origin。Bearer Token 照旧可用（脚本、CI）。
//
// Cookie 内容是"过期时间 + HMAC"，密钥由 DEPLOY_TOKEN 派生：服务重启不会
// 退出登录，更换 Token 会让所有会话立即失效。

const sessionLifetime = 30 * 24 * time.Hour

func (a *App) sessionCookieName() string {
	if strings.HasPrefix(a.cfg.AdminBaseURL, "https://") {
		return "__Host-dd_session"
	}
	return "dd_session"
}

func (a *App) sessionKey() []byte {
	mac := hmac.New(sha256.New, []byte(a.cfg.Token))
	mac.Write([]byte("drop-and-deploy session v1"))
	return mac.Sum(nil)
}

func (a *App) signSession(expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, a.sessionKey())
	mac.Write([]byte(exp))
	return exp + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) validSession(value string, now time.Time) bool {
	exp, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || now.Unix() >= unix {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, a.sessionKey())
	mac.Write([]byte(exp))
	return hmac.Equal(got, mac.Sum(nil))
}

func (a *App) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.sessionCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.cfg.AdminBaseURL, "https://"),
		SameSite: http.SameSiteStrictMode,
	})
}

// sameOrigin reports whether a request was sent by the admin page itself.
func (a *App) sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	want, err := url.Parse(a.cfg.AdminBaseURL)
	if err != nil {
		return false
	}
	return origin == want.Scheme+"://"+want.Host
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !a.sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, errBody("请从管理页面登录"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("请求格式无效"))
		return
	}
	want := sha256.Sum256([]byte(a.cfg.Token))
	got := sha256.Sum256([]byte(input.Token))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		log.Printf("event=login_failed ip=%q", ip)
		if !a.limiter.RecordFailure(ip) {
			w.Header().Set("Retry-After", "600")
			writeJSON(w, http.StatusTooManyRequests, errBody("尝试次数过多，请 10 分钟后再试"))
			return
		}
		writeJSON(w, http.StatusUnauthorized, errBody("Token 不正确"))
		return
	}
	a.limiter.Reset(ip)
	a.setSessionCookie(w, a.signSession(time.Now().Add(sessionLifetime)), int(sessionLifetime.Seconds()))
	log.Printf("event=login ip=%q", ip)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// handleSession answers whether the browser is signed in.
func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(a.sessionCookieName())
	writeJSON(w, http.StatusOK, map[string]bool{"signedIn": err == nil && a.validSession(c.Value, time.Now())})
}

// auth accepts the Bearer token, or a session cookie on a request the admin
// page sent itself.
func (a *App) auth(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(a.cfg.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			got := sha256.Sum256([]byte(raw))
			if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
				a.limiter.Reset(ip)
				next.ServeHTTP(w, r)
				return
			}
		} else if c, err := r.Cookie(a.sessionCookieName()); err == nil && a.validSession(c.Value, time.Now()) {
			if r.Method != http.MethodGet && !a.sameOrigin(r) {
				log.Printf("event=cross_origin_blocked ip=%q path=%q origin=%q", ip, r.URL.Path, r.Header.Get("Origin"))
				writeJSON(w, http.StatusForbidden, errBody("请求来源不是管理页面"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		log.Printf("event=auth_failed ip=%q path=%q", ip, r.URL.Path)
		if !a.limiter.RecordFailure(ip) {
			w.Header().Set("Retry-After", "600")
			writeJSON(w, http.StatusTooManyRequests, errBody("鉴权失败次数过多，请稍后再试"))
			return
		}
		writeJSON(w, http.StatusUnauthorized, errBody("请先登录"))
	})
}
