package github_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s-scout/internal/github"
)

func TestClientFetchIssuesPaginated(t *testing.T) {
	var pagesHit []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			http.NotFound(w, r)
			return
		}
		page := r.URL.Query().Get("page")
		pagesHit = append(pagesHit, page)
		if got := r.URL.Query().Get("per_page"); got != "1" {
			t.Errorf("per_page = %q, want 1", got)
		}

		w.Header().Set("Content-Type", "application/json")
		created := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		switch page {
		case "1":
			writePage(w, 2, map[string]any{
				"title":          "First",
				"html_url":       "https://github.com/kubernetes-sigs/kind/issues/1",
				"comments":       3,
				"created_at":     created,
				"labels":         []map[string]string{{"name": "good first issue"}},
				"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
			})
		case "2":
			writePage(w, 2, map[string]any{
				"title":          "Second",
				"html_url":       "https://github.com/kubernetes-sigs/kind/issues/2",
				"comments":       0,
				"created_at":     created,
				"labels":         []map[string]string{{"name": "good first issue"}, {"name": "python"}},
				"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
			})
		default:
			t.Fatalf("unexpected page %q", page)
		}
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{
		HTTP:    srv.Client(),
		BaseURL: srv.URL,
		PerPage: 1,
	}
	got, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(pagesHit) != 2 || pagesHit[0] != "1" || pagesHit[1] != "2" {
		t.Fatalf("pagesHit = %v", pagesHit)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "First" || got[0].Repository != "kubernetes-sigs/kind" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Title != "Second" || len(got[1].LanguageHints) == 0 || got[1].LanguageHints[0] != "python" {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestClientFetchIssuesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	_, err := client.FetchIssues()
	if err == nil {
		t.Fatal("expected error")
	}
}

func writePage(w http.ResponseWriter, total int, item map[string]any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_count": total,
		"items":       []map[string]any{item},
	})
}
