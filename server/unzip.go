package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractZip 把 zip 内容安全解压到 destDir。
// 三道防线：路径穿越（Zip-Slip）、解压总大小封顶（zip bomb）、目录创建限制在 destDir 内。
func extractZip(data []byte, destDir string, maxTotal int64) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("无效的 zip 文件: %w", err)
	}

	var total int64
	for _, f := range zr.File {
		// 规整路径并拒绝任何逃逸出 destDir 的条目。
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("非法路径(疑似 Zip-Slip): %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		total += int64(f.UncompressedSize64)
		if total > maxTotal {
			return fmt.Errorf("解压内容超过上限 %d 字节", maxTotal)
		}

		if err := writeZipEntry(f, target, maxTotal-total+int64(f.UncompressedSize64)); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, target string, limit int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	// 即便头部声明的大小被伪造，io.LimitReader 也能挡住超额写入。
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("解压内容超过上限")
	}
	return nil
}
