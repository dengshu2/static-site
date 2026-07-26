package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type ZipLimits struct {
	MaxTotalBytes int64
	MaxFiles      int
	MaxPathDepth  int
	MaxPathBytes  int
}

// extractZipFile 流式解压已经落盘的 zip，并限制路径、实际写入大小、文件数和目录深度。
func extractZipFile(zipPath, destDir string, limits ZipLimits) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("无效的 zip 文件: %w", err)
	}
	defer zr.Close()

	if len(zr.File) > limits.MaxFiles {
		return fmt.Errorf("zip 条目数超过上限 %d", limits.MaxFiles)
	}

	seen := make(map[string]struct{}, len(zr.File))
	var total int64
	files := 0
	for _, f := range zr.File {
		clean, err := cleanZipPath(f.Name, limits)
		if err != nil {
			return err
		}
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("zip 含重复路径: %s", clean)
		}
		seen[clean] = struct{}{}

		target := filepath.Join(destDir, filepath.FromSlash(clean))
		if !withinDir(destDir, target) {
			return fmt.Errorf("非法路径(疑似 Zip-Slip): %s", f.Name)
		}

		mode := f.Mode()
		if mode&os.ModeSymlink != 0 || mode&os.ModeType != 0 && !mode.IsDir() {
			return fmt.Errorf("不支持符号链接或特殊文件: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}

		files++
		if files > limits.MaxFiles {
			return fmt.Errorf("zip 文件数超过上限 %d", limits.MaxFiles)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}

		written, err := writeZipEntry(f, target, limits.MaxTotalBytes-total)
		if err != nil {
			return err
		}
		total += written
	}
	return nil
}

func cleanZipPath(name string, limits ZipLimits) (string, error) {
	if strings.ContainsRune(name, '\x00') || strings.Contains(name, `\`) {
		return "", fmt.Errorf("zip 路径包含非法字符: %q", name)
	}
	clean := path.Clean(strings.TrimSpace(name))
	if clean == "." || clean == "" {
		return "", nil
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("非法路径(疑似 Zip-Slip): %s", name)
	}
	if path.Base(clean) == trashMetaFile {
		return "", fmt.Errorf("zip 使用了系统保留文件名: %s", trashMetaFile)
	}
	if len(clean) > limits.MaxPathBytes {
		return "", fmt.Errorf("zip 路径过长（上限 %d 字节）: %s", limits.MaxPathBytes, clean)
	}
	parts := strings.Split(clean, "/")
	if len(parts) > limits.MaxPathDepth {
		return "", fmt.Errorf("zip 目录层级超过上限 %d: %s", limits.MaxPathDepth, clean)
	}
	for _, part := range parts {
		if part == "" || len(part) > 120 {
			return "", fmt.Errorf("zip 路径分段无效或过长: %s", clean)
		}
	}
	return clean, nil
}

func withinDir(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func writeZipEntry(f *zip.File, target string, remaining int64) (int64, error) {
	if remaining < 0 {
		return 0, errors.New("解压内容超过总大小上限")
	}
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return 0, err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()

	n, err := io.Copy(out, io.LimitReader(rc, remaining+1))
	if err != nil {
		return n, err
	}
	if n > remaining {
		return n, errors.New("解压内容超过总大小上限")
	}
	if err := out.Sync(); err != nil {
		return n, err
	}
	if err := out.Close(); err != nil {
		return n, err
	}
	ok = true
	return n, nil
}

// normalizeSiteRoot 接受常见的单层包装目录，例如 dist/index.html。
func normalizeSiteRoot(destDir string) error {
	if isRegularFile(filepath.Join(destDir, "index.html")) {
		return nil
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return err
	}
	var wrapper os.DirEntry
	for _, entry := range entries {
		if entry.Name() == "__MACOSX" || entry.Name() == ".DS_Store" {
			_ = os.RemoveAll(filepath.Join(destDir, entry.Name()))
			continue
		}
		if wrapper != nil || !entry.IsDir() {
			return errors.New("zip 根目录缺少 index.html；仅支持根目录或单层包装目录")
		}
		wrapper = entry
	}
	if wrapper == nil || !isRegularFile(filepath.Join(destDir, wrapper.Name(), "index.html")) {
		return errors.New("zip 根目录缺少 index.html")
	}
	wrapperDir := filepath.Join(destDir, wrapper.Name())
	children, err := os.ReadDir(wrapperDir)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := os.Rename(filepath.Join(wrapperDir, child.Name()), filepath.Join(destDir, child.Name())); err != nil {
			return fmt.Errorf("展开包装目录失败: %w", err)
		}
	}
	return os.Remove(wrapperDir)
}

func isRegularFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}
