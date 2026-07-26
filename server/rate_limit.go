package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateEntry struct {
	windowStart time.Time
	failures    int
}

type failureLimiter struct {
	mu        sync.Mutex
	entries   map[string]rateEntry
	limit     int
	window    time.Duration
	lastSweep time.Time
}

func newFailureLimiter(limit int, window time.Duration) *failureLimiter {
	return &failureLimiter{
		entries:   make(map[string]rateEntry),
		limit:     limit,
		window:    window,
		lastSweep: time.Now(),
	}
}

// RecordFailure 返回 false 表示本窗口内失败次数已经超过上限。
func (l *failureLimiter) RecordFailure(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > 10*l.window {
		for k, entry := range l.entries {
			if now.Sub(entry.windowStart) > l.window {
				delete(l.entries, k)
			}
		}
		l.lastSweep = now
	}
	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateEntry{windowStart: now}
	}
	entry.failures++
	l.entries[key] = entry
	return entry.failures <= l.limit
}

func (l *failureLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first, _, ok := strings.Cut(forwarded, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
