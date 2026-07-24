package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

const (
	githubSearchQ = `org:kubernetes-sigs is:issue is:open label:"good first issue" no:assignee`
	perPage       = 100
	maxPages      = 10 // GitHub Search API caps around 1000 results
	cacheInterval = 15 * time.Minute
	k8sBlue       = "#326ce5"
	defaultSort   = "newest"
)

// Issue is a trimmed view of a GitHub search result item.
type Issue struct {
	Title         string
	HTMLURL       string
	Comments      int
	Repository    string // e.g. kubernetes-sigs/kind
	Labels        []string
	LanguageHints []string // derived from repo name / labels for filtering
	CreatedAt     time.Time
}

type githubSearchResponse struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		Title     string    `json:"title"`
		HTMLURL   string    `json:"html_url"`
		Comments  int       `json:"comments"`
		CreatedAt time.Time `json:"created_at"`
		Labels    []struct {
			Name string `json:"name"`
		} `json:"labels"`
		RepositoryURL string `json:"repository_url"`
	} `json:"items"`
}

type Cache struct {
	mu        sync.RWMutex
	issues    []Issue
	updatedAt time.Time
	err       error  // set when cache is empty after a failed refresh
	lastErr   string // last refresh failure message (kept even if stale data remains)
}

func (c *Cache) Get() ([]Issue, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Issue, len(c.issues))
	copy(out, c.issues)
	return out, c.updatedAt, c.err
}

func (c *Cache) Set(issues []Issue, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.issues = issues
		c.updatedAt = time.Now().UTC()
		c.err = nil
		c.lastErr = ""
		return
	}
	c.lastErr = err.Error()
	if len(c.issues) == 0 {
		c.err = err
	}
}

type healthResponse struct {
	Status     string `json:"status"`
	Issues     int    `json:"issues"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (c *Cache) Health() healthResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	h := healthResponse{Issues: len(c.issues)}
	if !c.updatedAt.IsZero() {
		h.UpdatedAt = c.updatedAt.Format(time.RFC3339)
		h.AgeSeconds = int64(time.Since(c.updatedAt).Seconds())
	}
	if c.lastErr != "" {
		h.Error = c.lastErr
	}
	switch {
	case len(c.issues) == 0 && c.err != nil:
		h.Status = "error"
	case c.lastErr != "" && len(c.issues) > 0:
		h.Status = "degraded"
	case len(c.issues) == 0:
		h.Status = "starting"
	default:
		h.Status = "ok"
	}
	return h
}

func fetchIssues() ([]Issue, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []Issue
	seen := map[string]bool{}

	for page := 1; page <= maxPages; page++ {
		u, err := url.Parse("https://api.github.com/search/issues")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("q", githubSearchQ)
		q.Set("per_page", strconv.Itoa(perPage))
		q.Set("page", strconv.Itoa(page))
		q.Set("sort", "created")
		q.Set("order", "desc")
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "k8s-sigs-scout")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned %s (page %d)", resp.Status, page)
		}

		var payload githubSearchResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if len(payload.Items) == 0 {
			break
		}

		for _, item := range payload.Items {
			if seen[item.HTMLURL] {
				continue
			}
			seen[item.HTMLURL] = true
			repo := repoFromURL(item.RepositoryURL)
			labels := make([]string, 0, len(item.Labels))
			for _, l := range item.Labels {
				labels = append(labels, l.Name)
			}
			all = append(all, Issue{
				Title:         item.Title,
				HTMLURL:       item.HTMLURL,
				Comments:      item.Comments,
				Repository:    repo,
				Labels:        labels,
				LanguageHints: languageHints(repo, labels),
				CreatedAt:     item.CreatedAt,
			})
		}

		log.Printf("fetched page %d: +%d items (cache so far %d / reported total %d)",
			page, len(payload.Items), len(all), payload.TotalCount)

		if len(payload.Items) < perPage || len(all) >= payload.TotalCount {
			break
		}
	}

	return all, nil
}

func repoFromURL(repositoryURL string) string {
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

func languageHints(repo string, labels []string) []string {
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

func startCacheRefresher(cache *Cache) {
	refresh := func() {
		issues, err := fetchIssues()
		if err != nil {
			log.Printf("cache refresh failed: %v", err)
			cache.Set(nil, err)
			return
		}
		cache.Set(issues, nil)
		log.Printf("cache refreshed: %d issues", len(issues))
	}

	refresh()
	go func() {
		ticker := time.NewTicker(cacheInterval)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()
}

func uniqueRepos(issues []Issue) []string {
	seen := map[string]bool{}
	var repos []string
	for _, issue := range issues {
		if issue.Repository == "" || seen[issue.Repository] {
			continue
		}
		seen[issue.Repository] = true
		repos = append(repos, issue.Repository)
	}
	sort.Strings(repos)
	return repos
}

func filterIssues(issues []Issue, q, lang, repo string) []Issue {
	q = strings.TrimSpace(strings.ToLower(q))
	lang = strings.TrimSpace(strings.ToLower(lang))
	repo = strings.TrimSpace(repo)

	if q == "" && lang == "" && repo == "" {
		out := make([]Issue, len(issues))
		copy(out, issues)
		return out
	}

	out := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if repo != "" && issue.Repository != repo {
			continue
		}
		if lang != "" {
			matchLang := false
			for _, h := range issue.LanguageHints {
				if h == lang {
					matchLang = true
					break
				}
			}
			if !matchLang {
				blob := strings.ToLower(issue.Repository + " " + strings.Join(issue.Labels, " "))
				if !strings.Contains(blob, lang) {
					continue
				}
			}
		}
		if q != "" {
			blob := strings.ToLower(issue.Title + " " + issue.Repository + " " + strings.Join(issue.Labels, " "))
			if !strings.Contains(blob, q) {
				continue
			}
		}
		out = append(out, issue)
	}
	return out
}

func normalizeSort(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "comments", "repo", "title", "newest":
		return strings.TrimSpace(strings.ToLower(s))
	default:
		return defaultSort
	}
}

func sortIssues(issues []Issue, mode string) {
	mode = normalizeSort(mode)
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

// filterPath builds a shareable deep-link path like /?q=helm&lang=go&repo=...&sort=comments
func filterPath(q, lang, repo, sortMode string) string {
	q = strings.TrimSpace(q)
	lang = strings.TrimSpace(lang)
	repo = strings.TrimSpace(repo)
	sortMode = normalizeSort(sortMode)
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
	if sortMode != defaultSort {
		v.Set("sort", sortMode)
	}
	if len(v) == 0 {
		return "/"
	}
	return "/?" + v.Encode()
}

type pageData struct {
	Issues    []Issue
	Repos     []string
	Query     string
	Lang      string
	Repo      string
	Sort      string
	UpdatedAt string
	Count     int
	Total     int
	Error     string
	K8sBlue   string
}

func buildPageData(cache *Cache, q, lang, repo, sortMode string) pageData {
	issues, updatedAt, err := cache.Get()
	sortMode = normalizeSort(sortMode)
	filtered := filterIssues(issues, q, lang, repo)
	sortIssues(filtered, sortMode)
	data := pageData{
		Issues:  filtered,
		Repos:   uniqueRepos(issues),
		Query:   q,
		Lang:    lang,
		Repo:    repo,
		Sort:    sortMode,
		Count:   len(filtered),
		Total:   len(issues),
		K8sBlue: k8sBlue,
	}
	if !updatedAt.IsZero() {
		data.UpdatedAt = updatedAt.Format(time.RFC822)
	}
	if err != nil && len(issues) == 0 {
		data.Error = err.Error()
	}
	return data
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	tmpl := template.Must(template.ParseFS(templateFS,
		"templates/index.html",
		"templates/results.html",
	))

	cache := &Cache{}
	startCacheRefresher(cache)

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		h := cache.Health()
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusOK
		if h.Status == "error" {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(h)
	})

	render := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		lang := r.URL.Query().Get("lang")
		repo := r.URL.Query().Get("repo")
		sortMode := r.URL.Query().Get("sort")
		data := buildPageData(cache, q, lang, repo, sortMode)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Header.Get("HX-Request") == "true" {
			if err := tmpl.ExecuteTemplate(w, "results.html", data); err != nil {
				log.Printf("template error: %v", err)
				http.Error(w, "template error", http.StatusInternalServerError)
			}
			return
		}
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("template error: %v", err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		render(w, r)
	})

	mux.HandleFunc("/search", render)

	addr := ":" + port
	log.Printf("k8s-sigs-scout listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
