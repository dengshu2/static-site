// Command shot takes the screenshots used as project covers in the catalog.
//
// It runs next to the deployer on a private network and opens projects from
// the deployer's internal host, so it needs no secrets and reaches nothing
// else of the deployer. One headless Chrome stays running; each screenshot
// gets its own tab, one at a time.
//
//	GET /shot?path=/s/<name>/   an 800×500 JPEG of the project's first screen
//	GET /healthz
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"golang.org/x/image/draw"
)

const (
	viewW, viewH   = 1280, 800
	coverW, coverH = 800, 500
	settle         = 1500 * time.Millisecond // after load, for fonts, scripts and animations
)

var projectPath = regexp.MustCompile(`^/s/[a-z0-9]+(?:-[a-z0-9]+)*/$`)

type shooter struct {
	target   string // the deployer's internal base URL
	execPath string

	mu      sync.Mutex // one screenshot at a time
	browser context.Context
	stop    context.CancelFunc
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "health" {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://127.0.0.1" + env("LISTEN", ":9000") + "/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	s := &shooter{target: env("TARGET", "http://static-deployer:8080"), execPath: os.Getenv("CHROME_PATH")}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /shot", s.handle)
	srv := &http.Server{Addr: env("LISTEN", ":9000"), Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: time.Minute}
	log.Printf("shot listening on %s, target %s", srv.Addr, s.target)
	log.Fatal(srv.ListenAndServe())
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func (s *shooter) handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if !projectPath.MatchString(path) {
		http.Error(w, "path must be /s/<name>/", http.StatusBadRequest)
		return
	}
	start := time.Now()
	img, err := s.shoot(r.Context(), s.target+path)
	if err != nil {
		log.Printf("shot %s failed: %v", path, err)
		http.Error(w, "screenshot failed", http.StatusBadGateway)
		return
	}
	log.Printf("shot %s in %s (%d bytes)", path, time.Since(start).Round(10*time.Millisecond), len(img))
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(img)
}

// browserCtx starts Chrome on first use and again if it has died.
func (s *shooter) browserCtx() (context.Context, error) {
	if s.browser != nil && s.browser.Err() == nil {
		return s.browser, nil
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox, // the container is the sandbox
		chromedp.DisableGPU,
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if s.execPath != "" {
		opts = append(opts, chromedp.ExecPath(s.execPath))
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browser, cancelBrowser := chromedp.NewContext(alloc)
	if err := chromedp.Run(browser); err != nil {
		cancelBrowser()
		cancelAlloc()
		return nil, fmt.Errorf("start chrome: %w", err)
	}
	s.browser, s.stop = browser, func() { cancelBrowser(); cancelAlloc() }
	return browser, nil
}

func (s *shooter) shoot(ctx context.Context, url string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	browser, err := s.browserCtx()
	if err != nil {
		return nil, err
	}
	tab, closeTab := chromedp.NewContext(browser)
	defer closeTab()
	tab, cancel := context.WithTimeout(tab, 30*time.Second)
	defer cancel()
	go func() { // a caller giving up closes the tab too
		select {
		case <-ctx.Done():
			cancel()
		case <-tab.Done():
		}
	}()

	var (
		shot       []byte
		fontsReady bool
	)
	err = chromedp.Run(tab,
		// Always the current version: a restored older version has older file
		// times, which a cache would answer with the page it already holds.
		network.SetCacheDisabled(true),
		emulation.SetDeviceMetricsOverride(viewW, viewH, 1, false),
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "light"}}),
		chromedp.Navigate(url),
		// Wait for fonts, then a moment for scripts and entrance animations.
		chromedp.Evaluate(`document.fonts.ready.then(() => true)`, &fontsReady, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
		chromedp.Sleep(settle),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			shot, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return cover(shot)
}

// cover scales the 1280×800 screen to the 800×500 cover.
func cover(pngData []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, coverW, coverH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
