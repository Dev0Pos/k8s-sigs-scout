package main

import (
	"reflect"
	"testing"
	"time"
)

func TestRepoFromURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "standard api url",
			in:   "https://api.github.com/repos/kubernetes-sigs/kind",
			want: "kubernetes-sigs/kind",
		},
		{
			name: "fallback split",
			in:   "https://example.com/foo/bar/kubernetes-sigs/cluster-api",
			want: "kubernetes-sigs/cluster-api",
		},
		{
			name: "passthrough",
			in:   "already/formatted",
			want: "already/formatted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoFromURL(tt.in)
			if got != tt.want {
				t.Fatalf("repoFromURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLanguageHints(t *testing.T) {
	tests := []struct {
		name   string
		repo   string
		labels []string
		want   []string
	}{
		{
			name:   "golang normalizes to go",
			repo:   "kubernetes-sigs/controller-runtime",
			labels: []string{"language/golang"},
			want:   []string{"go"},
		},
		{
			name:   "documentation normalizes to docs",
			repo:   "kubernetes-sigs/kind",
			labels: []string{"kind/documentation"},
			want:   []string{"docs"},
		},
		{
			name:   "python from label",
			repo:   "kubernetes-sigs/kubespray",
			labels: []string{"python"},
			want:   []string{"python"},
		},
		{
			name:   "helm and yaml from blob",
			repo:   "kubernetes-sigs/helm-charts",
			labels: []string{"area/yaml"},
			want:   []string{"helm", "yaml"},
		},
		{
			name:   "no hints",
			repo:   "kubernetes-sigs/something",
			labels: []string{"good first issue"},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := languageHints(tt.repo, tt.labels)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("languageHints(%q, %v) = %v, want %v", tt.repo, tt.labels, got, tt.want)
			}
		})
	}
}

func TestFilterPath(t *testing.T) {
	tests := []struct {
		q, lang, repo, sortMode, want string
	}{
		{"", "", "", "", "/"},
		{"", "", "", "newest", "/"},
		{"helm", "", "", "", "/?q=helm"},
		{"", "go", "", "", "/?lang=go"},
		{"helm", "go", "", "", "/?lang=go&q=helm"},
		{"", "", "kubernetes-sigs/kind", "", "/?repo=kubernetes-sigs%2Fkind"},
		{"", "", "", "comments", "/?sort=comments"},
		{"x", "go", "kubernetes-sigs/kind", "repo", "/?lang=go&q=x&repo=kubernetes-sigs%2Fkind&sort=repo"},
	}
	for _, tt := range tests {
		got := filterPath(tt.q, tt.lang, tt.repo, tt.sortMode)
		if got != tt.want {
			t.Fatalf("filterPath(%q,%q,%q,%q) = %q, want %q", tt.q, tt.lang, tt.repo, tt.sortMode, got, tt.want)
		}
	}
}

func TestFilterIssues(t *testing.T) {
	issues := []Issue{
		{
			Title:         "Add Go helper",
			Repository:    "kubernetes-sigs/kind",
			Labels:        []string{"good first issue", "language/go"},
			LanguageHints: []string{"go"},
		},
		{
			Title:         "Improve docs for install",
			Repository:    "kubernetes-sigs/cluster-api",
			Labels:        []string{"documentation"},
			LanguageHints: []string{"docs"},
		},
		{
			Title:         "Fix Python script",
			Repository:    "kubernetes-sigs/kubespray",
			Labels:        []string{"python"},
			LanguageHints: []string{"python"},
		},
	}

	tests := []struct {
		name string
		q    string
		lang string
		repo string
		want []string
	}{
		{
			name: "empty filters returns all",
			want: []string{"Add Go helper", "Improve docs for install", "Fix Python script"},
		},
		{
			name: "query by title",
			q:    "helper",
			want: []string{"Add Go helper"},
		},
		{
			name: "filter by repo exact",
			repo: "kubernetes-sigs/kubespray",
			want: []string{"Fix Python script"},
		},
		{
			name: "lang go via hints",
			lang: "go",
			want: []string{"Add Go helper"},
		},
		{
			name: "combined query lang repo",
			q:    "helper",
			lang: "go",
			repo: "kubernetes-sigs/kind",
			want: []string{"Add Go helper"},
		},
		{
			name: "repo mismatch yields empty",
			q:    "helper",
			repo: "kubernetes-sigs/kubespray",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterIssues(issues, tt.q, tt.lang, tt.repo)
			titles := make([]string, 0, len(got))
			for _, issue := range got {
				titles = append(titles, issue.Title)
			}
			if !reflect.DeepEqual(titles, tt.want) {
				t.Fatalf("filterIssues(...) titles = %v, want %v", titles, tt.want)
			}
		})
	}
}

func TestSortIssues(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []Issue{
		{Title: "B", Repository: "kubernetes-sigs/z", Comments: 1, CreatedAt: t1, HTMLURL: "u1"},
		{Title: "A", Repository: "kubernetes-sigs/a", Comments: 5, CreatedAt: t2, HTMLURL: "u2"},
	}

	newest := append([]Issue(nil), issues...)
	sortIssues(newest, "newest")
	if newest[0].Title != "A" {
		t.Fatalf("newest: got %q first", newest[0].Title)
	}

	byComments := append([]Issue(nil), issues...)
	sortIssues(byComments, "comments")
	if byComments[0].Comments != 5 {
		t.Fatalf("comments: got %d first", byComments[0].Comments)
	}

	byRepo := append([]Issue(nil), issues...)
	sortIssues(byRepo, "repo")
	if byRepo[0].Repository != "kubernetes-sigs/a" {
		t.Fatalf("repo: got %q first", byRepo[0].Repository)
	}
}

func TestUniqueRepos(t *testing.T) {
	got := uniqueRepos([]Issue{
		{Repository: "kubernetes-sigs/kind"},
		{Repository: "kubernetes-sigs/cluster-api"},
		{Repository: "kubernetes-sigs/kind"},
	})
	want := []string{"kubernetes-sigs/cluster-api", "kubernetes-sigs/kind"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueRepos = %v, want %v", got, want)
	}
}

func TestCacheGetSet(t *testing.T) {
	c := &Cache{}
	issues := []Issue{{Title: "one", Repository: "kubernetes-sigs/kind"}}
	c.Set(issues, nil)

	got, updatedAt, err := c.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedAt.IsZero() {
		t.Fatal("expected updatedAt to be set")
	}
	if len(got) != 1 || got[0].Title != "one" {
		t.Fatalf("unexpected issues: %+v", got)
	}

	got[0].Title = "mutated"
	again, _, _ := c.Get()
	if again[0].Title != "one" {
		t.Fatal("Get should return a copy, not shared slice")
	}

	h := c.Health()
	if h.Status != "ok" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

func TestCacheHealthDegraded(t *testing.T) {
	c := &Cache{}
	c.Set([]Issue{{Title: "one"}}, nil)
	c.Set(nil, fmtError("boom"))
	h := c.Health()
	if h.Status != "degraded" || h.Error != "boom" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
