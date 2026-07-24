// Package issue defines the shared issue model and helpers.
package issue

import (
	"strings"
	"time"
)

// Issue is a trimmed view of a GitHub search result item.
type Issue struct {
	Title         string
	HTMLURL       string
	Comments      int
	Repository    string // e.g. kubernetes-sigs/kind
	Labels        []string
	LanguageHints []string
	CreatedAt     time.Time
}

// RepoFromURL turns an API repository URL into owner/name.
func RepoFromURL(repositoryURL string) string {
	const prefix = "https://api.github.com/repos/"
	if strings.HasPrefix(repositoryURL, prefix) {
		return strings.TrimPrefix(repositoryURL, prefix)
	}
	parts := strings.Split(repositoryURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return repositoryURL
}

// LanguageHints derives coarse language/tag hints from repo name and labels.
func LanguageHints(repo string, labels []string) []string {
	tokens := tokenize(repo + " " + strings.Join(labels, " "))
	known := []string{"go", "golang", "python", "javascript", "typescript", "rust", "java", "docs", "documentation", "helm", "yaml"}
	var hints []string
	seen := map[string]bool{}
	for _, k := range known {
		if tokens[k] && !seen[k] {
			hints = append(hints, k)
			seen[k] = true
		}
	}
	normalized := make([]string, 0, len(hints))
	seenNorm := map[string]bool{}
	for _, h := range hints {
		n := h
		if h == "golang" {
			n = "go"
		}
		if h == "documentation" {
			n = "docs"
		}
		if !seenNorm[n] {
			normalized = append(normalized, n)
			seenNorm[n] = true
		}
	}
	return normalized
}

func tokenize(s string) map[string]bool {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer("/", " ", "-", " ", "_", " ", ".", " ", ":", " ")
	s = replacer.Replace(s)
	out := map[string]bool{}
	for _, part := range strings.Fields(s) {
		out[part] = true
	}
	return out
}
