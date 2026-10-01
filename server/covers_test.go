package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoversAreTakenAndServed(t *testing.T) {
	var shots atomic.Int32
	shot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shot" || r.URL.Query().Get("path") != "/s/demo/" {
			t.Errorf("shot request %s", r.URL)
		}
		shots.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("\xff\xd8\xff fake jpeg"))
	}))
	defer shot.Close()

	app := newTestApp(t)
	covers, err := NewCovers(t.TempDir(), shot.URL)
	if err != nil {
		t.Fatal(err)
	}
	app.covers = covers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go covers.Run(ctx, app.store.Exists)

	if res := performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false); res.Code != http.StatusCreated {
		t.Fatalf("upload: %d", res.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for covers.URL("demo") == "" || covers.Pending("demo") {
		if time.Now().After(deadline) {
			t.Fatal("cover was not taken")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if shots.Load() != 1 {
		t.Fatalf("shots = %d", shots.Load())
	}

	req := httptest.NewRequest("GET", "https://site.test/api/sites", nil)
	req.Host = "site.test"
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	var sites []SiteResponse
	json.Unmarshal(res.Body.Bytes(), &sites)
	if len(sites) != 1 || !strings.HasPrefix(sites[0].Cover, "/covers/demo.jpg?v=") {
		t.Fatalf("sites = %+v", sites)
	}
	req = httptest.NewRequest("GET", "https://site.test"+sites[0].Cover, nil)
	req.Host = "site.test"
	res = httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != 200 || !strings.Contains(res.Header().Get("Cache-Control"), "immutable") || !strings.Contains(res.Body.String(), "fake jpeg") {
		t.Fatalf("cover: %d %q", res.Code, res.Header().Get("Cache-Control"))
	}
	for _, bad := range []string{"/covers/../meta.json", "/covers/Demo.jpg", "/covers/demo.png", "/covers/missing.jpg"} {
		req := httptest.NewRequest("GET", "https://site.test"+bad, nil)
		req.Host = "site.test"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		// The router answers "../" with a redirect to the cleaned path before
		// the cover handler is reached.
		if res.Code != 404 && res.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s: %d", bad, res.Code)
		}
	}

	// Deleting the project drops its cover.
	del := httptest.NewRequest("DELETE", "https://site.test/api/sites/demo", nil)
	del.Host = "site.test"
	del.Header.Set("Authorization", "Bearer "+testToken)
	app.routes().ServeHTTP(httptest.NewRecorder(), del)
	if _, err := os.Stat(covers.path("demo")); !os.IsNotExist(err) {
		t.Fatalf("cover left after delete: %v", err)
	}
}

func TestCoversOffWithoutAShotService(t *testing.T) {
	app := newTestApp(t)
	performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false)
	if app.covers.Pending("demo") || app.covers.URL("demo") != "" {
		t.Fatal("no shot service: nothing should be queued")
	}
	req := httptest.NewRequest("POST", "https://site.test/api/sites/demo/cover", nil)
	req.Host = "site.test"
	req.Header.Set("Authorization", "Bearer "+testToken)
	res := httptest.NewRecorder()
	app.routes().ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("retake without a shot service: %d", res.Code)
	}
}

func TestInternalHostServesProjectsWithoutCountingThem(t *testing.T) {
	app := newTestApp(t)
	app.cfg.InternalHost = "static-deployer"
	performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false)
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/s/demo/", 200},
		{"/api/sites", 404},
		{"/", 404},
	} {
		req := httptest.NewRequest("GET", "http://static-deployer:8080"+tc.path, nil)
		req.Host = "static-deployer:8080"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Errorf("%s: %d want %d", tc.path, res.Code, tc.want)
		}
	}
	if v := app.stats.Get("demo").Views; v != 0 {
		t.Fatalf("screenshot visits were counted: %d", v)
	}
}

func TestFailedCoversAreRetried(t *testing.T) {
	var calls atomic.Int32
	shot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "still starting", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("\xff\xd8\xff second try"))
	}))
	defer shot.Close()
	app := newTestApp(t)
	covers, _ := NewCovers(t.TempDir(), shot.URL)
	covers.retry = []time.Duration{20 * time.Millisecond}
	app.covers = covers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go covers.Run(ctx, app.store.Exists)
	performUpload(t, app, "page.html", []byte("<h1>demo</h1>"), "demo", false)
	deadline := time.Now().Add(3 * time.Second)
	for covers.URL("demo") == "" {
		if time.Now().After(deadline) {
			t.Fatalf("cover not taken after a retry (calls=%d)", calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}
