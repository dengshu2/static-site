package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	_ "time/tzdata"
)

const statsRetentionDays = 90

type SiteStats struct {
	Views        uint64            `json:"views"`
	LastViewedAt *time.Time        `json:"lastViewedAt,omitempty"`
	Daily        map[string]uint64 `json:"daily,omitempty"`
}

type StatsStore struct {
	mu    sync.Mutex
	path  string
	sites map[string]SiteStats
}

type AnalyticsDay struct {
	Date  string `json:"date"`
	Views uint64 `json:"views"`
}

type AnalyticsSummary struct {
	TotalSites  int            `json:"totalSites"`
	TotalFiles  int            `json:"totalFiles"`
	TotalBytes  int64          `json:"totalBytes"`
	TotalViews  uint64         `json:"totalViews"`
	ViewsToday  uint64         `json:"viewsToday"`
	UpdatedWeek int            `json:"updatedThisWeek"`
	RecentViews []AnalyticsDay `json:"recentViews"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

func NewStatsStore(dataDir string) (*StatsStore, error) {
	store := &StatsStore{
		path:  filepath.Join(dataDir, "stats.json"),
		sites: map[string]SiteStats{},
	}
	b, err := os.ReadFile(store.path)
	if err == nil {
		if err := json.Unmarshal(b, &store.sites); err != nil {
			return nil, fmt.Errorf("访问统计损坏，拒绝静默启动: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return store, nil
}

func (s *StatsStore) Get(name string) SiteStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSiteStats(s.sites[name])
}

func (s *StatsStore) RecordView(name string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	localNow := now.In(time.Local)
	recordedAt := now.UTC()
	next := cloneStats(s.sites)
	stats := next[name]
	stats.Views++
	stats.LastViewedAt = &recordedAt
	if stats.Daily == nil {
		stats.Daily = map[string]uint64{}
	}
	stats.Daily[localNow.Format(time.DateOnly)]++
	cutoff := localNow.AddDate(0, 0, -statsRetentionDays).Format(time.DateOnly)
	for day := range stats.Daily {
		if day < cutoff {
			delete(stats.Daily, day)
		}
	}
	next[name] = stats
	if err := s.flush(next); err != nil {
		return err
	}
	s.sites = next
	return nil
}

func (s *StatsStore) Reset(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sites[name]; !ok {
		return nil
	}
	next := cloneStats(s.sites)
	delete(next, name)
	if err := s.flush(next); err != nil {
		return err
	}
	s.sites = next
	return nil
}

func (s *StatsStore) Analytics(sites []Site, now time.Time) AnalyticsSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	localNow := now.In(time.Local)
	summary := AnalyticsSummary{
		TotalSites:  len(sites),
		GeneratedAt: now.UTC(),
	}
	active := make(map[string]bool, len(sites))
	weekAgo := localNow.AddDate(0, 0, -7)
	for _, site := range sites {
		active[site.Name] = true
		summary.TotalFiles += site.Files
		summary.TotalBytes += site.Size
		if normalizeSite(site).UpdatedAt.After(weekAgo) {
			summary.UpdatedWeek++
		}
	}
	today := localNow.Format(time.DateOnly)
	for name, stats := range s.sites {
		if !active[name] {
			continue
		}
		summary.TotalViews += stats.Views
		summary.ViewsToday += stats.Daily[today]
	}
	for offset := 13; offset >= 0; offset-- {
		day := localNow.AddDate(0, 0, -offset).Format(time.DateOnly)
		item := AnalyticsDay{Date: day}
		for name, stats := range s.sites {
			if active[name] {
				item.Views += stats.Daily[day]
			}
		}
		summary.RecentViews = append(summary.RecentViews, item)
	}
	return summary
}

func (s *StatsStore) flush(next map[string]SiteStats) error {
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".stats-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	if dataDir, err := os.Open(dir); err == nil {
		_ = dataDir.Sync()
		_ = dataDir.Close()
	}
	return nil
}

func cloneStats(in map[string]SiteStats) map[string]SiteStats {
	out := make(map[string]SiteStats, len(in))
	for name, stats := range in {
		out[name] = cloneSiteStats(stats)
	}
	return out
}

func cloneSiteStats(in SiteStats) SiteStats {
	out := in
	if in.LastViewedAt != nil {
		value := *in.LastViewedAt
		out.LastViewedAt = &value
	}
	if in.Daily != nil {
		out.Daily = make(map[string]uint64, len(in.Daily))
		keys := make([]string, 0, len(in.Daily))
		for key := range in.Daily {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out.Daily[key] = in.Daily[key]
		}
	}
	return out
}
