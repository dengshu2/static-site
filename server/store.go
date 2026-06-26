package main

import (
	"crypto/rand"
	"encoding/json"
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
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Origin    string    `json:"origin"` // 上传时的原始文件名
	Files     int       `json:"files"`  // 站点内文件数
	Size      int64     `json:"size"`   // 站点总字节数
	CreatedAt time.Time `json:"createdAt"`
}

// Store 管理站点目录与 meta.json，单进程内用一把锁串行化所有写操作。
type Store struct {
	mu       sync.Mutex
	dataDir  string
	sitesDir string
	metaPath string
	meta     map[string]Site
}

func NewStore(dataDir string) (*Store, error) {
	s := &Store{
		dataDir:  dataDir,
		sitesDir: filepath.Join(dataDir, "sites"),
		metaPath: filepath.Join(dataDir, "meta.json"),
		meta:     map[string]Site{},
	}
	if b, err := os.ReadFile(s.metaPath); err == nil {
		_ = json.Unmarshal(b, &s.meta)
	}
	return s, nil
}

// List 按创建时间倒序返回所有站点。
func (s *Store) List() []Site {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Site, 0, len(s.meta))
	for _, v := range s.meta {
		out = append(out, v)
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
	return v, ok
}

// Save 记录一条站点元数据并落盘。
func (s *Store) Save(site Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta[site.Name] = site
	return s.flush()
}

// Delete 删除站点目录与元数据。
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.meta[name]; !ok {
		return os.ErrNotExist
	}
	if err := os.RemoveAll(filepath.Join(s.sitesDir, name)); err != nil {
		return err
	}
	delete(s.meta, name)
	return s.flush()
}

func (s *Store) flush() error {
	b, _ := json.MarshalIndent(s.meta, "", "  ")
	tmp := s.metaPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.metaPath)
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
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
