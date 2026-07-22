package main

import (
	"reflect"
	"testing"
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
		q, lang, want string
	}{
		{"", "", "/"},
		{"  ", "  ", "/"},
		{"helm", "", "/?q=helm"},
		{"", "go", "/?lang=go"},
		{"helm", "go", "/?lang=go&q=helm"},
	}
	for _, tt := range tests {
		got := filterPath(tt.q, tt.lang)
		if got != tt.want {
			t.Fatalf("filterPath(%q, %q) = %q, want %q", tt.q, tt.lang, got, tt.want)
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
		want []string // titles
	}{
		{
			name: "empty filters returns all",
			q:    "",
			lang: "",
			want: []string{"Add Go helper", "Improve docs for install", "Fix Python script"},
		},
		{
			name: "query by title",
			q:    "helper",
			lang: "",
			want: []string{"Add Go helper"},
		},
		{
			name: "query by repo",
			q:    "kubespray",
			lang: "",
			want: []string{"Fix Python script"},
		},
		{
			name: "lang go via hints",
			q:    "",
			lang: "go",
			want: []string{"Add Go helper"},
		},
		{
			name: "lang docs via hints",
			q:    "",
			lang: "docs",
			want: []string{"Improve docs for install"},
		},
		{
			name: "combined query and lang",
			q:    "fix",
			lang: "python",
			want: []string{"Fix Python script"},
		},
		{
			name: "no matches",
			q:    "nonexistent",
			lang: "",
			want: []string{},
		},
		{
			name: "whitespace query ignored as empty with lang",
			q:    "   ",
			lang: "python",
			want: []string{"Fix Python script"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterIssues(issues, tt.q, tt.lang)
			titles := make([]string, 0, len(got))
			for _, issue := range got {
				titles = append(titles, issue.Title)
			}
			if !reflect.DeepEqual(titles, tt.want) {
				t.Fatalf("filterIssues(q=%q, lang=%q) titles = %v, want %v", tt.q, tt.lang, titles, tt.want)
			}
		})
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
}
