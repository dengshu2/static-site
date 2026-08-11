package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webEmbed embed.FS

type App struct {
	cfg       Config
	store     *Store
	stats     *StatsStore
	trash     *Trash
	sitesDir  string
	uploadDir string
	mutateMu  sync.Mutex
	limiter   *failureLimiter
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		runHealthcheck()
		return
	}

	cfg := loadConfig()
	sitesDir := filepath.Join(cfg.DataDir, "sites")
	uploadDir := filepath.Join(cfg.DataDir, "tmp")
	for _, dir := range []string{sitesDir, uploadDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Fatalf("无法创建数据目录: %v", err)
		}
	}
	cleanupStale(sitesDir, ".stage-", 24*time.Hour)
	cleanupStale(uploadDir, "upload-", 24*time.Hour)

	store, err := NewStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("初始化站点存储失败: %v", err)
	}
	stats, err := NewStatsStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("初始化访问统计失败: %v", err)
	}
	trash, err := NewTrash(cfg.DataDir, cfg.TrashRetention)
	if err != nil {
		log.Fatalf("初始化回收站失败: %v", err)
	}
	app := &App{
		cfg:       cfg,
		store:     store,
		stats:     stats,
		trash:     trash,
		sitesDir:  sitesDir,
		uploadDir: uploadDir,
		limiter:   newFailureLimiter(20, 10*time.Minute),
	}
	janitorCtx, stopJanitor := context.WithCancel(context.Background())
	defer stopJanitor()
	go app.runTrashJanitor(janitorCtx)

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf(
			"deployer 启动: listen=%s data=%s public=%s admin=%s content=%s",
			cfg.Listen, cfg.DataDir, cfg.PublicHost, cfg.AdminHost, cfg.ContentHost,
		)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Printf("收到信号 %s，开始优雅退出", sig)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("优雅退出失败: %v", err)
	}
}

func (a *App) runTrashJanitor(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mutateMu.Lock()
			err := a.trash.Cleanup()
			a.mutateMu.Unlock()
			if err != nil {
				log.Printf("event=trash_cleanup_failed error=%q", err)
			}
		}
	}
}

func (a *App) routes() http.Handler {
	handlers := make(map[string]http.Handler)
	for _, host := range []string{a.cfg.PublicHost, a.cfg.AdminHost, a.cfg.ContentHost} {
		if _, exists := handlers[host]; exists {
			continue
		}
		handlers[host] = a.routesFor(
			host == a.cfg.PublicHost,
			host == a.cfg.AdminHost,
			host == a.cfg.ContentHost,
		)
	}
	local := a.routesFor(true, true, true)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := requestHost(r.Host)
		if isLocalHost(host) {
			local.ServeHTTP(w, r)
			return
		}
		if handler, ok := handlers[host]; ok {
			handler.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusMisdirectedRequest, errBody("请求域名未配置"))
	})
}

func (a *App) routesFor(public, admin, content bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if public || admin {
		mux.HandleFunc("GET /api/sites", a.handleListSites)
		mux.HandleFunc("GET /api/analytics", a.handleAnalytics)
		mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{
				"publicURL":  a.cfg.PublicBaseURL,
				"adminURL":   a.cfg.AdminBaseURL,
				"contentURL": a.cfg.ContentBaseURL,
			})
		})
	}
	if admin {
		mux.Handle("POST /api/upload", a.auth(http.HandlerFunc(a.handleUpload)))
		mux.Handle("PATCH /api/sites/{name}", a.auth(http.HandlerFunc(a.handleUpdateSite)))
		mux.Handle("DELETE /api/sites/{name}", a.auth(http.HandlerFunc(a.handleDeleteSite)))
		mux.Handle("GET /api/trash", a.auth(http.HandlerFunc(a.handleListTrash)))
		mux.Handle("POST /api/trash/{id}/restore", a.auth(http.HandlerFunc(a.handleRestoreTrash)))
		mux.Handle("DELETE /api/trash/{id}", a.auth(http.HandlerFunc(a.handlePurgeTrash)))
	}
	if content {
		contentFS := indexOnlyFS{http.Dir(a.sitesDir)}
		contentHandler := http.StripPrefix("/s/", http.FileServer(contentFS))
		mux.Handle("GET /s/", contentHeaders(a.trackContentViews(contentHandler)))
	}

	publicUI := embeddedUI("web/public")
	adminUI := embeddedUI("web/admin")
	if public {
		if admin {
			mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/admin/", http.StatusPermanentRedirect)
			})
			mux.Handle("GET /admin/", http.StripPrefix("/admin/", uiHeaders(noCache(adminUI))))
		}
		mux.Handle("GET /", uiHeaders(noCache(publicUI)))
	} else if admin {
		mux.Handle("GET /", uiHeaders(noCache(adminUI)))
	}
	return mux
}

func embeddedUI(root string) http.Handler {
	sub, err := fs.Sub(webEmbed, root)
	if err != nil {
		panic(err)
	}
	shared, err := fs.Sub(webEmbed, "web/shared")
	if err != nil {
		panic(err)
	}
	return http.FileServer(sharedFS{primary: http.FS(sub), fallback: http.FS(shared)})
}

// noCache 让页面和脚本每次重新验证；字体内容与文件名一一对应，
// 长期缓存可以避免每次访问重新下载并闪一次字体。更换字体必须改文件名。
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}

func uiHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func contentHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := contentViewSite(r.URL.Path); ok {
			// HTML 每次重新验证，既避免覆盖部署后出现旧页面，也让访问统计覆盖正常浏览。
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (a *App) trackContentViews(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, track := contentViewSite(r.URL.Path)
		if !track || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		recorder := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if recorder.status != http.StatusOK && recorder.status != http.StatusNotModified {
			return
		}
		if _, exists := a.store.Get(name); !exists {
			return
		}
		if err := a.stats.RecordView(name, time.Now()); err != nil {
			log.Printf("event=view_stat_failed site=%q error=%q", name, err)
		}
	})
}

func contentViewSite(path string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/s/"), "/")
	if len(parts) == 0 || parts[0] == "" || parts[0] != slugify(parts[0]) {
		return "", false
	}
	relative := strings.Join(parts[1:], "/")
	if relative == "" || strings.HasSuffix(path, "/") {
		return parts[0], true
	}
	lower := strings.ToLower(relative)
	return parts[0], strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm")
}

func (a *App) auth(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(a.cfg.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(raw, "Bearer ")
		got := sha256.Sum256([]byte(token))
		ip := clientIP(r)
		if !ok || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			log.Printf("event=auth_failed ip=%q path=%q", ip, r.URL.Path)
			if !a.limiter.RecordFailure(ip) {
				w.Header().Set("Retry-After", "600")
				writeJSON(w, http.StatusTooManyRequests, errBody("鉴权失败次数过多，请稍后再试"))
				return
			}
			writeJSON(w, http.StatusUnauthorized, errBody("未授权：请提供正确的 Token"))
			return
		}
		a.limiter.Reset(ip)
		next.ServeHTTP(w, r)
	})
}

func requestHost(raw string) string {
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(strings.TrimSuffix(raw, "."))
}

func isLocalHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func cleanupStale(dir, prefix string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}
}

func runHealthcheck() {
	url := os.Getenv("HEALTHCHECK_URL")
	if url == "" {
		url = "http://127.0.0.1:8080/healthz"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		os.Exit(1)
	}
	_ = resp.Body.Close()
}
