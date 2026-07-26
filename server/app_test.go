package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef"

func newTestApp(t *testing.T) *App {
	t.Helper()
	dataDir := t.TempDir()
	sitesDir := filepath.Join(dataDir, "sites")
	uploadDir := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := NewStatsStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	trash, err := NewTrash(dataDir, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &App{
		cfg: Config{
			Token:          testToken,
			PublicHost:     "site.test",
			AdminHost:      "site.test",
			ContentHost:    "site.test",
			PublicBaseURL:  "https://site.test",
			AdminBaseURL:   "https://site.test/admin",
			ContentBaseURL: "https://site.test",
			MaxUploadMB:    20,
			MaxUnzipMB:     50,
			MaxTotalMB:     200,
			MaxZipFiles:    100,
			MaxPathDepth:   10,
			MaxSiteNameLen: 63,
			MaxSites:       20,
			TrashRetention: 7 * 24 * time.Hour,
		},
		store:     store,
		stats:     stats,
		trash:     trash,
		sitesDir:  sitesDir,
		uploadDir: uploadDir,
		limiter:   newFailureLimiter(20, 10*time.Minute),
	}
}

func TestProjectMetadataCanBeUpdatedWithoutLeakingOrigin(t *testing.T) {
	app := newTestApp(t)
	if res := performUpload(t, app, "private-name.html", []byte("<h1>demo</h1>"), "demo", false); res.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", res.Code, res.Body.String())
	}
	body := strings.NewReader(`{"title":"数据小站","description":"面向团队的可视化结果。"} `)
	req := httptest.NewRequest(http.MethodPatch, "https://site.test/api/sites/demo", body)
	req.Host = "site.test"
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "数据小站") || strings.Contains(res.Body.String(), "private-name") {
		t.Fatalf("unexpected metadata response: %s", res.Body.String())
	}
	site, ok := app.store.Get("demo")
	if !ok || site.Title != "数据小站" || site.Description != "面向团队的可视化结果。" {
		t.Fatalf("metadata not saved: %#v", site)
	}
}

func TestSuccessfulDocumentViewsAreCountedButAssetsAndMissingPagesAreNot(t *testing.T) {
	app := newTestApp(t)
	if res := performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false); res.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", res.Code, res.Body.String())
	}
	if err := os.WriteFile(filepath.Join(app.sitesDir, "demo", "about.html"), []byte("<p>about</p>"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, requestPath := range []string{"/s/demo/", "/s/demo/about.html", "/s/demo/index.html", "/s/demo/missing.html", "/s/demo/app.js"} {
		req := httptest.NewRequest(http.MethodGet, "https://site.test"+requestPath, nil)
		req.Host = "site.test"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
	}
	stats := app.stats.Get("demo")
	if stats.Views != 2 {
		t.Fatalf("views=%d want=2", stats.Views)
	}
	if stats.LastViewedAt == nil || len(stats.Daily) != 1 {
		t.Fatalf("view details missing: %#v", stats)
	}

	req := httptest.NewRequest(http.MethodGet, "https://site.test/api/analytics", nil)
	req.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"totalViews":2`) {
		t.Fatalf("analytics status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestPublicCatalogNeedsNoTokenAndOmitsOrigin(t *testing.T) {
	app := newTestApp(t)
	siteDir := filepath.Join(app.sitesDir, "demo")
	if err := os.MkdirAll(siteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>demo</h1>"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := app.store.Save(Site{
		Name: "demo", URL: "/s/demo/", Origin: "private-filename.html",
		Files: 1, Size: 13, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://site.test/api/sites", nil)
	req.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "private-filename") || strings.Contains(res.Body.String(), "origin") {
		t.Fatalf("public response leaked origin: %s", res.Body.String())
	}
	var sites []SiteResponse
	if err := json.Unmarshal(res.Body.Bytes(), &sites); err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].URL != "https://site.test/s/demo/" {
		t.Fatalf("unexpected sites: %#v", sites)
	}
}

func TestDirectoryListingDisabledAndSiteServed(t *testing.T) {
	app := newTestApp(t)
	siteDir := filepath.Join(app.sitesDir, "demo")
	if err := os.MkdirAll(siteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>demo</h1>"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/s/", http.StatusNotFound},
		{"/s/demo/", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "https://site.test"+tc.path, nil)
		req.Host = "site.test"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Fatalf("%s: status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
	}
}

func TestMutationRequiresTokenAndRateLimitsFailures(t *testing.T) {
	app := newTestApp(t)
	for i := 1; i <= 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "https://site.test/api/upload", strings.NewReader("bad"))
		req.Host = "site.test"
		req.RemoteAddr = "192.0.2.5:1234"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		want := http.StatusUnauthorized
		if i == 21 {
			want = http.StatusTooManyRequests
		}
		if res.Code != want {
			t.Fatalf("attempt %d: status=%d want=%d", i, res.Code, want)
		}
	}
}

func TestLargeHTMLUploadStreamsWithoutSystemTmp(t *testing.T) {
	app := newTestApp(t)
	payload := bytes.Repeat([]byte("a"), 17<<20)
	res := performUpload(t, app, "large.html", payload, "large", false)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	info, err := os.Stat(filepath.Join(app.sitesDir, "large", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(payload)) {
		t.Fatalf("size=%d want=%d", info.Size(), len(payload))
	}
}

func TestConcurrentSameNameDoesNotCorrupt(t *testing.T) {
	app := newTestApp(t)
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, text := range []string{"first", "second"} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			res := performUpload(t, app, "page.html", []byte(body), "same-name", false)
			statuses <- res.Code
		}(text)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("unexpected statuses: %#v", counts)
	}
	data, err := os.ReadFile(filepath.Join(app.sitesDir, "same-name", "index.html"))
	if err != nil || (string(data) != "first" && string(data) != "second") {
		t.Fatalf("published data corrupted: %q err=%v", data, err)
	}
}

func TestDeleteMovesToTrashAndRestoreWorks(t *testing.T) {
	app := newTestApp(t)
	if res := performUpload(t, app, "page.html", []byte("version"), "recoverable", false); res.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", res.Code, res.Body.String())
	}
	req := httptest.NewRequest(http.MethodDelete, "https://site.test/api/sites/recoverable", nil)
	req.Host = "site.test"
	req.Header.Set("Authorization", "Bearer "+testToken)
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", res.Code, res.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app.sitesDir, "recoverable")); !os.IsNotExist(err) {
		t.Fatalf("site still present after delete: %v", err)
	}

	restore := httptest.NewRequest(http.MethodPost, "https://site.test/api/trash/"+body["trashId"]+"/restore", nil)
	restore.Host = "site.test"
	restore.Header.Set("Authorization", "Bearer "+testToken)
	restoreRes := httptest.NewRecorder()
	app.routes().ServeHTTP(restoreRes, restore)
	if restoreRes.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restoreRes.Code, restoreRes.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.sitesDir, "recoverable", "index.html")); err != nil {
		t.Fatal(err)
	}
}

func TestAdminScriptDoesNotPersistTokenOrUseInnerHTML(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "https://site.test/admin/js/sites.js", nil)
	req.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	body := res.Body.String()
	for _, forbidden := range []string{"localStorage", "sessionStorage", "innerHTML"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("admin script contains %q", forbidden)
		}
	}
}

func TestSeparateHostsExposeOnlyTheirOwnSurface(t *testing.T) {
	app := newTestApp(t)
	app.cfg.AdminHost = "deploy.test"
	app.cfg.ContentHost = "pages.test"
	app.cfg.AdminBaseURL = "https://deploy.test"
	app.cfg.ContentBaseURL = "https://pages.test"
	siteDir := filepath.Join(app.sitesDir, "demo")
	if err := os.MkdirAll(siteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("demo"), 0o640); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		host   string
		method string
		path   string
		want   int
	}{
		{"site.test", http.MethodGet, "/", http.StatusOK},
		{"site.test", http.MethodGet, "/admin/", http.StatusNotFound},
		{"site.test", http.MethodPost, "/api/upload", http.StatusMethodNotAllowed},
		{"deploy.test", http.MethodGet, "/", http.StatusOK},
		{"pages.test", http.MethodGet, "/s/demo/", http.StatusOK},
		{"pages.test", http.MethodGet, "/api/sites", http.StatusNotFound},
		{"unknown.test", http.MethodGet, "/", http.StatusMisdirectedRequest},
	}
	for _, tc := range tests {
		t.Run(tc.host+tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
			req.Host = tc.host
			res := httptest.NewRecorder()
			app.routes().ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", res.Code, tc.want, res.Body.String())
			}
		})
	}
}

func TestUIHasSecurityHeaders(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "https://site.test/", nil)
	req.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	for _, header := range []string{
		"Content-Security-Policy",
		"Cross-Origin-Opener-Policy",
		"Permissions-Policy",
		"Referrer-Policy",
		"X-Content-Type-Options",
		"X-Frame-Options",
	} {
		if res.Header().Get(header) == "" {
			t.Errorf("missing %s", header)
		}
	}
}

func performUpload(t *testing.T, app *App, filename string, data []byte, name string, overwrite bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", name); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("overwrite", map[bool]string{true: "true", false: "false"}[overwrite]); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://site.test/api/upload", &body)
	req.Host = "site.test"
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	return res
}
