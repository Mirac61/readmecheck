package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckLocalReadme(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)

	good := writeReadme(t, "# demo\n\nDemo is a small tool that formats tables for people who write docs.\n\n## Installation\n\n```sh\ngo install example.com/demo@latest\n```\n\n## Usage\n\nRun `demo` in a folder.\n\n## License\n\nMIT\n")
	var stdout, stderr bytes.Buffer
	if code := runCheck(context.Background(), []string{"--readme", good}, &stdout, &stderr); code != 0 {
		t.Fatalf("good README: exit %d, stderr %q\n%s", code, stderr.String(), stdout.String())
	}
	for _, want := range []string{"**SKIP** — `repo-description`", "**SKIP** — `topics`", "**PASS** — `readme-exists`"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("report missing %q", want)
		}
	}
	if written, _ := os.ReadFile(summary); string(written) != stdout.String() {
		t.Errorf("step summary does not match stdout")
	}

	bad := writeReadme(t, "Just some text without a title.\n")
	stdout.Reset()
	if code := runCheck(context.Background(), []string{"--readme", bad}, &stdout, &stderr); code != 1 {
		t.Fatalf("README without title: exit %d, want 1", code)
	}

	if code := runCheck(context.Background(), []string{"--readme", filepath.Join(t.TempDir(), "missing.md")}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing README: exit %d, want 2", code)
	}
}

func writeReadme(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
