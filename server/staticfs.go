package main

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
)

// sharedFS 先在页面自己的目录里查找，未命中时回落到 web/shared，
// 让公开端和管理端共用同一份基础样式与字体，而不必各存一份。
type sharedFS struct {
	primary  http.FileSystem
	fallback http.FileSystem
}

func (fsys sharedFS) Open(name string) (http.File, error) {
	file, err := fsys.primary.Open(name)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return fsys.fallback.Open(name)
}

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
