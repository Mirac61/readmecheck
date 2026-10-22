package rules

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"readmecheck/internal/github"
)

// TestProjectReadmeScoresWell keeps the repository's own README honest. The
// only rules that cannot be evaluated locally are skipped: external links are
// not checked, and no GitHub Actions badge is used.
func TestProjectReadmeScoresWell(t *testing.T) {
	root := filepath.Join("..", "..")
	content, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	files := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == ".git" || relative == "bin" || strings.HasPrefix(relative, ".git/") {
				return filepath.SkipDir
			}
			return nil
		}
		files[relative] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}

	repository := &github.Repository{
		Ref:           github.RepositoryRef{Owner: "owner", Repo: "readmecheck"},
		FullName:      "owner/readmecheck",
		HTMLURL:       "https://github.com/owner/readmecheck",
		Description:   "A deterministic, rule-based README auditor for GitHub repositories.",
		Topics:        []string{"go", "readme", "linter", "templ", "htmx"},
		DefaultBranch: "main",
		License:       &github.License{Key: "mit", Name: "MIT License", SPDXID: "MIT"},
		Stars:         150,
		Readme:        &github.README{Name: "README.md", Path: "README.md", Content: content},
		ReadmeKnown:   true,
		Root: []github.ContentEntry{
			{Name: "README.md", Path: "README.md", Type: "file"},
			{Name: "LICENSE", Path: "LICENSE", Type: "file"},
			{Name: "CONTRIBUTING.md", Path: "CONTRIBUTING.md", Type: "file"},
			{Name: "docs", Path: "docs", Type: "dir"},
			{Name: "Makefile", Path: "Makefile", Type: "file"},
		},
		RootKnown:      true,
		Workflows:      []github.Workflow{{Name: "CI", Path: ".github/workflows/ci.yml", State: "active"}},
		WorkflowsKnown: true,
		Files:          files,
		TreeKnown:      true,
	}

	report := Analyze(context.Background(), repository, Options{
		CheckExternalLinks: false,
		Now:                func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) },
	})
	if report.Score != 100 || report.Grade != "A" {
		var failures []string
		for _, result := range report.Results {
			if result.Status == StatusFail {
				failures = append(failures, result.Rule.ID+": "+result.Detail)
			}
		}
		t.Fatalf("project README scored %d (%s), want 100 (A); failures: %v", report.Score, report.Grade, failures)
	}
	if report.Counts.Skipped != 2 {
		t.Errorf("skipped = %d, want 2 (external links and CI badge)", report.Counts.Skipped)
	}
}
