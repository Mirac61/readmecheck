package github

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

var reservedOwners = map[string]struct{}{
	"about": {}, "apps": {}, "collections": {}, "contact": {}, "codespaces": {},
	"enterprise": {}, "explore": {}, "features": {}, "login": {}, "marketplace": {},
	"new": {}, "notifications": {}, "orgs": {}, "pricing": {}, "search": {},
	"security": {}, "settings": {}, "signup": {}, "site": {}, "sponsors": {},
	"topics": {}, "users": {},
}

var repositorySubpages = map[string]struct{}{
	"actions": {}, "blame": {}, "commits": {}, "discussions": {}, "issues": {},
	"network": {}, "packages": {}, "projects": {}, "pulls": {}, "releases": {},
	"security": {}, "settings": {}, "wiki": {},
}

// RepositoryRef is the canonical owner/repository pair used by the GitHub provider.
type RepositoryRef struct {
	Owner string
	Repo  string
}

func (r RepositoryRef) FullName() string {
	return r.Owner + "/" + r.Repo
}

func (r RepositoryRef) URL() string {
	return "https://github.com/" + r.FullName()
}

func (r RepositoryRef) Key() string {
	return strings.ToLower(r.FullName())
}

// InvalidURLError describes input that cannot be resolved to a GitHub repository.
type InvalidURLError struct {
	Reason string
}

func (e *InvalidURLError) Error() string {
	return e.Reason
}

func invalid(format string, args ...any) error {
	return &InvalidURLError{Reason: fmt.Sprintf(format, args...)}
}

// ParseRepositoryURL accepts full GitHub URLs and the common shorthand forms.
func ParseRepositoryURL(input string) (RepositoryRef, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return RepositoryRef{}, invalid("Enter a GitHub repository URL.")
	}
	if strings.ContainsAny(raw, "\n\r\t") {
		return RepositoryRef{}, invalid("Enter a single GitHub repository URL.")
	}

	if strings.HasPrefix(strings.ToLower(raw), "git@github.com:") {
		raw = "https://github.com/" + raw[len("git@github.com:"):]
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "github.com/"), strings.HasPrefix(lower, "www.github.com/"):
		raw = "https://" + raw
	case !strings.Contains(raw, "://"):
		if strings.Contains(raw, "/") {
			raw = "https://github.com/" + strings.TrimPrefix(raw, "/")
		} else {
			return RepositoryRef{}, invalid("Use owner/repo or a full GitHub URL.")
		}
	}

	u, err := url.Parse(raw)
	if err != nil {
		return RepositoryRef{}, invalid("That is not a valid URL.")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return RepositoryRef{}, invalid("Only http and https GitHub URLs are supported.")
	}
	if u.User != nil {
		return RepositoryRef{}, invalid("GitHub URLs cannot contain a username or password.")
	}
	if u.Port() != "" {
		return RepositoryRef{}, invalid("GitHub URLs cannot include a port.")
	}
	if strings.ToLower(u.Hostname()) != "github.com" && strings.ToLower(u.Hostname()) != "www.github.com" {
		return RepositoryRef{}, invalid("Only github.com repositories are supported.")
	}

	path := strings.Trim(u.Path, "/")
	if path == "" {
		return RepositoryRef{}, invalid("Include an owner and repository, for example github.com/owner/repo.")
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" {
			return RepositoryRef{}, invalid("The repository URL contains an empty path segment.")
		}
	}

	if len(parts) < 2 {
		return RepositoryRef{}, invalid("Include both an owner and repository, for example github.com/owner/repo.")
	}
	owner := parts[0]
	repo := parts[1]
	if len(parts) > 2 {
		marker := strings.ToLower(parts[2])
		switch {
		case marker == "tree" || marker == "blob":
			if len(parts) < 4 {
				return RepositoryRef{}, invalid("The %s URL is missing a branch name.", marker)
			}
		case isRepositorySubpage(marker):
			return RepositoryRef{}, invalid("Use the repository root URL, not its %s page.", marker)
		default:
			return RepositoryRef{}, invalid("Use the repository root URL, not a subdirectory or unknown GitHub page.")
		}
	}

	if _, reserved := reservedOwners[strings.ToLower(owner)]; reserved {
		return RepositoryRef{}, invalid("That GitHub path is not a repository. Include the repository owner and name.")
	}
	if strings.HasSuffix(strings.ToLower(repo), ".git") {
		repo = repo[:len(repo)-len(".git")]
	}
	if !ownerPattern.MatchString(owner) {
		return RepositoryRef{}, invalid("The repository owner is not a valid GitHub username or organization.")
	}
	if repo == "" || repo == "." || repo == ".." || !repoPattern.MatchString(repo) {
		return RepositoryRef{}, invalid("The repository name is not valid.")
	}

	return RepositoryRef{Owner: owner, Repo: repo}, nil
}

func isRepositorySubpage(segment string) bool {
	_, ok := repositorySubpages[segment]
	return ok
}
