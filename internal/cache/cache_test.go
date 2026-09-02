package cache_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/github"
	"k8s-scout/internal/issue"
)

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

	got, _, err := c.Get()
	if err == nil || len(got) != 0 {
		t.Fatalf("Get = %v %v", got, err)
	}
	h := c.HealthSnapshot()
	if h.Status != "error" || h.Error == "" {
		t.Fatalf("health = %+v", h)
	}
}
