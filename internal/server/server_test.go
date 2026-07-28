package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/issue"
	"k8s-scout/internal/server"
)

type fakeStore struct {
	issues []issue.Issue
	health cache.Health
}

func (f fakeStore) Get() ([]issue.Issue, time.Time, error) {
	out := make([]issue.Issue, len(f.issues))
	copy(out, f.issues)
	return out, time.Now().UTC(), nil
}

func (f fakeStore) HealthSnapshot() cache.Health { return f.health }

func TestHealthz(t *testing.T) {
	srv, err := server.New(fakeStore{health: cache.Health{Status: "ok", Issues: 2}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body cache.Health
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Issues != 2 {
		t.Fatalf("body = %+v", body)
	}
}

func TestIndexDeepLink(t *testing.T) {
	store := fakeStore{issues: []issue.Issue{
		{Title: "Kind docs", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1", Labels: []string{"docs"}},
		{Title: "Other", Repository: "kubernetes-sigs/cluster-api", HTMLURL: "https://example.com/2"},
	}}
	srv, err := server.New(store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?repo=kubernetes-sigs/kind", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Kind docs") || strings.Contains(body, ">Other<") {
		t.Fatalf("unexpected body: %s", body)
	}
	if !strings.Contains(body, `selected>kubernetes-sigs/kind`) {
		t.Fatalf("repo not selected in form")
	}
}

func TestIndexHasCopyURLButton(t *testing.T) {
	srv, err := server.New(fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="copy-filter-url"`) {
		t.Fatal("missing copy filter URL button")
	}
	if !strings.Contains(body, `id="new-since-count"`) || !strings.Contains(body, `id="mark-seen"`) {
		t.Fatal("missing new-since-visit header controls")
	}
}

func TestResultsExposeCreatedAttr(t *testing.T) {
	created := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	store := fakeStore{issues: []issue.Issue{
		{Title: "Fresh", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1", CreatedAt: created},
	}}
	srv, err := server.New(store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `data-created="2025-06-01T12:00:00Z"`) {
		t.Fatalf("missing data-created attr: %s", body)
	}
	if !strings.Contains(body, `data-new-badge`) {
		t.Fatal("missing new badge placeholder")
	}
}

func TestIndexShowsDegradedBanner(t *testing.T) {
	store := fakeStore{
		issues: []issue.Issue{{Title: "One", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1"}},
		health: cache.Health{Status: "degraded", Issues: 1, Error: "GitHub API returned 403 Forbidden"},
	}
	srv, err := server.New(store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Showing cached data") {
		t.Fatal("missing degraded banner")
	}
	if !strings.Contains(body, "403 Forbidden") {
		t.Fatal("missing cache error detail")
	}
}
