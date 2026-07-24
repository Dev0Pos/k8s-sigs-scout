package issue_test

import (
	"reflect"
	"testing"

	"k8s-scout/internal/issue"
)

func TestRepoFromURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"standard api url", "https://api.github.com/repos/kubernetes-sigs/kind", "kubernetes-sigs/kind"},
		{"fallback split", "https://example.com/foo/bar/kubernetes-sigs/cluster-api", "kubernetes-sigs/cluster-api"},
		{"passthrough", "already/formatted", "already/formatted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := issue.RepoFromURL(tt.in); got != tt.want {
				t.Fatalf("RepoFromURL(%q) = %q, want %q", tt.in, got, tt.want)
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
		{"golang normalizes to go", "kubernetes-sigs/controller-runtime", []string{"language/golang"}, []string{"go"}},
		{"documentation normalizes to docs", "kubernetes-sigs/kind", []string{"kind/documentation"}, []string{"docs"}},
		{"python from label", "kubernetes-sigs/kubespray", []string{"python"}, []string{"python"}},
		{"helm and yaml from blob", "kubernetes-sigs/helm-charts", []string{"area/yaml"}, []string{"helm", "yaml"}},
		{"no hints", "kubernetes-sigs/something", []string{"good first issue"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := issue.LanguageHints(tt.repo, tt.labels)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("LanguageHints(%q, %v) = %v, want %v", tt.repo, tt.labels, got, tt.want)
			}
		})
	}
}
