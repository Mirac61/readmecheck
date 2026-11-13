package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"readmecheck/internal/github"
	"readmecheck/internal/rules"
)

// runCheck audits a README on disk and prints the Markdown report. It returns
// 1 only when a rule of the highest severity (error) fails, 2 on usage or I/O
// errors, so a low score alone never breaks a build.
func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	readmePath := flags.String("readme", "README.md", "README file to check, read from disk")
	repo := flags.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name for GitHub metadata (license, description, topics); empty skips those rules")
	externalLinks := flags.Bool("external-links", false, "check that external links are reachable")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	content, err := os.ReadFile(*readmePath)
	if err != nil {
		fmt.Fprintf(stderr, "readmecheck: %v\n", err)
		return 2
	}
	// shortcut: the path is used as given for resolving relative links, so run from the repository root.
	readme := &github.README{
		Name:    filepath.Base(*readmePath),
		Path:    filepath.ToSlash(filepath.Clean(*readmePath)),
		Content: content,
	}

	repository := &github.Repository{FullName: readme.Path, Local: true}
	if strings.TrimSpace(*repo) != "" {
		ref, err := github.ParseRepositoryURL(*repo)
		if err != nil {
			fmt.Fprintf(stderr, "readmecheck: --repo: %v\n", err)
			return 2
		}
		client, err := github.NewClient(github.ClientOptions{
			Token:   strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
			BaseURL: strings.TrimSpace(os.Getenv("GITHUB_API_URL")),
		})
		if err != nil {
			fmt.Fprintf(stderr, "readmecheck: %v\n", err)
			return 2
		}
		fetched, err := client.Fetch(ctx, ref)
		if err != nil {
			fmt.Fprintf(stderr, "readmecheck: %v\n", err)
			return 2
		}
		copied := *fetched
		repository = &copied
	}
	repository.Readme = readme
	repository.ReadmeKnown = true

	report := rules.Analyze(ctx, repository, rules.Options{CheckExternalLinks: *externalLinks})
	output := rules.ReportMarkdown(report)
	fmt.Fprint(stdout, output)

	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		if err := appendFile(summary, output); err != nil {
			fmt.Fprintf(stderr, "readmecheck: step summary: %v\n", err)
		}
	}

	if errs := report.Errors(); len(errs) > 0 {
		fmt.Fprintf(stderr, "readmecheck: %d error-level rule(s) failed\n", len(errs))
		return 1
	}
	return 0
}

func appendFile(path, content string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
