package cache_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/github"
	"k8s-scout/internal/issue"
)

func TestDefaultInterval(t *testing.T) {
	if cache.DefaultInterval != 15*time.Minute {
		t.Fatalf("DefaultInterval = %v, want 15m (unauthenticated Search budget)", cache.DefaultInterval)
	}
}

func TestCacheConcurrentGetSetHealth(t *testing.T) {
	c := &cache.Cache{}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(3)
		go func(n int) {
			defer wg.Done()
			c.Set([]issue.Issue{{
				Title:      fmt.Sprintf("issue-%d", n),
				Repository: "kubernetes-sigs/kind",
				HTMLURL:    fmt.Sprintf("https://example.com/%d", n),
			}}, nil)
		}(i)
		go func() {
			defer wg.Done()
			got, _, err := c.Get()
			if err != nil {
				t.Errorf("Get during concurrent Set: %v", err)
			}
			_ = len(got)
		}()
		go func() {
			defer wg.Done()
			h := c.HealthSnapshot()
			if h.Issues < 0 {
				t.Errorf("health issues = %d", h.Issues)
			}
		}()
	}
	wg.Wait()

	got, updatedAt, err := c.Get()
	if err != nil || updatedAt.IsZero() || len(got) != 1 {
		t.Fatalf("after concurrent writes Get = %v %v %v", got, updatedAt, err)
	}
	c.Set(nil, errors.New("boom"))
	h := c.HealthSnapshot()
	if h.Status != "degraded" || h.Issues != 1 || h.Error != "boom" {
		t.Fatalf("failed refresh after concurrent writes = %+v", h)
	}
}

func TestGetSetCopy(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "one", Repository: "kubernetes-sigs/kind"}}, nil)

	got, updatedAt, err := c.Get()
	if err != nil || updatedAt.IsZero() || len(got) != 1 {
		t.Fatalf("Get = %v %v %v", got, updatedAt, err)
	}
	got[0].Title = "mutated"
	again, _, _ := c.Get()
	if again[0].Title != "one" {
		t.Fatal("Get should return a copy")
	}

	h := c.HealthSnapshot()
	if h.Status != "ok" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

func TestSetCopiesInputSlice(t *testing.T) {
	c := &cache.Cache{}
	src := []issue.Issue{{Title: "one", Repository: "kubernetes-sigs/kind"}}

	c.Set(src, nil)
	src[0].Title = "mutated-after-set"

	got, _, _ := c.Get()
	if got[0].Title != "one" {
		t.Fatalf("cache mutated via input slice: got title %q", got[0].Title)
	}
}

func TestHealthDegraded(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "one"}}, nil)
	c.Set(nil, errors.New("boom"))
	h := c.HealthSnapshot()
	if h.Status != "degraded" || h.Error != "boom" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
	got, _, err := c.Get()
	if err != nil || len(got) != 1 || got[0].Title != "one" {
		t.Fatalf("degraded Get should keep snapshot: %v %v", got, err)
	}
}

func TestHealthErrorWhenEmpty(t *testing.T) {
	c := &cache.Cache{}
	c.Set(nil, errors.New("boom"))
	got, _, err := c.Get()
	if err == nil || err.Error() != "boom" || len(got) != 0 {
		t.Fatalf("Get = %v %v", got, err)
	}
	h := c.HealthSnapshot()
	if h.Status != "error" || h.Error != "boom" || h.Issues != 0 {
		t.Fatalf("health = %+v", h)
	}
}

func TestHealthStarting(t *testing.T) {
	c := &cache.Cache{}
	h := c.HealthSnapshot()
	if h.Status != "starting" || h.Issues != 0 || h.Error != "" {
		t.Fatalf("health = %+v", h)
	}
}

func TestHealthSnapshotUpdatedAtRFC3339(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "one"}}, nil)
	h := c.HealthSnapshot()
	if h.Status != "ok" {
		t.Fatalf("health = %+v", h)
	}
	parsed, err := time.Parse(time.RFC3339, h.UpdatedAt)
	if err != nil {
		t.Fatalf("updated_at %q is not RFC3339: %v", h.UpdatedAt, err)
	}
	if parsed.Location() != time.UTC {
		t.Fatalf("updated_at location = %v, want UTC", parsed.Location())
	}
	if h.AgeSeconds < 0 {
		t.Fatalf("age_seconds = %d", h.AgeSeconds)
	}
}

func TestSetSuccessClearsError(t *testing.T) {
	c := &cache.Cache{}
	c.Set(nil, errors.New("boom"))
	c.Set([]issue.Issue{{Title: "recovered"}}, nil)
	_, _, err := c.Get()
	if err != nil {
		t.Fatal(err)
	}
	h := c.HealthSnapshot()
	if h.Status != "ok" || h.Error != "" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

func waitHealth(t *testing.T, c *cache.Cache, want string) cache.Health {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var h cache.Health
	for time.Now().Before(deadline) {
		h = c.HealthSnapshot()
		if h.Status == want {
			return h
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("health status never became %s: %+v", want, h)
	return h
}

func TestStartRefresherPopulatesCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items": []map[string]any{{
				"title":          "From refresher",
				"html_url":       "https://github.com/kubernetes-sigs/kind/issues/9",
				"comments":       0,
				"created_at":     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				"labels":         []map[string]string{{"name": "good first issue"}},
				"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	prev := github.DefaultClient
	github.DefaultClient = &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 10}
	t.Cleanup(func() { github.DefaultClient = prev })

	c := &cache.Cache{}
	cache.StartRefresher(c, 0) // 0 → DefaultInterval; ticker must not fire during this test
	waitHealth(t, c, "ok")

	got, updatedAt, err := c.Get()
	if err != nil || updatedAt.IsZero() || len(got) != 1 || got[0].Title != "From refresher" {
		t.Fatalf("Get = %v %v %v", got, updatedAt, err)
	}
	h := c.HealthSnapshot()
	if h.Status != "ok" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

func TestStartRefresherRecordsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	prev := github.DefaultClient
	github.DefaultClient = &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	t.Cleanup(func() { github.DefaultClient = prev })

	c := &cache.Cache{}
	cache.StartRefresher(c, time.Hour)
	waitHealth(t, c, "error")

	got, _, err := c.Get()
	if err == nil || len(got) != 0 {
		t.Fatalf("Get = %v %v", got, err)
	}
	h := c.HealthSnapshot()
	if h.Status != "error" || h.Error == "" {
		t.Fatalf("health = %+v", h)
	}
}

func TestStartRefresherDoesNotBlockOnFirstFetch(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		if r.URL.Path != "/search/issues" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items": []map[string]any{{
				"title":          "Late",
				"html_url":       "https://github.com/kubernetes-sigs/kind/issues/10",
				"comments":       0,
				"created_at":     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				"labels":         []map[string]string{{"name": "good first issue"}},
				"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	prev := github.DefaultClient
	github.DefaultClient = &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 10}
	t.Cleanup(func() { github.DefaultClient = prev })

	c := &cache.Cache{}
	returned := make(chan struct{})
	go func() {
		cache.StartRefresher(c, time.Hour)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartRefresher blocked on the first GitHub fetch")
	}

	h := c.HealthSnapshot()
	if h.Status != "starting" || h.Issues != 0 {
		t.Fatalf("health while fetch in flight = %+v, want starting", h)
	}

	close(release)
	waitHealth(t, c, "ok")
	got, _, err := c.Get()
	if err != nil || len(got) != 1 || got[0].Title != "Late" {
		t.Fatalf("Get after release = %v %v", got, err)
	}
}

func TestStartRefresherFailedRefreshKeepsSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	prev := github.DefaultClient
	github.DefaultClient = &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	t.Cleanup(func() { github.DefaultClient = prev })

	c := &cache.Cache{}
	c.Set([]issue.Issue{{
		Title:      "Stale",
		Repository: "kubernetes-sigs/kind",
		HTMLURL:    "https://github.com/kubernetes-sigs/kind/issues/1",
	}}, nil)

	cache.StartRefresher(c, time.Hour)
	h := waitHealth(t, c, "degraded")
	if h.Issues != 1 || h.Error == "" {
		t.Fatalf("health = %+v", h)
	}

	got, _, err := c.Get()
	if err != nil || len(got) != 1 || got[0].Title != "Stale" {
		t.Fatalf("failed refresh must keep last snapshot: %v %v", got, err)
	}
}
