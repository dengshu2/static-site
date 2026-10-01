package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func adminApp(t *testing.T) *App {
	app := newTestApp(t)
	app.cfg.AdminHost = "deploy.test"
	app.cfg.ContentHost = "pages.test"
	app.cfg.AdminBaseURL = "https://deploy.test"
	app.cfg.ContentBaseURL = "https://pages.test"
	return app
}

func send(app *App, method, path, body string, headers map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://deploy.test"+path, strings.NewReader(body))
	req.Host = "deploy.test"
	req.RemoteAddr = "192.0.2.9:1234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	return res
}

var fromAdmin = map[string]string{"Origin": "https://deploy.test", "Content-Type": "application/json"}

func TestLoginGivesAThirtyDayCookie(t *testing.T) {
	app := adminApp(t)
	if res := send(app, "POST", "/api/session", `{"token":"wrong-token"}`, fromAdmin); res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", res.Code)
	}
	if res := send(app, "POST", "/api/session", `{"token":"`+testToken+`"}`, map[string]string{"Origin": "https://pages.test"}); res.Code != http.StatusForbidden {
		t.Fatalf("login from an uploaded page: %d", res.Code)
	}
	res := send(app, "POST", "/api/session", `{"token":"`+testToken+`"}`, fromAdmin)
	if res.Code != http.StatusNoContent {
		t.Fatalf("login: %d %s", res.Code, res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if c.Name != "__Host-dd_session" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || c.MaxAge < 29*24*3600 {
		t.Fatalf("cookie = %+v", c)
	}
	if res := send(app, "GET", "/api/session", "", nil, c); !strings.Contains(res.Body.String(), `"signedIn":true`) {
		t.Fatalf("session check: %d %s", res.Code, res.Body.String())
	}
	if res := send(app, "GET", "/api/trash", "", nil, c); res.Code != http.StatusOK {
		t.Fatalf("read with the cookie: %d", res.Code)
	}
	if res := send(app, "GET", "/api/session", "", nil); res.Code != 200 || !strings.Contains(res.Body.String(), `"signedIn":false`) {
		t.Fatalf("no cookie: %d %s", res.Code, res.Body.String())
	}

	out := send(app, "DELETE", "/api/session", "", nil, c)
	if out.Code != http.StatusNoContent || out.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout should clear the cookie: %d %v", out.Code, out.Result().Cookies())
	}
}

func TestCookieWritesMustComeFromTheAdminPage(t *testing.T) {
	app := adminApp(t)
	if res := performUpload(t, app, "page.html", []byte("<h1>x</h1>"), "demo", false); res.Code != http.StatusCreated {
		t.Fatalf("upload: %d", res.Code)
	}
	cookie := &http.Cookie{Name: "__Host-dd_session", Value: app.signSession(time.Now().Add(time.Hour))}
	body := `{"title":"新标题","description":""}`

	// An uploaded page on pages.test is same-site with deploy.test, so the
	// browser would send the cookie along: the Origin check must stop it.
	for _, h := range []map[string]string{
		{"Origin": "https://pages.test"},
		{"Sec-Fetch-Site": "same-site"},
		{},
	} {
		if res := send(app, "PATCH", "/api/sites/demo", body, h, cookie); res.Code != http.StatusForbidden {
			t.Fatalf("PATCH with %v: %d", h, res.Code)
		}
	}
	if res := send(app, "PATCH", "/api/sites/demo", body, fromAdmin, cookie); res.Code != http.StatusOK {
		t.Fatalf("PATCH from the admin page: %d %s", res.Code, res.Body.String())
	}
	if res := send(app, "PATCH", "/api/sites/demo", body, map[string]string{"Sec-Fetch-Site": "same-origin"}, cookie); res.Code != http.StatusOK {
		t.Fatalf("PATCH with Sec-Fetch-Site same-origin: %d", res.Code)
	}

	for name, value := range map[string]string{
		"expired":  app.signSession(time.Now().Add(-time.Minute)),
		"tampered": strings.Replace(app.signSession(time.Now().Add(time.Hour)), ".", "9.", 1),
		"garbage":  "nope",
	} {
		c := &http.Cookie{Name: "__Host-dd_session", Value: value}
		if res := send(app, "PATCH", "/api/sites/demo", body, fromAdmin, c); res.Code != http.StatusUnauthorized {
			t.Fatalf("%s cookie: %d", name, res.Code)
		}
	}

	// Changing DEPLOY_TOKEN signs everyone out.
	app.cfg.Token = "another-token-0123456789"
	if res := send(app, "PATCH", "/api/sites/demo", body, fromAdmin, cookie); res.Code != http.StatusUnauthorized {
		t.Fatalf("cookie after a token change: %d", res.Code)
	}
}
