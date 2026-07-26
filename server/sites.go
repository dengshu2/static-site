package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type SiteResponse struct {
	Name         string     `json:"name"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	URL          string     `json:"url"`
	Files        int        `json:"files"`
	Size         int64      `json:"size"`
	Views        uint64     `json:"views"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastViewedAt *time.Time `json:"lastViewedAt,omitempty"`
}

type TrashResponse struct {
	ID        string       `json:"id"`
	Site      SiteResponse `json:"site"`
	DeletedAt time.Time    `json:"deletedAt"`
	Reason    string       `json:"reason"`
}

func (a *App) handleListSites(w http.ResponseWriter, _ *http.Request) {
	sites := a.store.List()
	out := make([]SiteResponse, 0, len(sites))
	for _, site := range sites {
		out = append(out, a.siteResponse(site))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleAnalytics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.stats.Analytics(a.store.List(), time.Now()))
}

type updateSiteInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (a *App) handleUpdateSite(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || name != slugify(name) || len(name) > a.cfg.MaxSiteNameLen {
		writeJSON(w, http.StatusBadRequest, errBody("站点名无效"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input updateSiteInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("项目资料格式无效"))
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("项目资料格式无效"))
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateSiteMetadata(input.Title, input.Description); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("%v", err))
		return
	}
	site, err := a.store.UpdateMetadata(name, input.Title, input.Description)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusNotFound, errBody("站点不存在"))
		return
	}
	if err != nil {
		log.Printf("event=metadata_update_failed site=%q ip=%q error=%q", name, clientIP(r), err)
		writeJSON(w, http.StatusInternalServerError, errBody("保存项目资料失败"))
		return
	}
	log.Printf("event=metadata_updated site=%q ip=%q", name, clientIP(r))
	writeJSON(w, http.StatusOK, a.siteResponse(site))
}

func (a *App) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || name != slugify(name) || len(name) > a.cfg.MaxSiteNameLen {
		writeJSON(w, http.StatusBadRequest, errBody("站点名无效"))
		return
	}

	a.mutateMu.Lock()
	defer a.mutateMu.Unlock()
	site, ok := a.store.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("站点不存在"))
		return
	}
	source := filepath.Join(a.sitesDir, name)
	item, err := a.trash.Move(site, source, "deleted")
	if err != nil {
		log.Printf("event=delete_failed site=%q ip=%q error=%q", name, clientIP(r), err)
		writeJSON(w, http.StatusInternalServerError, errBody("移动到回收站失败"))
		return
	}
	if err := a.store.Delete(name); err != nil {
		_ = a.trash.Restore(item, source)
		log.Printf("event=delete_failed site=%q ip=%q error=%q", name, clientIP(r), err)
		writeJSON(w, http.StatusInternalServerError, errBody("保存元数据失败"))
		return
	}
	log.Printf("event=deleted site=%q trash_id=%q ip=%q", name, item.ID, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"trashId": item.ID})
}

func (a *App) handleListTrash(w http.ResponseWriter, _ *http.Request) {
	items := a.trash.List()
	out := make([]TrashResponse, 0, len(items))
	for _, item := range items {
		out = append(out, TrashResponse{
			ID:        item.ID,
			Site:      a.siteResponse(item.Site),
			DeletedAt: item.DeletedAt,
			Reason:    item.Reason,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := a.trash.Get(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, errBody("回收站条目不存在或损坏"))
		return
	}

	a.mutateMu.Lock()
	defer a.mutateMu.Unlock()
	if a.store.Exists(item.Site.Name) {
		writeJSON(w, http.StatusConflict, errBody("同名站点已经存在，无法恢复"))
		return
	}
	count, _ := a.store.Usage()
	if count >= a.cfg.MaxSites {
		writeJSON(w, http.StatusInsufficientStorage, errBody("恢复后将超过站点数量上限"))
		return
	}

	dest := filepath.Join(a.sitesDir, item.Site.Name)
	if err := a.trash.Restore(item, dest); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("恢复文件失败"))
		return
	}
	if err := a.store.Save(item.Site); err != nil {
		_ = a.trash.RollbackRestore(item, dest)
		writeJSON(w, http.StatusInternalServerError, errBody("恢复元数据失败"))
		return
	}
	log.Printf("event=restored site=%q trash_id=%q ip=%q", item.Site.Name, item.ID, clientIP(r))
	writeJSON(w, http.StatusOK, a.siteResponse(item.Site))
}

func (a *App) handlePurgeTrash(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mutateMu.Lock()
	defer a.mutateMu.Unlock()
	if err := a.trash.Purge(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, errBody("永久删除失败"))
		return
	}
	log.Printf("event=trash_purged trash_id=%q ip=%q", id, clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) siteResponse(site Site) SiteResponse {
	site = normalizeSite(site)
	stats := a.stats.Get(site.Name)
	return SiteResponse{
		Name:         site.Name,
		Title:        site.Title,
		Description:  site.Description,
		URL:          strings.TrimRight(a.cfg.ContentBaseURL, "/") + site.URL,
		Files:        site.Files,
		Size:         site.Size,
		Views:        stats.Views,
		CreatedAt:    site.CreatedAt,
		UpdatedAt:    site.UpdatedAt,
		LastViewedAt: stats.LastViewedAt,
	}
}

func validateSiteMetadata(title, description string) error {
	if utf8.RuneCountInString(title) > 120 {
		return errors.New("项目标题不能超过 120 个字符")
	}
	if utf8.RuneCountInString(description) > 400 {
		return errors.New("项目简介不能超过 400 个字符")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("请求只能包含一个 JSON 对象")
		}
		return err
	}
	return nil
}
