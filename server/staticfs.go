package main

import (
	"net/http"
	"os"
	"path"
)

// indexOnlyFS 禁用 FileServer 默认的目录列表，只允许目录中的 index.html。
type indexOnlyFS struct {
	http.FileSystem
}

func (fsys indexOnlyFS) Open(name string) (http.File, error) {
	file, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.IsDir() {
		return file, nil
	}
	index, err := fsys.FileSystem.Open(path.Join(name, "index.html"))
	if err != nil {
		file.Close()
		return nil, os.ErrNotExist
	}
	indexInfo, statErr := index.Stat()
	index.Close()
	if statErr != nil || !indexInfo.Mode().IsRegular() {
		file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}
