package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type uploadInput struct {
	tempFile    string
	filename    string
	name        string
	title       string
	description string
	overwrite   bool
}

type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

// handleUpload 接收 HTML 或 zip，先在 /data/tmp 流式落盘，再发布到同一文件系统。
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	input, err := a.readUpload(w, r)
	if err != nil {
		var reqErr *requestError
		if errors.As(err, &reqErr) {
			writeJSON(w, reqErr.status, errBody("%s", reqErr.msg))
		} else {
			writeJSON(w, http.StatusBadRequest, errBody("上传失败: %v", err))
		}
		return
	}
	defer os.Remove(input.tempFile)

	name, err := a.resolveName(input.name, input.filename)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("%v", err))
		return
	}

	stageDir, err := os.MkdirTemp(a.sitesDir, ".stage-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("创建发布目录失败"))
		return
	}
	defer os.RemoveAll(stageDir)

	ext := strings.ToLower(filepath.Ext(input.filename))
	switch ext {
	case ".zip":
		err = extractZipFile(input.tempFile, stageDir, ZipLimits{
			MaxTotalBytes: a.cfg.MaxUnzipMB << 20,
			MaxFiles:      a.cfg.MaxZipFiles,
			MaxPathDepth:  a.cfg.MaxPathDepth,
			MaxPathBytes:  240,
		})
		if err == nil {
			err = normalizeSiteRoot(stageDir)
		}
	case ".html", ".htm":
		err = copyFile(input.tempFile, filepath.Join(stageDir, "index.html"))
	default:
		err = errors.New("仅支持 .html / .htm / .zip")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("%v", err))
		return
	}

	files, size := dirStats(stageDir)
	site := Site{
		Name:        name,
		Title:       input.title,
		Description: input.description,
		URL:         "/s/" + name + "/",
		Origin:      input.filename,
		Files:       files,
		Size:        size,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := validateSiteMetadata(site.Title, site.Description); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("%v", err))
		return
	}
	if err := a.publish(stageDir, site, input.overwrite); err != nil {
		var reqErr *requestError
		if errors.As(err, &reqErr) {
			writeJSON(w, reqErr.status, errBody("%s", reqErr.msg))
		} else {
			log.Printf("event=upload_failed site=%q ip=%q error=%q", name, clientIP(r), err)
			writeJSON(w, http.StatusInternalServerError, errBody("发布失败"))
		}
		return
	}

	a.covers.Request(name)
	log.Printf("event=uploaded site=%q files=%d bytes=%d overwrite=%t ip=%q", name, files, size, input.overwrite, clientIP(r))
	published, _ := a.store.Get(name)
	writeJSON(w, http.StatusCreated, a.siteResponse(published))
}

func (a *App) readUpload(w http.ResponseWriter, r *http.Request) (uploadInput, error) {
	// 额外 1 MiB 留给 multipart 边界和小字段；文件本身仍单独按 MaxUploadMB 限制。
	maxFile := a.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxFile+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		return uploadInput{}, &requestError{http.StatusBadRequest, "表单格式无效"}
	}

	temp, err := os.CreateTemp(a.uploadDir, "upload-*")
	if err != nil {
		return uploadInput{}, err
	}
	input := uploadInput{tempFile: temp.Name()}
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(temp.Name())
		}
	}()

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return uploadInput{}, &requestError{http.StatusBadRequest, "上传体积超限或表单损坏"}
		}
		switch part.FormName() {
		case "file":
			if input.filename != "" {
				return uploadInput{}, &requestError{http.StatusBadRequest, "只能上传一个文件"}
			}
			input.filename = safeUploadFilename(part.FileName())
			if input.filename == "" {
				return uploadInput{}, &requestError{http.StatusBadRequest, "缺少文件名"}
			}
			n, err := io.Copy(temp, io.LimitReader(part, maxFile+1))
			if err != nil {
				return uploadInput{}, err
			}
			if n > maxFile {
				return uploadInput{}, &requestError{http.StatusRequestEntityTooLarge, fmt.Sprintf("文件超过 %dMB 上限", a.cfg.MaxUploadMB)}
			}
		case "name":
			input.name, err = readSmallField(part)
			if err != nil {
				return uploadInput{}, &requestError{http.StatusBadRequest, "站点名字段过长"}
			}
		case "title":
			input.title, err = readSmallField(part)
			if err != nil {
				return uploadInput{}, &requestError{http.StatusBadRequest, "项目标题字段过长"}
			}
		case "description":
			input.description, err = readSmallField(part)
			if err != nil {
				return uploadInput{}, &requestError{http.StatusBadRequest, "项目简介字段过长"}
			}
		case "overwrite":
			value, err := readSmallField(part)
			if err != nil {
				return uploadInput{}, &requestError{http.StatusBadRequest, "覆盖字段无效"}
			}
			input.overwrite = value == "true"
		default:
			return uploadInput{}, &requestError{http.StatusBadRequest, "包含未知表单字段"}
		}
	}
	if input.filename == "" {
		return uploadInput{}, &requestError{http.StatusBadRequest, "缺少 file 字段"}
	}
	if err := temp.Sync(); err != nil {
		return uploadInput{}, err
	}
	if err := temp.Close(); err != nil {
		return uploadInput{}, err
	}
	ok = true
	return input, nil
}

func readSmallField(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, 4097))
	if err != nil {
		return "", err
	}
	if len(b) > 4096 {
		return "", errors.New("字段过长")
	}
	return strings.TrimSpace(string(b)), nil
}

func safeUploadFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	return path.Base(strings.TrimSpace(name))
}

func (a *App) resolveName(input, filename string) (string, error) {
	if strings.TrimSpace(input) != "" {
		name := slugify(input)
		if name == "" {
			return "", errors.New("站点名必须包含英文字母或数字")
		}
		if len(name) > a.cfg.MaxSiteNameLen {
			return "", fmt.Errorf("站点名不能超过 %d 个字符", a.cfg.MaxSiteNameLen)
		}
		return name, nil
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	name := slugify(base)
	if name == "" {
		name = "site"
	}
	maxBase := a.cfg.MaxSiteNameLen - 5
	if len(name) > maxBase {
		name = strings.Trim(name[:maxBase], "-")
	}
	return name + "-" + randSuffix(), nil
}

func (a *App) publish(stageDir string, site Site, overwrite bool) error {
	a.mutateMu.Lock()
	defer a.mutateMu.Unlock()

	old, exists := a.store.Get(site.Name)
	if exists && !overwrite {
		return &requestError{http.StatusConflict, fmt.Sprintf("站点 %q 已存在，请改名或确认覆盖", site.Name)}
	}
	count, onlineBytes := a.store.Usage()
	if !exists && count >= a.cfg.MaxSites {
		return &requestError{http.StatusInsufficientStorage, fmt.Sprintf("站点数已达到上限 %d", a.cfg.MaxSites)}
	}
	_, trashBytes := a.trash.Usage()
	if onlineBytes+trashBytes+site.Size > a.cfg.MaxTotalMB<<20 {
		return &requestError{http.StatusInsufficientStorage, fmt.Sprintf("在线项目与回收站总容量将超过 %dMB", a.cfg.MaxTotalMB)}
	}

	dest := filepath.Join(a.sitesDir, site.Name)
	var previous *TrashItem
	if exists {
		site.CreatedAt = old.CreatedAt
		if site.Title == "" {
			site.Title = old.Title
		}
		if site.Description == "" {
			site.Description = old.Description
		}
		item, err := a.trash.Move(old, dest, "overwritten")
		if err != nil {
			return err
		}
		previous = &item
	} else if _, err := os.Stat(dest); err == nil {
		return errors.New("发现未登记的同名目录，拒绝覆盖")
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(stageDir, dest); err != nil {
		if previous != nil {
			_ = a.trash.Restore(*previous, dest)
		}
		return err
	}
	if err := a.store.Save(site); err != nil {
		_ = os.RemoveAll(dest)
		if previous != nil {
			_ = a.trash.Restore(*previous, dest)
		}
		return err
	}
	if !exists {
		if err := a.stats.Reset(site.Name); err != nil {
			log.Printf("event=view_stat_reset_failed site=%q error=%q", site.Name, err)
		}
	}
	return nil
}

func copyFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dest)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func errBody(format string, args ...any) map[string]string {
	return map[string]string{"error": fmt.Sprintf(format, args...)}
}
