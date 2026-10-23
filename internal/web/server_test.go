package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"readmecheck/internal/github"
	"readmecheck/internal/rules"
	"readmecheck/internal/store"
)

func TestHomePage(t *testing.T) {
	server := newTestServer(t, testDeps{})
	response := get(t, server, "/")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, want := range []string{
		"Make your README",
		`hx-post="/check"`,
		`id="external-links"`,
		`name="external"`,
		"23 rules",
		"data-tui-dialog-trigger",
		`data-tui-dialog-target="rules-dialog"`,
		`id="rules-dialog"`,
		"Details",
		`id="results"`,
		`id="check-loading"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page does not contain %q", want)
		}
	}
	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestCheckHTMXReturnsReportAndRecordsHistory(t *testing.T) {
	checker := &fakeLinkChecker{}
	history := &fakeStore{}
	deps := testDeps{
		checker:      checker,
		store:        history,
		external:     true,
		repository:   goodRepository(t),
		now:          fixedTime,
		historyLimit: 1,
	}
	server := newTestServer(t, deps)

	response := postCheck(t, server, "github.com/owner/demo", true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("HX-Push-Url"); got != "/report/owner/demo?external=1" {
		t.Errorf("HX-Push-Url = %q", got)
	}
	body := response.Body.String()
	for _, want := range []string{
		`id="report"`,
		"owner/demo",
		"Overall score",
		"Category scores",
		"Checklist",
		"Copy report as Markdown",
		`data-download-report="true"`,
		"readmecheck-owner-demo.md",
		"External links checked",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("report does not contain %q", want)
		}
	}
	if checker.calls != 1 {
		t.Errorf("link checker calls = %d, want 1", checker.calls)
	}
	if len(history.checks) != 1 {
		t.Fatalf("history rows = %d, want 1", len(history.checks))
	}
	check := history.checks[0]
	if check.Owner != "owner" || check.Repo != "demo" {
		t.Errorf("stored repository = %s/%s", check.Owner, check.Repo)
	}
	if check.Grade != "A" || check.Score != 100 {
		t.Errorf("stored score = %d (%s), want 100 (A)", check.Score, check.Grade)
	}
	if !check.ExternalLinks {
		t.Error("stored external_links = false, want true")
	}
}

func TestCheckWithoutExternalLinksSkipsChecker(t *testing.T) {
	checker := &fakeLinkChecker{}
	deps := testDeps{checker: checker, repository: goodRepository(t), now: fixedTime}
	server := newTestServer(t, deps)

	response := postCheck(t, server, "owner/demo", false, false)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if checker.calls != 0 {
		t.Errorf("link checker calls = %d, want 0", checker.calls)
	}
	body := response.Body.String()
	if !strings.Contains(body, "External links not checked") {
		t.Error("report does not state that external links were not checked")
	}
	if !strings.Contains(body, "External link checking is disabled for this run.") {
		t.Error("skipped external link rule is not explained")
	}
}

func TestCheckInvalidURLHTMXReturnsInlineError(t *testing.T) {
	server := newTestServer(t, testDeps{})
	response := postCheck(t, server, "https://example.com/not-github", true, true)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for htmx fragment", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Only github.com repositories are supported.") {
		t.Errorf("error message missing from fragment: %s", body)
	}
	if !strings.Contains(body, `role="alert"`) {
		t.Error("error fragment has no alert role")
	}
}

func TestCheckInvalidURLFullPageReturns422(t *testing.T) {
	server := newTestServer(t, testDeps{})
	response := postCheck(t, server, "not a repository", false, false)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Use owner/repo or a full GitHub URL.") {
		t.Errorf("error message missing from page: %s", body)
	}
	if !strings.Contains(body, "Make your README") {
		t.Error("full page error did not render the home page")
	}
}

func TestCheckProviderNotFound(t *testing.T) {
	server := newTestServer(t, testDeps{providerErr: github.ErrNotFound})

	htmxResponse := postCheck(t, server, "owner/missing", true, true)
	if htmxResponse.Code != http.StatusOK {
		t.Errorf("htmx status = %d, want 200", htmxResponse.Code)
	}
	if !strings.Contains(htmxResponse.Body.String(), "Repository not found") {
		t.Errorf("htmx error missing friendly message: %s", htmxResponse.Body.String())
	}

	fullResponse := postCheck(t, server, "owner/missing", false, false)
	if fullResponse.Code != http.StatusNotFound {
		t.Errorf("full status = %d, want 404", fullResponse.Code)
	}
}

func TestMissingReadmeStillProducesReport(t *testing.T) {
	repository := goodRepository(t)
	repository.Readme = nil
	repository.ReadmeKnown = true
	server := newTestServer(t, testDeps{repository: repository, now: fixedTime})

	response := postCheck(t, server, "owner/demo", true, false)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "README exists and has content") {
		t.Error("report does not list the README rule")
	}
	if !strings.Contains(body, "GitHub returned no README for this repository.") {
		t.Error("report does not explain the missing README")
	}
}

func TestReportRouteRendersAndSaves(t *testing.T) {
	history := &fakeStore{}
	deps := testDeps{repository: goodRepository(t), store: history, now: fixedTime, external: true}
	server := newTestServer(t, deps)

	response := get(t, server, "/report/owner/demo?external=1")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"owner/demo", "Check another repository", "Download .md"} {
		if !strings.Contains(body, want) {
			t.Errorf("report page does not contain %q", want)
		}
	}
	if len(history.checks) != 1 {
		t.Errorf("history rows = %d, want 1", len(history.checks))
	}
}

func TestHistoryPageListsLatestChecksAndRerun(t *testing.T) {
	history := &fakeStore{}
	deps := testDeps{store: history, now: fixedTime}
	server := newTestServer(t, deps)
	history.checks = []store.Check{
		{
			Owner:         "owner",
			Repo:          "demo",
			Description:   "A small command-line tool for diagrams.",
			License:       "MIT",
			Stars:         2500,
			Score:         94,
			Grade:         "A",
			CheckedAt:     fixedTime(),
			ExternalLinks: true,
		},
	}

	response := get(t, server, "/history")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		"Check history",
		"owner/demo",
		"Re-run",
		"/report/owner/demo?external=1",
		"94",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("history page does not contain %q", want)
		}
	}
}

func TestHistoryPageEmptyAndError(t *testing.T) {
	server := newTestServer(t, testDeps{})
	response := get(t, server, "/history")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "No checks yet") {
		t.Error("empty history state is missing")
	}

	failing := newTestServer(t, testDeps{storeErr: errors.New("database is unavailable")})
	response = get(t, failing, "/history")
	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Check history could not be loaded.") {
		t.Error("history error message is missing")
	}
}

func TestHealthzAndAssets(t *testing.T) {
	server := newTestServer(t, testDeps{assetsDir: t.TempDir()})
	response := get(t, server, "/healthz")
	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Errorf("healthz = %d %q", response.Code, response.Body.String())
	}
}

func TestServerUsesConfiguredDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	server, err := New(Config{
		AssetsDir:    t.TempDir(),
		DatabasePath: path,
		Provider:     &fakeProvider{repository: goodRepository(t)},
		LinkChecker:  &fakeLinkChecker{},
		Now:          fixedTime,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !server.ownsHistory {
		t.Error("server did not open its own SQLite store")
	}
	response := postCheck(t, server, "owner/demo", true, false)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if err := server.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file was not created: %v", err)
	}
}

type testDeps struct {
	repository   *github.Repository
	providerErr  error
	checker      *fakeLinkChecker
	store        *fakeStore
	storeErr     error
	assetsDir    string
	now          func() time.Time
	external     bool
	historyLimit int
}

func newTestServer(t *testing.T, deps testDeps) *Server {
	t.Helper()
	if deps.store == nil {
		deps.store = &fakeStore{recentErr: deps.storeErr}
	}
	if deps.checker == nil {
		deps.checker = &fakeLinkChecker{}
	}
	if deps.repository == nil {
		deps.repository = goodRepository(t)
	}
	if deps.now == nil {
		deps.now = fixedTime
	}
	if deps.assetsDir == "" {
		deps.assetsDir = t.TempDir()
	}
	server, err := New(Config{
		AssetsDir:     deps.assetsDir,
		Provider:      &fakeProvider{repository: deps.repository, err: deps.providerErr},
		History:       deps.store,
		LinkChecker:   deps.checker,
		Now:           deps.now,
		ExternalLinks: deps.external,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return server
}

func goodRepository(t *testing.T) *github.Repository {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "good.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	files := map[string]bool{
		"README.md":                true,
		"LICENSE":                  true,
		"CONTRIBUTING.md":          true,
		"docs/guide.md":            true,
		"docs/screenshot.png":      true,
		"demo.yaml":                true,
		".github/workflows/ci.yml": true,
	}
	return &github.Repository{
		Ref:           github.RepositoryRef{Owner: "owner", Repo: "demo"},
		FullName:      "owner/demo",
		HTMLURL:       "https://github.com/owner/demo",
		Description:   "A small command-line tool that turns structured text into colorful diagrams.",
		Homepage:      "https://example.com/demo",
		Topics:        []string{"go", "cli", "diagrams"},
		DefaultBranch: "main",
		License:       &github.License{Key: "mit", Name: "MIT License", SPDXID: "MIT"},
		Stars:         2500,
		Forks:         120,
		PushedAt:      fixedTime().Add(-24 * time.Hour),
		FetchedAt:     fixedTime(),
		Readme:        &github.README{Name: "README.md", Path: "README.md", Content: content},
		ReadmeKnown:   true,
		Root: []github.ContentEntry{
			{Name: "README.md", Path: "README.md", Type: "file"},
			{Name: "LICENSE", Path: "LICENSE", Type: "file"},
			{Name: "CONTRIBUTING.md", Path: "CONTRIBUTING.md", Type: "file"},
			{Name: "docs", Path: "docs", Type: "dir"},
		},
		RootKnown: true,
		Workflows: []github.Workflow{
			{Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
		},
		WorkflowsKnown: true,
		LatestRelease:  &github.Release{TagName: "v1.2.3", Name: "v1.2.3", PublishedAt: fixedTime().Add(-48 * time.Hour)},
		Files:          files,
		TreeKnown:      true,
	}
}

func fixedTime() time.Time {
	return time.Date(2026, time.October, 22, 12, 30, 0, 0, time.UTC)
}

type fakeProvider struct {
	repository *github.Repository
	err        error
	calls      int
	lastRef    github.RepositoryRef
}

func (p *fakeProvider) Fetch(_ context.Context, ref github.RepositoryRef) (*github.Repository, error) {
	p.calls++
	p.lastRef = ref
	if p.err != nil {
		return nil, p.err
	}
	return p.repository, nil
}

type fakeStore struct {
	checks    []store.Check
	saveErr   error
	recentErr error
	closed    bool
}

func (s *fakeStore) Save(_ context.Context, check store.Check) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.checks = append(s.checks, check)
	return nil
}

func (s *fakeStore) Recent(_ context.Context, limit int) ([]store.Check, error) {
	if s.recentErr != nil {
		return nil, s.recentErr
	}
	if limit > 0 && len(s.checks) > limit {
		return s.checks[:limit], nil
	}
	return s.checks, nil
}

func (s *fakeStore) Close() error {
	s.closed = true
	return nil
}

type fakeLinkChecker struct {
	calls   int
	urls    []string
	results []rules.LinkCheck
}

func (c *fakeLinkChecker) Check(_ context.Context, urls []string) []rules.LinkCheck {
	c.calls++
	c.urls = append(c.urls, urls...)
	if c.results != nil {
		return c.results
	}
	results := make([]rules.LinkCheck, 0, len(urls))
	for _, target := range urls {
		results = append(results, rules.LinkCheck{URL: target, OK: true, StatusCode: http.StatusOK})
	}
	return results
}

func postCheck(t *testing.T, server *Server, rawURL string, external, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"url": {rawURL}}
	if external {
		form.Set("external", "1")
	}
	request := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		request.Header.Set("HX-Request", "true")
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func get(t *testing.T, server *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
