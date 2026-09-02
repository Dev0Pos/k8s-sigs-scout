// Package filter provides in-memory filtering, sorting, and deep-link helpers.
package filter

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"k8s-scout/internal/issue"
)

// DefaultSort is used when no sort query param is provided.
const DefaultSort = "newest"

// PageSize is the maximum number of issues shown per UI page.
const PageSize = 10

// PageInfo describes the current UI pagination window.
type PageInfo struct {
	Page    int
	Pages   int
	Size    int
	Matched int
	From    int // 1-based inclusive, 0 if empty
	To      int // 1-based inclusive, 0 if empty
}

// Paginate returns a single page of issues and metadata.
func Paginate(issues []issue.Issue, page int) ([]issue.Issue, PageInfo) {
	matched := len(issues)
	info := PageInfo{Page: 1, Pages: 1, Size: PageSize, Matched: matched}
	if matched == 0 {
		return nil, info
	}
	pages := (matched + PageSize - 1) / PageSize
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * PageSize
	end := start + PageSize
	if end > matched {
		end = matched
	}
	info.Page = page
	info.Pages = pages
	info.From = start + 1
	info.To = end
	out := make([]issue.Issue, end-start)
	copy(out, issues[start:end])
	return out, info
}

// UniqueRepos returns sorted distinct repository names.
func UniqueRepos(issues []issue.Issue) []string {
	seen := map[string]bool{}
	var repos []string
	for _, iss := range issues {
		if iss.Repository == "" || seen[iss.Repository] {
			continue
		}
		seen[iss.Repository] = true
		repos = append(repos, iss.Repository)
	}
	sort.Strings(repos)
	return repos
}

// Issues filters by free-text query, language/tag hint, and exact repository.
func Issues(issues []issue.Issue, q, lang, repo string) []issue.Issue {
	q = strings.TrimSpace(strings.ToLower(q))
	lang = strings.TrimSpace(strings.ToLower(lang))
	repo = strings.TrimSpace(repo)

	if q == "" && lang == "" && repo == "" {
		out := make([]issue.Issue, len(issues))
		copy(out, issues)
		return out
	}

	out := make([]issue.Issue, 0, len(issues))
	for _, iss := range issues {
		if repo != "" && iss.Repository != repo {
			continue
		}
		if lang != "" {
			matchLang := false
			for _, h := range iss.LanguageHints {
				if h == lang {
					matchLang = true
					break
				}
			}
			if !matchLang {
				continue
			}
		}
		if q != "" {
			blob := strings.ToLower(iss.Title + " " + iss.Repository + " " + strings.Join(iss.Labels, " "))
			if !strings.Contains(blob, q) {
				continue
			}
		}
		out = append(out, iss)
	}
	return out
}

// NormalizeSort returns a known sort mode or DefaultSort.
func NormalizeSort(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "comments", "repo", "title", "newest":
		return strings.TrimSpace(strings.ToLower(s))
	default:
		return DefaultSort
	}
}

// SortIssues sorts issues in place by mode.
func SortIssues(issues []issue.Issue, mode string) {
	mode = NormalizeSort(mode)
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		switch mode {
		case "comments":
			if a.Comments != b.Comments {
				return a.Comments > b.Comments
			}
		case "repo":
			if a.Repository != b.Repository {
				return a.Repository < b.Repository
			}
		case "title":
			if !strings.EqualFold(a.Title, b.Title) {
				return strings.ToLower(a.Title) < strings.ToLower(b.Title)
			}
		default: // newest
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
		}
		return a.HTMLURL < b.HTMLURL
	})
}

// Path builds a shareable deep-link path like /?q=helm&lang=go&page=2.
func Path(q, lang, repo, sortMode string, page int) string {
	q = strings.TrimSpace(q)
	lang = strings.TrimSpace(lang)
	repo = strings.TrimSpace(repo)
	sortMode = NormalizeSort(sortMode)
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if lang != "" {
		v.Set("lang", lang)
	}
	if repo != "" {
		v.Set("repo", repo)
	}
	if sortMode != DefaultSort {
		v.Set("sort", sortMode)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return "/"
	}
	return "/?" + v.Encode()
}
