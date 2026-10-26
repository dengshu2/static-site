package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestModelFilesServedWithModelTypes(t *testing.T) {
	app := newTestApp(t)
	siteDir := filepath.Join(app.sitesDir, "demo")
	if err := os.MkdirAll(siteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// 内容以 ZIP 魔数开头，确认不是靠嗅探得到的类型
	for _, name := range []string{"cake.usdz", "cake.glb"} {
		if err := os.WriteFile(filepath.Join(siteDir, name), []byte("PK\x03\x04model"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ path, want string }{
		{"/s/demo/cake.usdz", "model/vnd.usdz+zip"},
		{"/s/demo/cake.glb", "model/gltf-binary"},
	} {
		req := httptest.NewRequest(http.MethodGet, "https://site.test"+tc.path, nil)
		req.Host = "site.test"
		res := httptest.NewRecorder()
		app.routes().ServeHTTP(res, req)
		if res.Code != http.StatusOK || res.Header().Get("Content-Type") != tc.want {
			t.Fatalf("%s: status=%d type=%q", tc.path, res.Code, res.Header().Get("Content-Type"))
		}
	}
}
