package github_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	_, err := client.FetchIssues()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate-limit-remaining=0") {
		t.Fatalf("error %q, want rate-limit-remaining", err.Error())
	}
}

func TestClientSendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 0,
			"items":       []any{},
		})
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{
		HTTP:    srv.Client(),
		BaseURL: srv.URL,
		Token:   "ghs_test_token",
		PerPage: 1,
	}
	_, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer ghs_test_token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestConfigureDefaultFromEnv(t *testing.T) {
	prev := github.DefaultClient.Token
	t.Cleanup(func() { github.DefaultClient.Token = prev })

	t.Setenv("GITHUB_TOKEN", "  ghs_from_env  ")
	if !github.ConfigureDefaultFromEnv() {
		t.Fatal("expected auth enabled")
	}
	if github.DefaultClient.Token != "ghs_from_env" {
		t.Fatalf("token = %q", github.DefaultClient.Token)
	}

	t.Setenv("GITHUB_TOKEN", " \t ")
	if github.ConfigureDefaultFromEnv() {
		t.Fatal("whitespace-only token should disable auth")
	}
	if github.DefaultClient.Token != "" {
		t.Fatalf("token = %q, want empty", github.DefaultClient.Token)
	}
}

func TestClientOmitsAuthorizationWithoutToken(t *testing.T) {
	var gotAuth, gotUA, gotAccept, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "items": []any{}})
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := client.FetchIssues(); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty", gotAuth)
	}
	if gotUA != "k8s-sigs-scout" {
		t.Fatalf("User-Agent = %q", gotUA)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotPerPage != "100" {
		t.Fatalf("per_page = %q, want 100", gotPerPage)
	}
}

func TestClientFetchIssuesHTTPErrorNoRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	_, err := client.FetchIssues()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error %q, want status 500", err.Error())
	}
	if strings.Contains(err.Error(), "rate-limit-remaining") {
		t.Fatalf("error %q should not include rate-limit when header absent", err.Error())
	}
}

func TestClientErrorDoesNotLeakToken(t *testing.T) {
	const token = "ghs_must_not_appear_in_errors"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, Token: token, PerPage: 1}
	_, err := client.FetchIssues()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("token leaked in error: %q", err.Error())
	}
}

func TestClientFetchIssuesBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not-json"))
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	if _, err := client.FetchIssues(); err == nil {
		t.Fatal("expected JSON decode error")
	}
}

func TestClientFetchIssuesDedupesHTMLURL(t *testing.T) {
	dup := map[string]any{
		"title":          "Dup",
		"html_url":       "https://github.com/kubernetes-sigs/kind/issues/1",
		"comments":       0,
		"created_at":     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		"labels":         []map[string]string{{"name": "good first issue"}},
		"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items":       []map[string]any{dup, dup},
		})
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 2}
	got, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Dup" {
		t.Fatalf("got %+v, want one deduped issue", got)
	}
}

func TestClientFetchIssuesTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	srv.Close()
	if _, err := client.FetchIssues(); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestClientFetchIssuesStopsAtMaxPages(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		n := r.URL.Query().Get("page")
		writePage(w, 999, map[string]any{
			"title":          "P" + n,
			"html_url":       "https://github.com/kubernetes-sigs/kind/issues/" + n,
			"comments":       0,
			"created_at":     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			"labels":         []map[string]string{{"name": "good first issue"}},
			"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
		})
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	got, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if pages != 10 {
		t.Fatalf("pages = %d, want 10 (Search API cap)", pages)
	}
	if len(got) != 10 {
		t.Fatalf("len = %d, want 10", len(got))
	}
}

func TestClientFetchIssuesNilHTTPUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "items": []any{}})
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{BaseURL: srv.URL, PerPage: 1}
	got, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestClientFetchIssuesInvalidBaseURL(t *testing.T) {
	client := &github.Client{HTTP: http.DefaultClient, BaseURL: "http://[", PerPage: 1}
	if _, err := client.FetchIssues(); err == nil {
		t.Fatal("expected URL parse error")
	}
}

func TestClientFetchIssuesDedupesAcrossPages(t *testing.T) {
	item := func(title, url string) map[string]any {
		return map[string]any{
			"title":          title,
			"html_url":       url,
			"comments":       0,
			"created_at":     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			"labels":         []map[string]string{{"name": "good first issue"}},
			"repository_url": "https://api.github.com/repos/kubernetes-sigs/kind",
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count": 3,
				"items": []map[string]any{
					item("A", "https://github.com/kubernetes-sigs/kind/issues/1"),
					item("B", "https://github.com/kubernetes-sigs/kind/issues/2"),
				},
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count": 3,
				"items": []map[string]any{
					item("B-dup", "https://github.com/kubernetes-sigs/kind/issues/2"),
					item("C", "https://github.com/kubernetes-sigs/kind/issues/3"),
				},
			})
		default:
			t.Fatalf("unexpected page %q", r.URL.Query().Get("page"))
		}
	}))
	t.Cleanup(srv.Close)

	client := &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 2}
	got, err := client.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Title != "A" || got[1].Title != "B" || got[2].Title != "C" {
		t.Fatalf("got %+v, want A,B,C without cross-page dup", got)
	}
}

func TestFetchIssuesUsesDefaultClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "items": []any{}})
	}))
	t.Cleanup(srv.Close)

	prev := github.DefaultClient
	github.DefaultClient = &github.Client{HTTP: srv.Client(), BaseURL: srv.URL, PerPage: 1}
	t.Cleanup(func() { github.DefaultClient = prev })

	got, err := github.FetchIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func writePage(w http.ResponseWriter, total int, item map[string]any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_count": total,
		"items":       []map[string]any{item},
	})
}
