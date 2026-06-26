package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// handleUpload 接收 HTML 或 zip 并部署为一个站点。
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	maxBytes := a.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("上传体积超限或表单无效（上限 %dMB）", a.cfg.MaxUploadMB))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("缺少 file 字段"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("读取文件失败: %v", err))
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".zip" && ext != ".html" && ext != ".htm" {
		writeJSON(w, http.StatusBadRequest, errBody("仅支持 .html / .htm / .zip"))
		return
	}

	name := a.resolveName(r.FormValue("name"), header.Filename)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errBody("站点名为空或全为非法字符"))
		return
	}

	overwrite := r.FormValue("overwrite") == "true"
	if a.store.Exists(name) && !overwrite {
		writeJSON(w, http.StatusConflict, errBody("站点 %q 已存在，请改名或勾选覆盖", name))
		return
	}

	// 先解压/写入到临时目录，成功后再原子替换正式目录。
	tmpDir := filepath.Join(a.cfg.DataDir, "sites", ".tmp-"+randSuffix())
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("创建临时目录失败: %v", err))
		return
	}
	defer os.RemoveAll(tmpDir)

	if ext == ".zip" {
		if err := extractZip(data, tmpDir, a.cfg.MaxUnzipMB<<20); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody("%v", err))
			return
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "index.html")); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody("zip 根目录缺少 index.html"))
			return
		}
	} else {
		if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), data, 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody("写入失败: %v", err))
			return
		}
	}

	dest := filepath.Join(a.cfg.DataDir, "sites", name)
	_ = os.RemoveAll(dest) // 覆盖场景：先清掉旧目录
	if err := os.Rename(tmpDir, dest); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("发布失败: %v", err))
		return
	}

	files, size := dirStats(dest)
	site := Site{
		Name:      name,
		URL:       "/s/" + name + "/",
		Origin:    header.Filename,
		Files:     files,
		Size:      size,
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.Save(site); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("保存元数据失败: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, site)
}

// resolveName 根据用户输入或文件名决定站点名。
// 用户填了 name 就用它的 slug；否则取文件名 slug 加随机后缀防撞。
func (a *App) resolveName(input, filename string) string {
	if s := slugify(input); s != "" {
		return s
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	s := slugify(base)
	if s == "" {
		s = "site"
	}
	return s + "-" + randSuffix()
}

func errBody(format string, args ...any) map[string]string {
	return map[string]string{"error": fmt.Sprintf(format, args...)}
}
