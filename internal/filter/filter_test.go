package filter_test

import (
	"reflect"
	"testing"
	"time"

	"k8s-scout/internal/filter"
	"k8s-scout/internal/issue"
)

func TestPath(t *testing.T) {
	tests := []struct {
		q, lang, repo, sortMode string
		page                    int
		want                    string
	}{
		{"", "", "", "", 1, "/"},
		{"", "", "", "newest", 1, "/"},
		{"helm", "", "", "", 1, "/?q=helm"},
		{"", "go", "", "", 1, "/?lang=go"},
		{"helm", "go", "", "", 1, "/?lang=go&q=helm"},
		{"", "", "kubernetes-sigs/kind", "", 1, "/?repo=kubernetes-sigs%2Fkind"},
		{"", "", "", "comments", 1, "/?sort=comments"},
		{"", "", "", "", 2, "/?page=2"},
		{"x", "go", "kubernetes-sigs/kind", "repo", 3, "/?lang=go&page=3&q=x&repo=kubernetes-sigs%2Fkind&sort=repo"},
		{"  helm  ", "  go  ", "  kubernetes-sigs/kind  ", " NEWEST ", 1, "/?lang=go&q=helm&repo=kubernetes-sigs%2Fkind"},
		{"", "", "", "nope", 0, "/"},
		{"a&b=c", "", "", "", 1, "/?q=a%26b%3Dc"},
		{"hello world", "", "", "", 1, "/?q=hello+world"},
	}
	for _, tt := range tests {
		got := filter.Path(tt.q, tt.lang, tt.repo, tt.sortMode, tt.page)
		if got != tt.want {
			t.Fatalf("Path(...) = %q, want %q", got, tt.want)
		}
	}
}

func TestPaginate(t *testing.T) {
	var issues []issue.Issue
	for i := 0; i < 25; i++ {
		issues = append(issues, issue.Issue{Title: string(rune('A' + i%26)), HTMLURL: string(rune('a' + i%26))})
	}
	page1, info := filter.Paginate(issues, 1)
	if len(page1) != 10 || info.Pages != 3 || info.From != 1 || info.To != 10 {
		t.Fatalf("page1: len=%d info=%+v", len(page1), info)
	}
	page3, info3 := filter.Paginate(issues, 3)
	if len(page3) != 5 || info3.From != 21 || info3.To != 25 {
		t.Fatalf("page3: len=%d info=%+v", len(page3), info3)
	}
	clamped, infoC := filter.Paginate(issues, 99)
	if infoC.Page != 3 || len(clamped) != 5 {
		t.Fatalf("clamp: len=%d info=%+v", len(clamped), infoC)
	}
	low, infoLow := filter.Paginate(issues, 0)
	if infoLow.Page != 1 || len(low) != 10 {
		t.Fatalf("page<1: len=%d info=%+v", len(low), infoLow)
	}
	empty, infoE := filter.Paginate(nil, 3)
	if empty != nil || infoE.Page != 1 || infoE.Pages != 1 || infoE.From != 0 || infoE.To != 0 || infoE.Matched != 0 {
		t.Fatalf("empty: %v info=%+v", empty, infoE)
	}
	page1[0].Title = "mutated"
	if issues[0].Title == "mutated" {
		t.Fatal("Paginate should return a copy")
	}
}

func TestIssues(t *testing.T) {
	issues := []issue.Issue{
		{Title: "Add Go helper", Repository: "kubernetes-sigs/kind", Labels: []string{"good first issue"}, LanguageHints: []string{"go"}},
		{Title: "Improve docs for install", Repository: "kubernetes-sigs/cluster-api", Labels: []string{"documentation"}, LanguageHints: []string{"docs"}},
		{Title: "Fix Python script", Repository: "kubernetes-sigs/kubespray", Labels: []string{"python"}, LanguageHints: []string{"python"}},
	}

	got := filter.Issues(issues, "helper", "go", "kubernetes-sigs/kind")
	if len(got) != 1 || got[0].Title != "Add Go helper" {
		t.Fatalf("unexpected filter result: %+v", got)
	}

	empty := filter.Issues(issues, "helper", "", "kubernetes-sigs/kubespray")
	if len(empty) != 0 {
		t.Fatalf("expected no matches, got %+v", empty)
	}

	// lang matches LanguageHints only — repo/label substrings must not leak in.
	viaRepoName := filter.Issues(issues, "", "kubespray", "")
	if len(viaRepoName) != 0 {
		t.Fatalf("lang must not match repository name, got %+v", viaRepoName)
	}

	viaLabel := filter.Issues(issues, "documentation", "", "")
	if len(viaLabel) != 1 || viaLabel[0].Title != "Improve docs for install" {
		t.Fatalf("q should match label blob, got %+v", viaLabel)
	}

	copied := filter.Issues(issues, "  ", "\t", "")
	if len(copied) != 3 {
		t.Fatalf("blank filters should copy all, got %+v", copied)
	}
	copied[0].Title = "mutated"
	if issues[0].Title != "Add Go helper" {
		t.Fatal("empty filter should not alias input")
	}

	viaLangCase := filter.Issues(issues, "", "GO", "")
	if len(viaLangCase) != 1 || viaLangCase[0].Title != "Add Go helper" {
		t.Fatalf("lang must fold case, got %+v", viaLangCase)
	}

	viaTitleCase := filter.Issues(issues, "HELPER", "", "")
	if len(viaTitleCase) != 1 || viaTitleCase[0].Title != "Add Go helper" {
		t.Fatalf("q must match title case-insensitively, got %+v", viaTitleCase)
	}

	viaRepoBlob := filter.Issues(issues, "KIND", "", "")
	if len(viaRepoBlob) != 1 || viaRepoBlob[0].Repository != "kubernetes-sigs/kind" {
		t.Fatalf("q must match repository blob, got %+v", viaRepoBlob)
	}

	shortRepo := filter.Issues(issues, "", "", "kind")
	if len(shortRepo) != 0 {
		t.Fatalf("repo must be exact owner/name, got %+v", shortRepo)
	}

	wrongCaseRepo := filter.Issues(issues, "", "", "kubernetes-sigs/Kind")
	if len(wrongCaseRepo) != 0 {
		t.Fatalf("repo must be case-sensitive, got %+v", wrongCaseRepo)
	}
}

func TestIssuesLangGoDoesNotMatchGoodFirstIssueLabel(t *testing.T) {
	issues := []issue.Issue{
		{
			Title:         "Python fix",
			Repository:    "kubernetes-sigs/kubespray",
			Labels:        []string{"good first issue", "python"},
			LanguageHints: []string{"python"},
		},
		{
			Title:         "Go helper",
			Repository:    "kubernetes-sigs/kind",
			Labels:        []string{"good first issue"},
			LanguageHints: []string{"go"},
		},
		{
			Title:      "Untagged cluster-api task",
			Repository: "kubernetes-sigs/cluster-api",
			Labels:     []string{"good first issue"},
		},
	}

	got := filter.Issues(issues, "", "go", "")
	if len(got) != 1 || got[0].Title != "Go helper" {
		t.Fatalf("lang=go should only match LanguageHints go, got %+v", got)
	}
}

func TestIssuesLangJavaDoesNotMatchJavascript(t *testing.T) {
	issues := []issue.Issue{
		{
			Title:         "JS",
			Repository:    "kubernetes-sigs/js-app",
			Labels:        []string{"good first issue", "javascript"},
			LanguageHints: []string{"javascript"},
		},
		{
			Title:         "Java",
			Repository:    "kubernetes-sigs/java-app",
			Labels:        []string{"good first issue", "java"},
			LanguageHints: []string{"java"},
		},
	}

	got := filter.Issues(issues, "", "java", "")
	if len(got) != 1 || got[0].Title != "Java" {
		t.Fatalf("lang=java should not match javascript, got %+v", got)
	}
}

func TestSortIssues(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []issue.Issue{
		{Title: "B", Repository: "kubernetes-sigs/z", Comments: 1, CreatedAt: t1, HTMLURL: "u1"},
		{Title: "A", Repository: "kubernetes-sigs/a", Comments: 5, CreatedAt: t2, HTMLURL: "u2"},
	}

	newest := append([]issue.Issue(nil), issues...)
	filter.SortIssues(newest, "newest")
	if newest[0].Title != "A" {
		t.Fatalf("newest: got %q", newest[0].Title)
	}

	byComments := append([]issue.Issue(nil), issues...)
	filter.SortIssues(byComments, "comments")
	if byComments[0].Comments != 5 {
		t.Fatalf("comments: got %d", byComments[0].Comments)
	}

	byRepo := append([]issue.Issue(nil), issues...)
	filter.SortIssues(byRepo, "repo")
	if byRepo[0].Repository != "kubernetes-sigs/a" {
		t.Fatalf("repo: got %q", byRepo[0].Repository)
	}

	byTitle := append([]issue.Issue(nil), issues...)
	filter.SortIssues(byTitle, "TITLE")
	if byTitle[0].Title != "A" {
		t.Fatalf("title: got %q", byTitle[0].Title)
	}

	for _, mode := range []string{"comments", "newest", "repo", "title"} {
		tied := []issue.Issue{
			{Title: "same", Repository: "r", Comments: 1, CreatedAt: t1, HTMLURL: "u-b"},
			{Title: "same", Repository: "r", Comments: 1, CreatedAt: t1, HTMLURL: "u-a"},
		}
		filter.SortIssues(tied, mode)
		if tied[0].HTMLURL != "u-a" {
			t.Fatalf("%s HTMLURL tie-break: got %q", mode, tied[0].HTMLURL)
		}
	}
}

func TestUniqueRepos(t *testing.T) {
	got := filter.UniqueRepos([]issue.Issue{
		{Repository: "kubernetes-sigs/kind"},
		{Repository: ""},
		{Repository: "kubernetes-sigs/cluster-api"},
		{Repository: "kubernetes-sigs/kind"},
	})
	want := []string{"kubernetes-sigs/cluster-api", "kubernetes-sigs/kind"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UniqueRepos = %v, want %v", got, want)
	}
}

func TestNormalizeSort(t *testing.T) {
	if filter.NormalizeSort(" COMMENTS ") != "comments" {
		t.Fatal("comments")
	}
	if filter.NormalizeSort("nope") != filter.DefaultSort {
		t.Fatal("unknown should fall back to newest")
	}
}

func TestSortIssuesUnknownModeUsesNewest(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []issue.Issue{
		{Title: "old", CreatedAt: t1, HTMLURL: "u1"},
		{Title: "new", CreatedAt: t2, HTMLURL: "u2"},
	}
	filter.SortIssues(issues, "not-a-sort")
	if issues[0].Title != "new" {
		t.Fatalf("unknown sort should use newest, got %q", issues[0].Title)
	}
}
