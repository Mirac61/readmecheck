package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeGitHub struct {
	server      *httptest.Server
	mu          sync.Mutex
	requests    map[string]int
	total       atomic.Int32
	fail        map[string]func(w http.ResponseWriter, r *http.Request)
	readmeBody  string
	readmeName  string
	description string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{
		requests:    make(map[string]int),
		fail:        make(map[string]func(http.ResponseWriter, *http.Request)),
		readmeBody:  "# Demo\n\nA useful demo project.\n",
		readmeName:  "README.md",
		description: "A useful demo project.",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo", fake.repository)
	mux.HandleFunc("/repos/owner/repo/readme", fake.readme)
	mux.HandleFunc("/repos/owner/repo/contents", fake.contents)
	mux.HandleFunc("/repos/owner/repo/actions/workflows", fake.workflows)
	mux.HandleFunc("/repos/owner/repo/releases/latest", fake.release)
	mux.HandleFunc("/repos/owner/repo/git/trees/main", fake.tree)
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.total.Add(1)
		fake.mu.Lock()
		fake.requests[r.URL.Path]++
		handler := fake.fail[r.URL.Path]
		fake.mu.Unlock()
		if handler != nil {
			handler(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[path]
}

func (f *fakeGitHub) setFailure(path string, handler http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[path] = handler
}

func (f *fakeGitHub) client(t *testing.T, opts ClientOptions) *Client {
	t.Helper()
	if opts.BaseURL == "" {
		opts.BaseURL = f.server.URL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = f.server.Client()
	}
	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func (f *fakeGitHub) repository(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, `{
		"full_name":"owner/repo",
		"html_url":"https://github.com/owner/repo",
		"description":"`+f.description+`",
		"homepage":"https://example.com",
		"topics":["go","readme"],
		"default_branch":"main",
		"stargazers_count":123,
		"forks_count":7,
		"archived":false,
		"pushed_at":"2026-01-02T03:04:05Z",
		"license":{"key":"mit","name":"MIT License","spdx_id":"MIT"}
	}`)
}

func (f *fakeGitHub) readme(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+f.readmeName+`"`)
	_, _ = w.Write([]byte(f.readmeBody))
}

func (f *fakeGitHub) contents(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("ref") != "main" {
		http.Error(w, `{"message":"expected ref=main"}`, http.StatusBadRequest)
		return
	}
	jsonResponse(w, `[
		{"name":"README.md","path":"README.md","type":"file"},
		{"name":"LICENSE","path":"LICENSE","type":"file"},
		{"name":"docs","path":"docs","type":"dir"}
	]`)
}

func (f *fakeGitHub) workflows(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, `{"total_count":1,"workflows":[
		{"name":"CI","path":".github/workflows/ci.yml","state":"active"}
	]}`)
}

func (f *fakeGitHub) release(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, `{"tag_name":"v1.2.3","name":"v1.2.3","published_at":"2026-02-03T04:05:06Z","draft":false,"prerelease":false}`)
}

func (f *fakeGitHub) tree(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("recursive") != "1" {
		http.Error(w, `{"message":"expected recursive=1"}`, http.StatusBadRequest)
		return
	}
	jsonResponse(w, `{"truncated":false,"tree":[
		{"path":"README.md","type":"blob"},
		{"path":"LICENSE","type":"blob"},
		{"path":"docs","type":"tree"},
		{"path":"docs/guide.md","type":"blob"}
	]}`)
}

func jsonResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestClientFetchSuccess(t *testing.T) {
	fake := newFakeGitHub(t)
	var authHeaders []string
	fake.setFailure("/repos/owner/repo", func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "readmecheck/") {
			t.Errorf("User-Agent = %q", got)
		}
		fake.repository(w, r)
	})
	fake.setFailure("/repos/owner/repo/readme", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github.raw" {
			t.Errorf("README Accept = %q", got)
		}
		if got := r.URL.Query().Get("ref"); got != "main" {
			t.Errorf("README ref = %q", got)
		}
		fake.readme(w, r)
	})

	client := fake.client(t, ClientOptions{Token: "secret", CacheTTL: time.Minute})
	repository, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if repository.FullName != "owner/repo" || repository.HTMLURL != "https://github.com/owner/repo" {
		t.Fatalf("metadata = %+v", repository)
	}
	if repository.Description != "A useful demo project." || repository.Stars != 123 || repository.Forks != 7 {
		t.Fatalf("metadata fields = %+v", repository)
	}
	if len(repository.Topics) != 2 || repository.DefaultBranch != "main" || repository.Archived {
		t.Fatalf("topics/branch/archived = %+v", repository)
	}
	if repository.License == nil || repository.License.SPDXID != "MIT" {
		t.Fatalf("license = %+v", repository.License)
	}
	if repository.Readme == nil || repository.Readme.Name != "README.md" || !strings.Contains(string(repository.Readme.Content), "useful demo") {
		t.Fatalf("readme = %+v", repository.Readme)
	}
	if !repository.RootKnown || !repository.HasRootEntry("LICENSE") || !repository.HasRootEntry("docs") {
		t.Fatalf("root = %+v known=%v", repository.Root, repository.RootKnown)
	}
	if !repository.WorkflowsKnown || len(repository.Workflows) != 1 || repository.Workflows[0].Path != ".github/workflows/ci.yml" {
		t.Fatalf("workflows = %+v known=%v", repository.Workflows, repository.WorkflowsKnown)
	}
	if repository.LatestRelease == nil || repository.LatestRelease.TagName != "v1.2.3" {
		t.Fatalf("release = %+v", repository.LatestRelease)
	}
	if !repository.TreeKnown || !repository.HasFile("docs/guide.md") {
		t.Fatalf("tree = %+v known=%v", repository.Files, repository.TreeKnown)
	}
	if repository.FetchedAt.IsZero() {
		t.Fatal("FetchedAt is zero")
	}
	if len(authHeaders) != 1 || authHeaders[0] != "Bearer secret" {
		t.Fatalf("Authorization headers = %#v", authHeaders)
	}
}

func TestClientRepositoryNotFound(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.setFailure("/repos/owner/repo", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	client := fake.client(t, ClientOptions{})

	_, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	if err == nil {
		t.Fatal("Fetch succeeded, want error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("error = %#v, want HTTPError 404", err)
	}
}

func TestClientRateLimit(t *testing.T) {
	fake := newFakeGitHub(t)
	reset := time.Now().Add(12 * time.Minute).Truncate(time.Second)
	fake.setFailure("/repos/owner/repo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
	})
	client := fake.client(t, ClientOptions{})

	_, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("error = %v, want RateLimitError", err)
	}
	if !rateErr.Reset.Equal(reset) {
		t.Fatalf("reset = %v, want %v", rateErr.Reset, reset)
	}
	if rateErr.Limit != 60 || rateErr.Remaining != 0 {
		t.Fatalf("limit info = %+v", rateErr)
	}
}

func TestClientMissingReadme(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.setFailure("/repos/owner/repo/readme", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	client := fake.client(t, ClientOptions{})

	repository, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if repository.Readme != nil || !repository.ReadmeKnown {
		t.Fatalf("README = %+v known=%v", repository.Readme, repository.ReadmeKnown)
	}
}

func TestClientWorkflowPermissionIsNotFatal(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.setFailure("/repos/owner/repo/actions/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "44")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})
	client := fake.client(t, ClientOptions{})

	repository, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if repository.WorkflowsKnown || len(repository.Workflows) != 0 {
		t.Fatalf("workflows = %+v known=%v", repository.Workflows, repository.WorkflowsKnown)
	}
}

func TestClientCache(t *testing.T) {
	fake := newFakeGitHub(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	client := fake.client(t, ClientOptions{
		CacheTTL: time.Minute,
		Now:      func() time.Time { return now },
	})
	ref := RepositoryRef{Owner: "owner", Repo: "repo"}

	first, err := client.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	afterFirst := fake.total.Load()
	second, err := client.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if first != second {
		t.Fatal("cache did not return the same snapshot")
	}
	if got := fake.total.Load(); got != afterFirst {
		t.Fatalf("second Fetch made %d extra requests", got-afterFirst)
	}

	now = now.Add(61 * time.Second)
	if _, err := client.Fetch(context.Background(), ref); err != nil {
		t.Fatalf("third Fetch: %v", err)
	}
	if got := fake.total.Load(); got <= afterFirst {
		t.Fatal("expired cache was not refreshed")
	}
}

func TestClientNoToken(t *testing.T) {
	fake := newFakeGitHub(t)
	var gotAuth atomic.Value
	fake.setFailure("/repos/owner/repo", func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		fake.repository(w, r)
	})
	client := fake.client(t, ClientOptions{})
	if _, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if auth, _ := gotAuth.Load().(string); auth != "" {
		t.Fatalf("Authorization = %q, want empty", auth)
	}
}

func TestClientCancellation(t *testing.T) {
	fake := newFakeGitHub(t)
	client := fake.client(t, ClientOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Fetch(ctx, RepositoryRef{Owner: "owner", Repo: "repo"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	for _, baseURL := range []string{"ftp://example.com", "not a url", "https://"} {
		if _, err := NewClient(ClientOptions{BaseURL: baseURL}); err == nil {
			t.Errorf("NewClient(%q) succeeded, want error", baseURL)
		}
	}
	client, err := NewClient(ClientOptions{})
	if err != nil {
		t.Fatalf("NewClient defaults: %v", err)
	}
	if client.baseURL != DefaultAPIBaseURL {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
	if client.cache.ttl != DefaultCacheTTL {
		t.Fatalf("cache TTL = %v", client.cache.ttl)
	}
}

func TestHTTPErrorNotRateLimit(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.setFailure("/repos/owner/repo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "12")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
	})
	client := fake.client(t, ClientOptions{})
	_, err := client.Fetch(context.Background(), RepositoryRef{Owner: "owner", Repo: "repo"})
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		t.Fatalf("unexpected rate limit: %v", err)
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(fmt.Sprint(err), "/repos/owner/repo") {
		t.Fatalf("error lacks path: %v", err)
	}
}
