package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAPIBaseURL = "https://api.github.com"
	DefaultCacheTTL   = 10 * time.Minute
	// DefaultCacheMaxEntries bounds the in-memory snapshot cache. Each entry
	// holds a README and a repository file tree, so unbounded growth would be
	// noticeable on a small host.
	DefaultCacheMaxEntries = 256
	defaultUserAgent       = "readmecheck/1.0"
	maxAPIBodyBytes        = 5 << 20
)

type ClientOptions struct {
	Token      string
	BaseURL    string
	HTTPClient *http.Client
	CacheTTL   time.Duration
	UserAgent  string
	Now        func() time.Time
}

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	userAgent  string
	cache      *snapshotCache
	etags      *etagCache
	now        func() time.Time
}

var _ Provider = (*Client)(nil)

func NewClient(opts ClientOptions) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid GitHub API base URL %q", opts.BaseURL)
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	ttl := opts.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	return &Client{
		token:      strings.TrimSpace(opts.Token),
		baseURL:    baseURL,
		httpClient: httpClient,
		userAgent:  userAgent,
		cache:      newSnapshotCache(ttl, now),
		etags:      newETagCache(),
		now:        now,
	}, nil
}

func (c *Client) Fetch(ctx context.Context, ref RepositoryRef) (*Repository, error) {
	if ref.Owner == "" || ref.Repo == "" {
		return nil, fmt.Errorf("repository reference must include an owner and name")
	}
	if cached, ok := c.cache.get(ref.Key()); ok {
		return cached, nil
	}

	repository, err := c.fetch(ctx, ref)
	if err != nil {
		return nil, err
	}
	c.cache.put(ref.Key(), repository)
	return repository, nil
}

func (c *Client) fetch(ctx context.Context, ref RepositoryRef) (*Repository, error) {
	var metadata apiRepository
	if err := c.getJSON(ctx, c.repositoryURL(ref, ""), &metadata); err != nil {
		return nil, fmt.Errorf("fetch repository metadata: %w", err)
	}

	repository := &Repository{
		Ref:           ref,
		FullName:      metadata.FullName,
		HTMLURL:       metadata.HTMLURL,
		Description:   strings.TrimSpace(metadata.Description),
		Homepage:      metadata.Homepage,
		Topics:        metadata.Topics,
		DefaultBranch: metadata.DefaultBranch,
		Stars:         metadata.StargazersCount,
		Forks:         metadata.ForksCount,
		Archived:      metadata.Archived,
		PushedAt:      metadata.PushedAt,
		FetchedAt:     c.now(),
	}
	if repository.FullName == "" {
		repository.FullName = ref.FullName()
	}
	if repository.HTMLURL == "" {
		repository.HTMLURL = ref.URL()
	}
	if metadata.License != nil {
		repository.License = &License{
			Key:    metadata.License.Key,
			Name:   metadata.License.Name,
			SPDXID: metadata.License.SPDXID,
		}
	}

	var (
		wg            sync.WaitGroup
		readme        *README
		readmeErr     error
		workflows     []Workflow
		workflowsErr  error
		latestRelease *Release
		releaseErr    error
		treeFiles     map[string]bool
		treeErr       error
		treeTruncated bool
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		readme, readmeErr = c.fetchReadme(ctx, ref, repository.DefaultBranch)
	}()
	go func() {
		defer wg.Done()
		workflows, workflowsErr = c.fetchWorkflows(ctx, ref)
	}()
	go func() {
		defer wg.Done()
		latestRelease, releaseErr = c.fetchLatestRelease(ctx, ref)
	}()
	go func() {
		defer wg.Done()
		treeFiles, treeTruncated, treeErr = c.fetchTree(ctx, ref, repository.DefaultBranch)
	}()
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if readmeErr != nil {
		if errors.Is(readmeErr, ErrNotFound) {
			repository.ReadmeKnown = true
		} else {
			return nil, fmt.Errorf("fetch README: %w", readmeErr)
		}
	} else {
		repository.Readme = readme
		repository.ReadmeKnown = true
	}

	if treeErr == nil && !treeTruncated {
		repository.Files = treeFiles
		repository.TreeKnown = true
		// The recursive tree already contains every path. Deriving the root
		// listing from it saves the separate /contents request.
		repository.Root = rootEntriesFromTree(treeFiles)
		repository.RootKnown = true
	} else {
		root, rootErr := c.fetchRoot(ctx, ref, repository.DefaultBranch)
		if rootErr == nil {
			repository.Root = root
			repository.RootKnown = true
		} else if errors.Is(rootErr, ErrNotFound) {
			repository.RootKnown = true
		}
	}

	if workflowsErr == nil {
		repository.Workflows = workflows
		repository.WorkflowsKnown = true
	}
	if releaseErr == nil {
		repository.LatestRelease = latestRelease
	}
	if repository.Readme != nil {
		if repository.Readme.Path == "" {
			repository.Readme.Path = repository.Readme.Name
		}
		if repository.TreeKnown {
			repository.Readme.Path = resolveReadmePath(repository.Files, repository.Readme.Name)
		}
		if repository.Files == nil {
			repository.Files = make(map[string]bool)
		}
		repository.Files[repository.Readme.Path] = true
	}

	return repository, nil
}

type apiRepository struct {
	FullName        string    `json:"full_name"`
	HTMLURL         string    `json:"html_url"`
	Description     string    `json:"description"`
	Homepage        string    `json:"homepage"`
	Topics          []string  `json:"topics"`
	DefaultBranch   string    `json:"default_branch"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	Archived        bool      `json:"archived"`
	PushedAt        time.Time `json:"pushed_at"`
	License         *struct {
		Key    string `json:"key"`
		Name   string `json:"name"`
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
}

type apiContent struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type apiWorkflowList struct {
	Workflows []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		State string `json:"state"`
	} `json:"workflows"`
}

type apiRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

type apiTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
	Truncated bool `json:"truncated"`
}

func (c *Client) fetchReadme(ctx context.Context, ref RepositoryRef, branch string) (*README, error) {
	target := c.repositoryURL(ref, "/readme")
	if branch != "" {
		target += "?ref=" + url.QueryEscape(branch)
	}
	body, disposition, err := c.get(ctx, target, "application/vnd.github.raw")
	if err != nil {
		return nil, err
	}
	name := readmeFilename(disposition)
	return &README{
		Name:    name,
		Path:    name,
		Content: body,
	}, nil
}

func (c *Client) fetchRoot(ctx context.Context, ref RepositoryRef, branch string) ([]ContentEntry, error) {
	target := c.repositoryURL(ref, "/contents")
	if branch != "" {
		target += "?ref=" + url.QueryEscape(branch)
	}
	var contents []apiContent
	if err := c.getJSON(ctx, target, &contents); err != nil {
		return nil, err
	}
	result := make([]ContentEntry, 0, len(contents))
	for _, entry := range contents {
		result = append(result, ContentEntry{Name: entry.Name, Path: entry.Path, Type: entry.Type})
	}
	return result, nil
}

func (c *Client) fetchWorkflows(ctx context.Context, ref RepositoryRef) ([]Workflow, error) {
	target := c.repositoryURL(ref, "/actions/workflows") + "?per_page=100"
	var payload apiWorkflowList
	if err := c.getJSON(ctx, target, &payload); err != nil {
		return nil, err
	}
	result := make([]Workflow, 0, len(payload.Workflows))
	for _, workflow := range payload.Workflows {
		result = append(result, Workflow{Name: workflow.Name, Path: workflow.Path, State: workflow.State})
	}
	return result, nil
}

func (c *Client) fetchLatestRelease(ctx context.Context, ref RepositoryRef) (*Release, error) {
	target := c.repositoryURL(ref, "/releases/latest")
	var payload apiRelease
	if err := c.getJSON(ctx, target, &payload); err != nil {
		return nil, err
	}
	return &Release{
		TagName:     payload.TagName,
		Name:        payload.Name,
		PublishedAt: payload.PublishedAt,
		Draft:       payload.Draft,
		Prerelease:  payload.Prerelease,
	}, nil
}

func (c *Client) fetchTree(ctx context.Context, ref RepositoryRef, branch string) (map[string]bool, bool, error) {
	if branch == "" {
		return nil, false, fmt.Errorf("default branch is unknown")
	}
	target := c.repositoryURL(ref, "/git/trees/"+url.PathEscape(branch)) + "?recursive=1"
	var payload apiTree
	if err := c.getJSON(ctx, target, &payload); err != nil {
		return nil, false, err
	}
	files := make(map[string]bool, len(payload.Tree))
	for _, entry := range payload.Tree {
		files[entry.Path] = true
	}
	return files, payload.Truncated, nil
}

func (c *Client) repositoryURL(ref RepositoryRef, suffix string) string {
	return c.baseURL + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + suffix
}

func (c *Client) newRequest(ctx context.Context, target, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *Client) getJSON(ctx context.Context, target string, destination any) error {
	body, _, err := c.get(ctx, target, "application/vnd.github+json")
	if err != nil {
		return err
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(destination); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

// get performs a conditional GET. When GitHub answers 304 Not Modified the
// previously stored body is reused; such responses do not count against the
// primary rate limit. The second return value is the Content-Disposition
// header, restored from the cache on a 304.
func (c *Client) get(ctx context.Context, target, accept string) ([]byte, string, error) {
	req, err := c.newRequest(ctx, target, accept)
	if err != nil {
		return nil, "", err
	}
	key := accept + " " + target
	cached, hasCached := c.etags.lookup(key)
	if hasCached {
		req.Header.Set("If-None-Match", cached.etag)
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if hasCached {
			return cached.body, cached.meta, nil
		}
		return nil, "", fmt.Errorf("GitHub returned 304 for an unconditional request")
	}
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read GitHub response: %w", err)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.etags.store(key, etag, disposition, body)
	} else if hasCached {
		c.etags.delete(key)
	}
	return body, disposition, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GitHub API: %w", err)
	}
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return nil, newHTTPError(resp, body)
}

func readLimited(reader io.Reader) ([]byte, error) {
	const limit = maxAPIBodyBytes
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if len(content) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return content, nil
}

// rootEntriesFromTree derives the top-level directory listing from a recursive
// git tree, replacing a second /contents request.
func rootEntriesFromTree(files map[string]bool) []ContentEntry {
	entries := make(map[string]ContentEntry)
	for file := range files {
		name, rest, nested := strings.Cut(file, "/")
		if name == "" || name == ".git" {
			continue
		}
		entryType := "file"
		if nested && rest != "" {
			entryType = "dir"
		}
		existing, exists := entries[name]
		if exists && existing.Type == "dir" {
			continue
		}
		entries[name] = ContentEntry{Name: name, Path: name, Type: entryType}
	}
	result := make([]ContentEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func readmeFilename(disposition string) string {
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename := params["filename"]; filename != "" {
				return path.Base(filename)
			}
		}
	}
	return "README.md"
}

// resolveReadmePath finds the location GitHub used for a README. The raw
// endpoint only exposes the base filename, so the recursive tree is used to
// recover the directory. GitHub prefers .github/ over the root and docs/.
func resolveReadmePath(files map[string]bool, name string) string {
	if name == "" {
		name = "README.md"
	}
	for _, candidate := range []string{".github/" + name, name, "docs/" + name} {
		if files[candidate] {
			return candidate
		}
	}
	base := strings.ToLower(name)
	best := ""
	for file := range files {
		if strings.ToLower(path.Base(file)) != base {
			continue
		}
		if best == "" || len(file) < len(best) {
			best = file
		}
	}
	if best != "" {
		return best
	}
	return name
}
