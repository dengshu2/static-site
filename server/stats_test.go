package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatsStorePersistsAndBuildsFourteenDayTrend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStatsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	if err := store.RecordView("demo", now); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordView("demo", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStatsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	stats := reloaded.Get("demo")
	if stats.Views != 2 || stats.LastViewedAt == nil {
		t.Fatalf("unexpected persisted stats: %#v", stats)
	}
	summary := reloaded.Analytics([]Site{{
		Name: "demo", Files: 2, Size: 123, CreatedAt: now, UpdatedAt: now,
	}}, now)
	if summary.TotalViews != 2 || summary.ViewsToday != 1 || len(summary.RecentViews) != 14 {
		t.Fatalf("unexpected analytics: %#v", summary)
	}
}

func TestStatsStoreRejectsMalformedData(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte("{broken"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStatsStore(dir); err == nil {
		t.Fatal("expected malformed stats error")
	}
}
