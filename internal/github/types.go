package github

import (
	"context"
	"strings"
	"time"
)

// Provider fetches everything the rules need from a repository host.
type Provider interface {
	Fetch(ctx context.Context, ref RepositoryRef) (*Repository, error)
}

// Repository is a read-only snapshot used by the rule engine. It intentionally
// contains no HTTP details so another provider can populate it later.
type Repository struct {
	Ref           RepositoryRef
	FullName      string
	HTMLURL       string
	Description   string
	Homepage      string
	Topics        []string
	DefaultBranch string
	License       *License
	Stars         int
	Forks         int
	Archived      bool
	PushedAt      time.Time
	FetchedAt     time.Time

	Readme      *README
	ReadmeKnown bool

	Root      []ContentEntry
	RootKnown bool

	Workflows      []Workflow
	WorkflowsKnown bool

	LatestRelease *Release

	Files     map[string]bool
	TreeKnown bool
}

type README struct {
	Name    string
	Path    string
	Content []byte
}

type ContentEntry struct {
	Name string
	Path string
	Type string
}

type Workflow struct {
	Name  string
	Path  string
	State string
}

type Release struct {
	TagName     string
	Name        string
	PublishedAt time.Time
	Draft       bool
	Prerelease  bool
}

type License struct {
	Key    string
	Name   string
	SPDXID string
}

func (r *Repository) HasRootEntry(name string) bool {
	for _, entry := range r.Root {
		if strings.EqualFold(entry.Name, name) {
			return true
		}
	}
	return false
}

func (r *Repository) RootEntryPrefix(prefix string) bool {
	for _, entry := range r.Root {
		if strings.HasPrefix(strings.ToLower(entry.Name), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func (r *Repository) HasFile(path string) bool {
	return r.TreeKnown && r.Files[path]
}

func (r *Repository) HasAnyFile(paths ...string) bool {
	for _, path := range paths {
		if r.HasFile(path) {
			return true
		}
	}
	return false
}

func (r *Repository) WorkflowByPath(path string) (Workflow, bool) {
	for _, workflow := range r.Workflows {
		if strings.EqualFold(workflow.Path, path) {
			return workflow, true
		}
	}
	return Workflow{}, false
}
