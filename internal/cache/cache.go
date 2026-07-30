// Package cache keeps an in-memory issue snapshot refreshed in the background.
package cache

import (
	"log/slog"
	"sync"
	"time"

	"k8s-scout/internal/github"
	"k8s-scout/internal/issue"
)

// DefaultInterval is how often the cache refreshes from GitHub.
const DefaultInterval = 15 * time.Minute

// Cache holds the in-memory issue list refreshed in the background.
type Cache struct {
	mu        sync.RWMutex
	issues    []issue.Issue
	updatedAt time.Time
	err       error
	lastErr   string
}

// Get returns a copy of cached issues.
func (c *Cache) Get() ([]issue.Issue, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]issue.Issue, len(c.issues))
	copy(out, c.issues)
	return out, c.updatedAt, c.err
}

// Set stores a successful snapshot or records a refresh failure.
func (c *Cache) Set(issues []issue.Issue, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.issues = issues
		c.updatedAt = time.Now().UTC()
		c.err = nil
		c.lastErr = ""
		return
	}
	c.lastErr = err.Error()
	if len(c.issues) == 0 {
		c.err = err
	}
}

// Health is a JSON-friendly cache status for probes.
type Health struct {
	Status     string `json:"status"`
	Issues     int    `json:"issues"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HealthSnapshot returns the current probe payload.
func (c *Cache) HealthSnapshot() Health {
	c.mu.RLock()
	defer c.mu.RUnlock()

	h := Health{Issues: len(c.issues)}
	if !c.updatedAt.IsZero() {
		h.UpdatedAt = c.updatedAt.Format(time.RFC3339)
		h.AgeSeconds = int64(time.Since(c.updatedAt).Seconds())
	}
	if c.lastErr != "" {
		h.Error = c.lastErr
	}
	switch {
	case len(c.issues) == 0 && c.err != nil:
		h.Status = "error"
	case c.lastErr != "" && len(c.issues) > 0:
		h.Status = "degraded"
	case len(c.issues) == 0:
		h.Status = "starting"
	default:
		h.Status = "ok"
	}
	return h
}

// StartRefresher loads issues immediately and refreshes on interval.
func StartRefresher(c *Cache, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	refresh := func() {
		start := time.Now()
		issues, err := github.FetchIssues()
		if err != nil {
			slog.Warn("cache refresh failed", "err", err, "duration_ms", time.Since(start).Milliseconds())
			c.Set(nil, err)
			return
		}
		c.Set(issues, nil)
		slog.Info("cache refreshed", "issues", len(issues), "duration_ms", time.Since(start).Milliseconds())
	}

	refresh()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()
}
