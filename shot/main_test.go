package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestPathIsChecked(t *testing.T) {
	s := &shooter{target: "http://example.invalid"}
	for _, p := range []string{"", "/s/demo", "/s/../x/", "/s/Demo/", "/s/a b/", "http://evil/s/x/", "/s/x/y/", "/s/-x/"} {
		rec := httptest.NewRecorder()
		s.handle(rec, httptest.NewRequest("GET", "/shot?path="+url.QueryEscape(p), nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: %d", p, rec.Code)
		}
	}
}

func TestCoverSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, viewW, viewH))
	src.Set(10, 10, color.Black)
	var buf bytes.Buffer
	png.Encode(&buf, src)
	out, err := cover(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil || img.Bounds().Dx() != coverW || img.Bounds().Dy() != coverH {
		t.Fatalf("cover %v, %v", img.Bounds(), err)
	}
}

// Needs Chrome: runs in the image (CHROME_PATH is set there).
func TestShoot(t *testing.T) {
	path := os.Getenv("CHROME_PATH")
	if path == "" {
		t.Skip("CHROME_PATH not set")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!doctype html><meta charset="utf-8"><body style="margin:0;background:#1f6a52;color:#fff;font:64px sans-serif"><h1>静态项目 ✓</h1>`))
	}))
	defer site.Close()
	s := &shooter{target: site.URL, execPath: path}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := s.shoot(ctx, site.URL+"/s/demo/")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil || img.Bounds().Dx() != coverW {
		t.Fatalf("shot %v, %v", img.Bounds(), err)
	}
	// The page's green background fills the corner.
	r, g, b, _ := img.At(790, 490).RGBA()
	if g>>8 < 90 || r>>8 > 60 || b>>8 > 110 {
		t.Fatalf("corner colour = %d,%d,%d", r>>8, g>>8, b>>8)
	}
	if dest := os.Getenv("SHOT_OUT"); dest != "" {
		os.WriteFile(dest, out, 0o644)
	}
}
