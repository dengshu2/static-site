package main

import (
	"errors"
	"net/http"
	"os"
)

// handleListSites 返回所有已部署站点，按创建时间倒序。
func (a *App) handleListSites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.store.List())
}

// handleDeleteSite 删除指定站点的目录与元数据。
func (a *App) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	name := slugify(r.PathValue("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errBody("站点名无效"))
		return
	}
	switch err := a.store.Delete(name); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, os.ErrNotExist):
		writeJSON(w, http.StatusNotFound, errBody("站点不存在"))
	default:
		writeJSON(w, http.StatusInternalServerError, errBody("删除失败: %v", err))
	}
}
