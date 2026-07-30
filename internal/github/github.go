// Package github fetches good-first-issue search results from the GitHub API.
package github

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s-scout/internal/issue"
)

const (
	searchQuery    = `org:kubernetes-sigs is:issue is:open label:"good first issue" no:assignee`
	perPage        = 100
	maxPages       = 10 // GitHub Search API caps around 1000 results
	userAgent      = "k8s-sigs-scout"
	defaultBaseURL = "https://api.github.com"
)

// Client talks to the GitHub Search API.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	// Token is an optional GitHub PAT / fine-grained token (Authorization: Bearer).
	// Without it, unauthenticated Search limits apply (~60 req/h per IP).
	Token string
	// PerPage overrides the default page size (useful in tests).
	PerPage int
}

// DefaultClient is used by FetchIssues.
var DefaultClient = &Client{
	HTTP:    &http.Client{Timeout: 30 * time.Second},
	BaseURL: defaultBaseURL,
}

// ConfigureDefaultFromEnv sets DefaultClient.Token from GITHUB_TOKEN if present.
// Returns whether auth is enabled (token non-empty). Does not log the token.
func ConfigureDefaultFromEnv() bool {
	tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	DefaultClient.Token = tok
	return tok != ""
}

type searchResponse struct {
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

// FetchIssues loads all paginated good-first-issue results using DefaultClient.
func FetchIssues() ([]issue.Issue, error) {
	return DefaultClient.FetchIssues()
}

// FetchIssues loads all paginated good-first-issue results for kubernetes-sigs.
func (c *Client) FetchIssues() ([]issue.Issue, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	pageSize := c.PerPage
	if pageSize <= 0 {
		pageSize = perPage
	}

	var all []issue.Issue
	seen := map[string]bool{}

	for page := 1; page <= maxPages; page++ {
		u, err := url.Parse(base + "/search/issues")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("q", searchQuery)
		q.Set("per_page", strconv.Itoa(pageSize))
		q.Set("page", strconv.Itoa(page))
		q.Set("sort", "created")
		q.Set("order", "desc")
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", userAgent)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			remaining := resp.Header.Get("X-RateLimit-Remaining")
			_ = resp.Body.Close()
			if remaining != "" {
				return nil, fmt.Errorf("GitHub API returned %s (page %d, rate-limit-remaining=%s)", resp.Status, page, remaining)
			}
			return nil, fmt.Errorf("GitHub API returned %s (page %d)", resp.Status, page)
		}

		var payload searchResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()
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
			repo := issue.RepoFromURL(item.RepositoryURL)
			labels := make([]string, 0, len(item.Labels))
			for _, l := range item.Labels {
				labels = append(labels, l.Name)
			}
			all = append(all, issue.Issue{
				Title:         item.Title,
				HTMLURL:       item.HTMLURL,
				Comments:      item.Comments,
				Repository:    repo,
				Labels:        labels,
				LanguageHints: issue.LanguageHints(repo, labels),
				CreatedAt:     item.CreatedAt,
			})
		}

		log.Printf("fetched page %d: +%d items (cache so far %d / reported total %d)",
			page, len(payload.Items), len(all), payload.TotalCount)

		if len(payload.Items) < pageSize || len(all) >= payload.TotalCount {
			break
		}
	}

	return all, nil
}
