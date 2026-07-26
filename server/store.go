package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Site 是一个已部署站点的元数据。
type Site struct {
	Name        string    `json:"name"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url"`
	Origin      string    `json:"origin"` // 上传时的原始文件名
	Files       int       `json:"files"`  // 站点内文件数
	Size        int64     `json:"size"`   // 站点总字节数
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// Store 管理站点目录与 meta.json，单进程内用一把锁串行化所有写操作。
type Store struct {
	mu       sync.Mutex
	sitesDir string
	metaPath string
	meta     map[string]Site
}

func NewStore(dataDir string) (*Store, error) {
	s := &Store{
		sitesDir: filepath.Join(dataDir, "sites"),
		metaPath: filepath.Join(dataDir, "meta.json"),
		meta:     map[string]Site{},
	}
	if err := os.MkdirAll(s.sitesDir, 0o750); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(s.metaPath); err == nil {
		if err := json.Unmarshal(b, &s.meta); err != nil {
			return nil, fmt.Errorf("元数据损坏，拒绝静默启动: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := s.reconcile(); err != nil {
		return nil, err
	}
	return s, nil
}

// List 按创建时间倒序返回所有站点。
func (s *Store) List() []Site {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Site, 0, len(s.meta))
	for _, v := range s.meta {
		out = append(out, normalizeSite(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) Exists(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.meta[name]
	return ok
}

func (s *Store) Get(name string) (Site, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.meta[name]
	return normalizeSite(v), ok
}

// Save 记录一条站点元数据并落盘。
func (s *Store) Save(site Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	site = normalizeSite(site)
	next := cloneSites(s.meta)
	next[site.Name] = site
	if err := s.flush(next); err != nil {
		return err
	}
	s.meta = next
	return nil
}

// UpdateMetadata 修改公开标题和简介，并保留部署文件与创建时间。
func (s *Store) UpdateMetadata(name, title, description string) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	site, ok := s.meta[name]
	if !ok {
		return Site{}, os.ErrNotExist
	}
	site = normalizeSite(site)
	site.Title = strings.TrimSpace(title)
	site.Description = strings.TrimSpace(description)
	site.UpdatedAt = time.Now().UTC()
	next := cloneSites(s.meta)
	next[name] = site
	if err := s.flush(next); err != nil {
		return Site{}, err
	}
	s.meta = next
	return site, nil
}

// Delete 只在目录已安全移入回收站后删除元数据。
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.meta[name]; !ok {
		return os.ErrNotExist
	}
	next := cloneSites(s.meta)
	delete(next, name)
	if err := s.flush(next); err != nil {
		return err
	}
	s.meta = next
	return nil
}

func (s *Store) Usage() (count int, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.meta {
		count++
		total += site.Size
	}
	return
}

func (s *Store) flush(next map[string]Site) error {
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.metaPath), ".meta-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.metaPath); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(s.metaPath)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Store) reconcile() error {
	entries, err := os.ReadDir(s.sitesDir)
	if err != nil {
		return err
	}
	next := cloneSites(s.meta)
	changed := false
	present := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		present[name] = true
		site, ok := next[name]
		files, size := dirStats(filepath.Join(s.sitesDir, name))
		if !ok {
			info, _ := entry.Info()
			createdAt := time.Now().UTC()
			if info != nil {
				createdAt = info.ModTime().UTC()
			}
			next[name] = Site{
				Name:      name,
				Title:     name,
				URL:       "/s/" + name + "/",
				Origin:    "recovered",
				Files:     files,
				Size:      size,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			changed = true
			continue
		}
		normalized := normalizeSite(site)
		if normalized != site || site.URL != "/s/"+name+"/" || site.Files != files || site.Size != size {
			site = normalized
			site.URL = "/s/" + name + "/"
			site.Files = files
			site.Size = size
			next[name] = site
			changed = true
		}
	}
	for name := range next {
		if !present[name] {
			delete(next, name)
			changed = true
		}
	}
	if changed {
		if err := s.flush(next); err != nil {
			return err
		}
		s.meta = next
	}
	return nil
}

func normalizeSite(site Site) Site {
	if site.Title == "" {
		site.Title = site.Name
	}
	if site.UpdatedAt.IsZero() {
		site.UpdatedAt = site.CreatedAt
	}
	return site
}

func cloneSites(in map[string]Site) map[string]Site {
	out := make(map[string]Site, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// dirStats 统计目录下文件数与总字节数。
func dirStats(dir string) (files int, size int64) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			files++
			size += info.Size()
		}
		return nil
	})
	return
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify 把任意字符串规整为 URL 安全的站点名。
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// randSuffix 生成 4 位小写字母数字后缀，避免自动命名撞名。
func randSuffix() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 4)
	raw := make([]byte, len(b))
	for i := range b {
		for {
			if _, err := rand.Read(raw[i : i+1]); err != nil {
				panic("系统随机数不可用: " + err.Error())
			}
			// 252 是 36 的最大整数倍，拒绝其后的值以避免模偏差。
			if raw[i] < 252 {
				b[i] = alphabet[int(raw[i])%len(alphabet)]
				break
			}
		}
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
