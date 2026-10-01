package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Covers keeps one screenshot per project for the catalog. The screenshots
// come from a separate service (see /shot) that loads the project from this
// server's internal content host; the deployer itself stays a small binary
// without a browser. Covers are best effort: a project without one is shown
// with a plain placeholder.
type Covers struct {
	dir     string
	shotURL string // empty: covers are off
	client  *http.Client

	mu       sync.Mutex
	pending  map[string]bool
	attempts map[string]int
	queue    chan string
	retry    []time.Duration // waits before trying a failed screenshot again
}

const maxCoverBytes = 4 << 20

func NewCovers(dataDir, shotURL string) (*Covers, error) {
	dir := filepath.Join(dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Covers{
		dir:      dir,
		shotURL:  shotURL,
		client:   &http.Client{Timeout: 45 * time.Second},
		pending:  map[string]bool{},
		attempts: map[string]int{},
		queue:    make(chan string, 1024),
		retry:    []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute},
	}, nil
}

func (c *Covers) path(name string) string { return filepath.Join(c.dir, name+".jpg") }

// URL is the cover's address with a version that changes with the image, or
// "" when there is none.
func (c *Covers) URL(name string) string {
	info, err := os.Stat(c.path(name))
	if err != nil {
		return ""
	}
	return "/covers/" + name + ".jpg?v=" + strconv.FormatInt(info.ModTime().UnixNano(), 36)
}

// Pending reports whether a screenshot is queued or being taken.
func (c *Covers) Pending(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending[name]
}

// Request queues a new screenshot (again, if one exists).
func (c *Covers) Request(name string) {
	if c.shotURL == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[name] {
		return
	}
	select {
	case c.queue <- name:
		c.pending[name] = true
	default:
		log.Printf("event=cover_queue_full site=%q", name)
	}
}

// Backfill queues every project that has no cover yet and has not run out
// of attempts recently.
func (c *Covers) Backfill(sites []Site) {
	c.mu.Lock()
	c.attempts = map[string]int{} // a new round: everything gets its tries again
	c.mu.Unlock()
	for _, site := range sites {
		if _, err := os.Stat(c.path(site.Name)); os.IsNotExist(err) {
			c.Request(site.Name)
		}
	}
}

func (c *Covers) Remove(name string) {
	_ = os.Remove(c.path(name))
}

// Run takes the queued screenshots one at a time until ctx ends. A failed
// one (the service still starting, a page that would not load) is tried
// again a few times, waiting longer each time.
func (c *Covers) Run(ctx context.Context, exists func(string) bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case name := <-c.queue:
			var err error
			if exists(name) {
				err = c.take(ctx, name)
			}
			c.mu.Lock()
			delete(c.pending, name)
			n := c.attempts[name]
			if err == nil {
				delete(c.attempts, name)
			} else {
				c.attempts[name] = n + 1
			}
			c.mu.Unlock()
			switch {
			case err == nil:
				if exists(name) {
					log.Printf("event=cover_taken site=%q", name)
				}
			case n < len(c.retry):
				log.Printf("event=cover_failed site=%q retry_in=%s error=%q", name, c.retry[n], err)
				time.AfterFunc(c.retry[n], func() { c.Request(name) })
			default:
				log.Printf("event=cover_given_up site=%q error=%q", name, err)
			}
		}
	}
}

func (c *Covers) take(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	endpoint := c.shotURL + "/shot?path=" + url.QueryEscape("/s/"+name+"/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("shot service: %s %s", resp.Status, body)
	}
	if resp.Header.Get("Content-Type") != "image/jpeg" {
		return errors.New("shot service did not return a JPEG")
	}
	tmp, err := os.CreateTemp(c.dir, ".cover-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxCoverBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxCoverBytes || n == 0 {
		return fmt.Errorf("cover has an unexpected size (%d bytes)", n)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path(name))
}

// serveCover handles GET /covers/<name>.jpg on the catalog and admin hosts.
func (a *App) serveCover(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	name, ok := cutSuffix(file, ".jpg")
	if !ok || name == "" || name != slugify(name) || len(name) > a.cfg.MaxSiteNameLen {
		http.NotFound(w, r)
		return
	}
	path := a.covers.path(name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	// The URL carries ?v=<image version>, so the image can be cached for good.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func (a *App) handleRetakeCover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !a.store.Exists(name) {
		writeJSON(w, http.StatusNotFound, errBody("站点不存在"))
		return
	}
	if a.covers.shotURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, errBody("没有配置截图服务"))
		return
	}
	a.covers.Request(name)
	writeJSON(w, http.StatusAccepted, map[string]bool{"pending": true})
}

func cutSuffix(s, suffix string) (string, bool) {
	if len(s) < len(suffix) || s[len(s)-len(suffix):] != suffix {
		return s, false
	}
	return s[:len(s)-len(suffix)], true
}
