package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

type zipEntry struct {
	name string
	body string
	mode os.FileMode
}

func writeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "test.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func testZipLimits() ZipLimits {
	return ZipLimits{MaxTotalBytes: 1024, MaxFiles: 5, MaxPathDepth: 4, MaxPathBytes: 120}
}

func TestExtractZipRejectsTraversalSymlinkAndTooManyFiles(t *testing.T) {
	tests := []struct {
		name    string
		entries []zipEntry
		limits  ZipLimits
	}{
		{"traversal", []zipEntry{{name: "../escape", body: "x"}}, testZipLimits()},
		{"backslash", []zipEntry{{name: `..\\escape`, body: "x"}}, testZipLimits()},
		{"symlink", []zipEntry{{name: "link", body: "target", mode: os.ModeSymlink | 0o777}}, testZipLimits()},
		{"file-count", []zipEntry{{name: "a"}, {name: "b"}, {name: "c"}}, ZipLimits{MaxTotalBytes: 1024, MaxFiles: 2, MaxPathDepth: 4, MaxPathBytes: 120}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dest := t.TempDir()
			if err := extractZipFile(writeZip(t, tc.entries), dest, tc.limits); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestExtractZipUsesActualWrittenSize(t *testing.T) {
	dest := t.TempDir()
	limits := testZipLimits()
	limits.MaxTotalBytes = 4
	if err := extractZipFile(writeZip(t, []zipEntry{{name: "index.html", body: "12345"}}), dest, limits); err == nil {
		t.Fatal("expected actual-size limit rejection")
	}
}

func TestNormalizeSingleWrapperDirectory(t *testing.T) {
	dest := t.TempDir()
	zipName := writeZip(t, []zipEntry{
		{name: "dist/index.html", body: "<h1>ok</h1>"},
		{name: "dist/app.js", body: "console.log('ok')"},
	})
	if err := extractZipFile(zipName, dest, testZipLimits()); err != nil {
		t.Fatal(err)
	}
	if err := normalizeSiteRoot(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "dist")); !os.IsNotExist(err) {
		t.Fatalf("wrapper still exists: %v", err)
	}
}
