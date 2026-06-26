package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// 前端落地页打进二进制，部署时无需额外挂载 web/。
//
//go:embed web
var webEmbed embed.FS

// App 持有运行期依赖：配置 + 站点存储。
type App struct {
	cfg   Config
	store *Store
}

func main() {
	cfg := loadConfig()

	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "sites"), 0o755); err != nil {
		log.Fatalf("无法创建数据目录: %v", err)
	}

	store, err := NewStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("初始化站点存储失败: %v", err)
	}
	app := &App{cfg: cfg, store: store}

	mux := http.NewServeMux()

	// ── 管理 API（需 Token）──────────────────────────────────────────
	mux.Handle("POST /api/upload", app.auth(http.HandlerFunc(app.handleUpload)))
	mux.Handle("GET /api/sites", app.auth(http.HandlerFunc(app.handleListSites)))
	mux.Handle("DELETE /api/sites/{name}", app.auth(http.HandlerFunc(app.handleDeleteSite)))

	// ── 已部署站点（公开只读）────────────────────────────────────────
	sitesDir := filepath.Join(cfg.DataDir, "sites")
	mux.Handle("GET /s/", http.StripPrefix("/s/", http.FileServer(http.Dir(sitesDir))))

	// ── 前端落地页（公开）────────────────────────────────────────────
	// 管理 UI 必须实时反映最新版本，禁用缓存避免浏览器/CDN 留旧页面。
	webRoot, _ := fs.Sub(webEmbed, "web")
	mux.Handle("GET /", noCache(http.FileServerFS(webRoot)))

	log.Printf("deployer 启动: listen=%s data=%s", cfg.Listen, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Fatal(err)
	}
}

// noCache 让管理 UI 资源（HTML/CSS/JS）始终重新校验，避免刷新仍看到旧页面。
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// auth 是 Bearer Token 中间件，仅保护 /api/*。
func (a *App) auth(next http.Handler) http.Handler {
	want := "Bearer " + a.cfg.Token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权：请提供正确的 Token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
