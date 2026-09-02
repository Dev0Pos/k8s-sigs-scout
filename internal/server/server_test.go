package server_test

import (
	"encoding/json"
	"errors"
	"fmt"
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
	err    error
}

func (f fakeStore) Get() ([]issue.Issue, time.Time, error) {
	out := make([]issue.Issue, len(f.issues))
	copy(out, f.issues)
	return out, time.Now().UTC(), f.err
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

func TestHealthzErrorUnavailable(t *testing.T) {
	srv, err := server.New(fakeStore{health: cache.Health{Status: "error", Error: "github down"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body cache.Health
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "error" || body.Error != "github down" {
		t.Fatalf("body = %+v", body)
	}
}

func TestHealthzDegradedStaysOK(t *testing.T) {
	srv, err := server.New(fakeStore{health: cache.Health{Status: "degraded", Issues: 3, Error: "timeout"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("degraded healthz should stay 200 for probes, got %d", rec.Code)
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	srv, err := server.New(fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSearchPathFilters(t *testing.T) {
	store := fakeStore{issues: []issue.Issue{
		{Title: "Kind docs", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1"},
		{Title: "Other", Repository: "kubernetes-sigs/cluster-api", HTMLURL: "https://example.com/2"},
	}}
	srv, err := server.New(store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/search?q=Kind", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Kind docs") || strings.Contains(body, ">Other<") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestIndexInvalidPageStaysFirst(t *testing.T) {
	var issues []issue.Issue
	for i := 0; i < 11; i++ {
		issues = append(issues, issue.Issue{
			Title:      fmt.Sprintf("Issue %02d", i),
			Repository: "kubernetes-sigs/kind",
			HTMLURL:    fmt.Sprintf("https://example.com/%d", i),
			CreatedAt:  time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC),
		})
	}
	srv, err := server.New(fakeStore{issues: issues})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?page=not-a-number", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Page 1 / 2") {
		t.Fatalf("invalid page should clamp to 1: %s", body)
	}
}

func TestIndexPaginationLinks(t *testing.T) {
	var issues []issue.Issue
	for i := 0; i < 11; i++ {
		issues = append(issues, issue.Issue{
			Title:      fmt.Sprintf("Issue %02d", i),
			Repository: "kubernetes-sigs/kind",
			HTMLURL:    fmt.Sprintf("https://example.com/%d", i),
			CreatedAt:  time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC),
		})
	}
	srv, err := server.New(fakeStore{issues: issues})
	if err != nil {
		t.Fatal(err)
	}

	page1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(page1, httptest.NewRequest(http.MethodGet, "/", nil))
	b1 := page1.Body.String()
	if !strings.Contains(b1, `hx-get="/?page=2"`) {
		t.Fatalf("missing next page link: %s", b1)
	}
	if !strings.Contains(b1, "Issue 10") || strings.Contains(b1, ">Issue 00<") {
		t.Fatalf("page 1 should be newest 10: %s", b1)
	}

	page2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(page2, httptest.NewRequest(http.MethodGet, "/?page=2", nil))
	b2 := page2.Body.String()
	if !strings.Contains(b2, ">Issue 00<") {
		t.Fatalf("page 2 should include oldest issue: %s", b2)
	}
	if !strings.Contains(b2, `hx-get="/"`) {
		t.Fatalf("missing prev page link: %s", b2)
	}
}

func TestIndexSortComments(t *testing.T) {
	store := fakeStore{issues: []issue.Issue{
		{Title: "Quiet", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1", Comments: 0},
		{Title: "Hot", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/2", Comments: 9},
	}}
	srv, err := server.New(store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?sort=comments", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	hot := strings.Index(body, ">Hot<")
	quiet := strings.Index(body, ">Quiet<")
	if hot < 0 || quiet < 0 || hot > quiet {
		t.Fatalf("comments sort order: hot=%d quiet=%d body=%s", hot, quiet, body)
	}
	if !strings.Contains(body, `selected>Most comments`) {
		t.Fatal("sort select should mark comments")
	}
}

func TestIndexHXRequestReturnsFragment(t *testing.T) {
	srv, err := server.New(fakeStore{issues: []issue.Issue{
		{Title: "One", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	full := httptest.NewRecorder()
	srv.Handler().ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(full.Body.String(), "<!DOCTYPE html>") {
		t.Fatal("full page should be a document")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	partial := httptest.NewRecorder()
	srv.Handler().ServeHTTP(partial, req)
	body := partial.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") || strings.Contains(body, "<html") {
		t.Fatalf("HX fragment should not be a full document: %s", body)
	}
	if !strings.Contains(body, ">One<") {
		t.Fatalf("missing issue in fragment: %s", body)
	}
}

func TestIndexCacheErrorEmpty(t *testing.T) {
	c := &cache.Cache{}
	c.Set(nil, errors.New("github 403"))
	srv, err := server.New(c)
	if err != nil {
		t.Fatal(err)
	}

	page := httptest.NewRecorder()
	srv.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(page.Body.String(), "Failed to load issues: github 403") {
		t.Fatalf("missing load error: %s", page.Body.String())
	}

	health := httptest.NewRecorder()
	srv.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz = %d, want 503", health.Code)
	}
}

func TestIndexCacheDegradedKeepsIssues(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "Stale", Repository: "kubernetes-sigs/kind", HTMLURL: "https://example.com/1"}}, nil)
	c.Set(nil, errors.New("github 403"))
	srv, err := server.New(c)
	if err != nil {
		t.Fatal(err)
	}

	page := httptest.NewRecorder()
	srv.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	body := page.Body.String()
	if !strings.Contains(body, ">Stale<") {
		t.Fatalf("stale issues missing: %s", body)
	}
	if !strings.Contains(body, "Showing cached data") {
		t.Fatal("missing degraded banner")
	}
	if strings.Contains(body, "Failed to load issues") {
		t.Fatal("degraded cache should not show empty-load error")
	}

	health := httptest.NewRecorder()
	srv.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("degraded healthz = %d, want 200", health.Code)
	}
}
