package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRejectsMalformedMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sites"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{broken"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir); err == nil {
		t.Fatal("expected malformed metadata error")
	}
}

func TestStoreSaveFailureDoesNotMutateMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store.metaPath = filepath.Join(dir, "cannot-replace-directory")
	if err := os.Mkdir(store.metaPath, 0o750); err != nil {
		t.Fatal(err)
	}
	site := Site{Name: "demo", URL: "/s/demo/", CreatedAt: time.Now().UTC()}
	if err := store.Save(site); err == nil {
		t.Fatal("expected save failure")
	}
	if _, ok := store.Get("demo"); ok {
		t.Fatal("failed save mutated in-memory metadata")
	}
}

func TestStoreReconcilesOrphanDirectory(t *testing.T) {
	dir := t.TempDir()
	siteDir := filepath.Join(dir, "sites", "orphan")
	if err := os.MkdirAll(siteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, ok := store.Get("orphan")
	if !ok || site.Files != 1 || site.Size != 5 {
		t.Fatalf("orphan not reconciled: %#v ok=%t", site, ok)
	}
}
