package github

import (
	"strings"
	"testing"
)

func TestParseRepositoryURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		owner   string
		repo    string
		wantErr string
	}{
		{name: "canonical", input: "https://github.com/charmbracelet/bubbletea", owner: "charmbracelet", repo: "bubbletea"},
		{name: "trailing slash", input: "https://github.com/charmbracelet/bubbletea/", owner: "charmbracelet", repo: "bubbletea"},
		{name: "dot git", input: "https://github.com/charmbracelet/bubbletea.git", owner: "charmbracelet", repo: "bubbletea"},
		{name: "dot git upper", input: "https://github.com/charmbracelet/bubbletea.GIT", owner: "charmbracelet", repo: "bubbletea"},
		{name: "tree path", input: "https://github.com/charmbracelet/bubbletea/tree/main/examples", owner: "charmbracelet", repo: "bubbletea"},
		{name: "tree branch only", input: "https://github.com/charmbracelet/bubbletea/tree/main", owner: "charmbracelet", repo: "bubbletea"},
		{name: "blob path", input: "https://github.com/charmbracelet/bubbletea/blob/main/README.md", owner: "charmbracelet", repo: "bubbletea"},
		{name: "repository tab query", input: "https://github.com/charmbracelet/bubbletea?tab=readme-ov-file#readme", owner: "charmbracelet", repo: "bubbletea"},
		{name: "without scheme", input: "github.com/charmbracelet/bubbletea", owner: "charmbracelet", repo: "bubbletea"},
		{name: "www without scheme", input: "www.github.com/charmbracelet/bubbletea/", owner: "charmbracelet", repo: "bubbletea"},
		{name: "owner repo shorthand", input: "charmbracelet/bubbletea", owner: "charmbracelet", repo: "bubbletea"},
		{name: "owner repo dot git", input: "charmbracelet/bubbletea.git", owner: "charmbracelet", repo: "bubbletea"},
		{name: "ssh clone", input: "git@github.com:charmbracelet/bubbletea.git", owner: "charmbracelet", repo: "bubbletea"},
		{name: "http", input: "http://github.com/owner/repo", owner: "owner", repo: "repo"},
		{name: "uppercase host", input: "HTTPS://GITHUB.COM/Owner/Repo", owner: "Owner", repo: "Repo"},
		{name: "repo punctuation", input: "owner/my.repo_name-2", owner: "owner", repo: "my.repo_name-2"},
		{name: "surrounding whitespace", input: "  owner/repo  ", owner: "owner", repo: "repo"},

		{name: "empty", input: "", wantErr: "Enter a GitHub repository URL"},
		{name: "blank", input: "   ", wantErr: "Enter a GitHub repository URL"},
		{name: "one word", input: "bubbletea", wantErr: "owner/repo"},
		{name: "host only", input: "https://github.com", wantErr: "Include an owner"},
		{name: "owner only", input: "https://github.com/charmbracelet", wantErr: "both an owner"},
		{name: "empty owner", input: "https://github.com//bubbletea", wantErr: "both an owner"},
		{name: "gitlab", input: "https://gitlab.com/owner/repo", wantErr: "Only github.com"},
		{name: "gist", input: "https://gist.github.com/owner/123", wantErr: "Only github.com"},
		{name: "ftp", input: "ftp://github.com/owner/repo", wantErr: "http and https"},
		{name: "userinfo", input: "https://user@github.com/owner/repo", wantErr: "username or password"},
		{name: "port", input: "https://github.com:443/owner/repo", wantErr: "port"},
		{name: "issues", input: "https://github.com/owner/repo/issues/1", wantErr: "issues page"},
		{name: "settings", input: "https://github.com/owner/repo/settings", wantErr: "settings page"},
		{name: "tree no branch", input: "https://github.com/owner/repo/tree", wantErr: "missing a branch"},
		{name: "extra segment", input: "https://github.com/owner/repo/extra", wantErr: "subdirectory"},
		{name: "reserved owner", input: "https://github.com/topics/go", wantErr: "not a repository"},
		{name: "empty repo", input: "https://github.com/owner/", wantErr: "both an owner"},
		{name: "leading slash shorthand", input: "/owner/repo", owner: "owner", repo: "repo"},
		{name: "invalid owner hyphen edges", input: "https://github.com/-owner/repo", wantErr: "owner is not a valid"},
		{name: "repo only dots", input: "https://github.com/owner/..", wantErr: "repository name is not valid"},
		{name: "repo spaces", input: "https://github.com/owner/re po", wantErr: "repository name is not valid"},
		{name: "multi line", input: "owner/repo\nhttps://github.com/other/repo", wantErr: "single"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRepositoryURL(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseRepositoryURL(%q) = %+v, want error containing %q", tt.input, got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepositoryURL(%q): unexpected error: %v", tt.input, err)
			}
			if got.Owner != tt.owner || got.Repo != tt.repo {
				t.Fatalf("ParseRepositoryURL(%q) = %s/%s, want %s/%s", tt.input, got.Owner, got.Repo, tt.owner, tt.repo)
			}
			if got.FullName() != tt.owner+"/"+tt.repo {
				t.Fatalf("FullName = %q", got.FullName())
			}
			if got.URL() != "https://github.com/"+tt.owner+"/"+tt.repo {
				t.Fatalf("URL = %q", got.URL())
			}
		})
	}
}

func TestParseRepositoryURLNeverPanics(t *testing.T) {
	t.Parallel()
	inputs := []string{"https://github.com/", "//", "http://[::1", "github.com/", "a/b/c/d/e", "https://github.com/a/b/tree/"}
	for _, input := range inputs {
		_, _ = ParseRepositoryURL(input)
	}
}
