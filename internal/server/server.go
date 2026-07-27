// Package server serves the HTMX dashboard and health endpoint.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/filter"
	"k8s-scout/internal/issue"
)

const k8sBlue = "#326ce5"

//go:embed templates/*.html
var templateFS embed.FS

// Store is the cache surface needed by HTTP handlers.
type Store interface {
	Get() ([]issue.Issue, time.Time, error)
	HealthSnapshot() cache.Health
}

// Server serves the dashboard and health endpoint.
type Server struct {
	store Store
	tmpl  *template.Template
}

// New constructs a Server with parsed templates.
func New(store Store) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS,
		"templates/index.html",
		"templates/results.html",
	)
	if err != nil {
		return nil, err
	}
	return &Server{store: store, tmpl: tmpl}, nil
}

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/search", s.handleIndex)
	return mux
}

type pageData struct {
	Issues      []issue.Issue
	Repos       []string
	Query       string
	Lang        string
	Repo        string
	Sort        string
	UpdatedAt   string
	Count       int // issues on this page
	Matched     int // after filters, before UI pagination
	Total       int // full cache size
	Page        int
	Pages       int
	From        int
	To          int
	HasPrev     bool
	HasNext     bool
	PrevURL     string
	NextURL     string
	Error       string
	CacheStatus string
	CacheError  string
	K8sBlue     string
}

func (s *Server) buildPageData(q, lang, repo, sortMode string, page int) pageData {
	issues, updatedAt, err := s.store.Get()
	health := s.store.HealthSnapshot()
	sortMode = filter.NormalizeSort(sortMode)
	filtered := filter.Issues(issues, q, lang, repo)
	filter.SortIssues(filtered, sortMode)
	pageIssues, info := filter.Paginate(filtered, page)

	data := pageData{
		Issues:      pageIssues,
		Repos:       filter.UniqueRepos(issues),
		Query:       q,
		Lang:        lang,
		Repo:        repo,
		Sort:        sortMode,
		Count:       len(pageIssues),
		Matched:     info.Matched,
		Total:       len(issues),
		Page:        info.Page,
		Pages:       info.Pages,
		From:        info.From,
		To:          info.To,
		HasPrev:     info.Page > 1,
		HasNext:     info.Page < info.Pages,
		PrevURL:     filter.Path(q, lang, repo, sortMode, info.Page-1),
		NextURL:     filter.Path(q, lang, repo, sortMode, info.Page+1),
		CacheStatus: health.Status,
		CacheError:  health.Error,
		K8sBlue:     k8sBlue,
	}
	if !updatedAt.IsZero() {
		data.UpdatedAt = updatedAt.Format(time.RFC822)
	}
	if err != nil && len(issues) == 0 {
		data.Error = err.Error()
	}
	return data
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	h := s.store.HealthSnapshot()
	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if h.Status == "error" {
		status = http.StatusServiceUnavailable
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(h)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/search" {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query().Get("q")
	lang := r.URL.Query().Get("lang")
	repo := r.URL.Query().Get("repo")
	sortMode := r.URL.Query().Get("sort")
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			page = n
		}
	}
	data := s.buildPageData(q, lang, repo, sortMode, page)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	name := "index.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "results.html"
	}
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
