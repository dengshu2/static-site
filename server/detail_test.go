package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func call(app *App, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://site.test"+path, nil)
	req.Host = "site.test"
	req.Header.Set("Authorization", "Bearer "+testToken)
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	return res
}

func TestSiteDetailAndRestoringAnEarlierVersion(t *testing.T) {
	app := newTestApp(t)
	performUpload(t, app, "page.html", []byte("v1"), "demo", false)
	if res := performUpload(t, app, "page.html", []byte("v2"), "demo", true); res.Code != http.StatusCreated {
		t.Fatalf("overwrite: %d %s", res.Code, res.Body.String())
	}

	res := call(app, "GET", "/api/sites/demo")
	var d SiteDetail
	if err := json.Unmarshal(res.Body.Bytes(), &d); err != nil || res.Code != 200 {
		t.Fatalf("detail: %d %s", res.Code, res.Body.String())
	}
	if len(d.Daily) != 30 || d.Site.Name != "demo" || d.Site.Files != 1 || len(d.Versions) != 1 || d.Versions[0].Reason != "overwritten" {
		t.Fatalf("detail = %+v", d)
	}
	if res := call(app, "GET", "/api/sites/missing"); res.Code != 404 {
		t.Fatalf("missing detail: %d", res.Code)
	}

	// Restoring v1 without replace conflicts; with replace it swaps the versions.
	if res := call(app, "POST", "/api/trash/"+d.Versions[0].ID+"/restore"); res.Code != http.StatusConflict {
		t.Fatalf("restore over a live site: %d", res.Code)
	}
	if res := call(app, "POST", "/api/trash/"+d.Versions[0].ID+"/restore?replace=1"); res.Code != 200 {
		t.Fatalf("restore with replace: %d %s", res.Code, res.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(app.sitesDir, "demo", "index.html")); string(b) != "v1" {
		t.Fatalf("live content = %q, want v1", b)
	}
	res = call(app, "GET", "/api/sites/demo")
	json.Unmarshal(res.Body.Bytes(), &d)
	if len(d.Versions) != 1 || d.Versions[0].Reason != "overwritten" {
		t.Fatalf("v2 should now be in the history: %+v", d.Versions)
	}
}

func TestCatalogHidesFileDetails(t *testing.T) {
	app := newTestApp(t)
	app.cfg.AdminHost = "deploy.test"
	app.cfg.AdminBaseURL = "https://deploy.test"
	performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false)

	public := httptest.NewRequest("GET", "https://site.test/api/sites", nil)
	public.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, public)
	if strings.Contains(res.Body.String(), `"files"`) || strings.Contains(res.Body.String(), `"size"`) {
		t.Fatalf("catalog shows file details: %s", res.Body.String())
	}

	admin := httptest.NewRequest("GET", "https://deploy.test/api/sites", nil)
	admin.Host = "deploy.test"
	res = httptest.NewRecorder()
	app.routes().ServeHTTP(res, admin)
	if !strings.Contains(res.Body.String(), `"files":1`) {
		t.Fatalf("admin list lost file details: %s", res.Body.String())
	}
}

func TestRestoredVersionIsNotHiddenByBrowserCaches(t *testing.T) {
	app := newTestApp(t)
	performUpload(t, app, "page.html", []byte("v1"), "demo", false)
	time.Sleep(1100 * time.Millisecond) // Last-Modified has one-second resolution
	performUpload(t, app, "page.html", []byte("v2"), "demo", true)

	get := func(ims string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://site.test/s/demo/", nil)
		req.Host = "site.test"
		if ims != "" {
			req.Header.Set("If-Modified-Since", ims)
		}
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		return res
	}
	cached := get("").Header().Get("Last-Modified") // a browser now holds v2

	var d SiteDetail
	json.Unmarshal(call(app, "GET", "/api/sites/demo").Body.Bytes(), &d)
	time.Sleep(1100 * time.Millisecond)
	if res := call(app, "POST", "/api/trash/"+d.Versions[0].ID+"/restore?replace=1"); res.Code != 200 {
		t.Fatalf("restore: %d", res.Code)
	}
	res := get(cached)
	if res.Code != http.StatusOK || res.Body.String() != "v1" {
		t.Fatalf("after restoring v1, a browser holding v2 got %d %q", res.Code, res.Body.String())
	}
}
