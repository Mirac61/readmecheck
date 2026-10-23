package rules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"readmecheck/internal/github"
)

type fakeLinkChecker struct {
	fail map[string]LinkCheck
}

func (f fakeLinkChecker) Check(_ context.Context, urls []string) []LinkCheck {
	results := make([]LinkCheck, 0, len(urls))
	for _, target := range urls {
		if failure, ok := f.fail[target]; ok {
			failure.URL = target
			results = append(results, failure)
			continue
		}
		results = append(results, LinkCheck{URL: target, OK: true, StatusCode: 200})
	}
	return results
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return content
}

func repositoryFixture(t *testing.T, readmeFile string) *github.Repository {
	t.Helper()
	content := fixture(t, readmeFile)
	return &github.Repository{
		Ref:           github.RepositoryRef{Owner: "owner", Repo: "demo"},
		FullName:      "owner/demo",
		HTMLURL:       "https://github.com/owner/demo",
		Description:   "Turn structured text into colorful diagrams.",
		Topics:        []string{"go", "cli", "diagrams"},
		DefaultBranch: "main",
		License:       &github.License{Key: "mit", Name: "MIT License", SPDXID: "MIT"},
		Stars:         120,
		Readme:        &github.README{Name: readmeFile, Path: "README.md", Content: content},
		ReadmeKnown:   true,
		RootKnown:     true,
		Root: []github.ContentEntry{
			{Name: "README.md", Path: "README.md", Type: "file"},
			{Name: "LICENSE", Path: "LICENSE", Type: "file"},
			{Name: "CONTRIBUTING.md", Path: "CONTRIBUTING.md", Type: "file"},
			{Name: "docs", Path: "docs", Type: "dir"},
			{Name: ".github", Path: ".github", Type: "dir"},
		},
		WorkflowsKnown: true,
		Workflows: []github.Workflow{
			{Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
		},
		TreeKnown: true,
		Files: map[string]bool{
			"README.md":                true,
			"LICENSE":                  true,
			"CONTRIBUTING.md":          true,
			"docs":                     true,
			"docs/screenshot.png":      true,
			"docs/guide.md":            true,
			"docs/leitfaden.md":        true,
			"docs/logo.svg":            true,
			".github":                  true,
			".github/workflows":        true,
			".github/workflows/ci.yml": true,
		},
	}
}

func analyzeFixture(t *testing.T, readmeFile string) Report {
	t.Helper()
	repository := repositoryFixture(t, readmeFile)
	fixedNow := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	return Analyze(context.Background(), repository, Options{
		CheckExternalLinks: true,
		LinkChecker:        fakeLinkChecker{},
		Now:                func() time.Time { return fixedNow },
	})
}

func resultByID(t *testing.T, report Report, id string) Result {
	t.Helper()
	for _, result := range report.Results {
		if result.Rule.ID == id {
			return result
		}
	}
	t.Fatalf("rule %q not found", id)
	return Result{}
}

func TestRuleRegistry(t *testing.T) {
	t.Parallel()
	rules := Rules()
	if len(rules) < 20 {
		t.Fatalf("only %d rules; require at least 20", len(rules))
	}
	if len(rules) != len(ruleSet) {
		t.Fatal("Rules did not return a full copy")
	}
	seen := make(map[string]bool, len(rules))
	categoryCount := make(map[Category]int)
	for _, rule := range rules {
		if rule.ID == "" || rule.Title == "" || rule.Why == "" || rule.Fix == "" {
			t.Errorf("rule is missing metadata: %+v", rule)
		}
		if seen[rule.ID] {
			t.Errorf("duplicate rule ID %q", rule.ID)
		}
		seen[rule.ID] = true
		if rule.Weight <= 0 {
			t.Errorf("%s has non-positive weight", rule.ID)
		}
		if rule.Check == nil {
			t.Errorf("%s has no check function", rule.ID)
		}
		if _, ok := categoryLabels[rule.Category]; !ok {
			t.Errorf("%s has unknown category %q", rule.ID, rule.Category)
		}
		if rule.Severity != SeverityError && rule.Severity != SeverityWarning && rule.Severity != SeverityInfo {
			t.Errorf("%s has unknown severity %q", rule.ID, rule.Severity)
		}
		categoryCount[rule.Category]++
	}
	minimums := map[Category]int{
		CategoryBasics:         5,
		CategoryGettingStarted: 4,
		CategoryVisuals:        2,
		CategoryTrust:          5,
		CategoryContribution:   1,
		CategoryHygiene:        4,
		CategoryLinks:          2,
	}
	for category, minimum := range minimums {
		if categoryCount[category] < minimum {
			t.Errorf("category %s has %d rules, want at least %d", category, categoryCount[category], minimum)
		}
	}
}

func TestGoodReadmeScoresWell(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "good.md")
	if report.Score < 90 {
		t.Errorf("score = %d, want >= 90; failures:", report.Score)
		for _, result := range report.Results {
			if result.Status == StatusFail {
				t.Errorf("  %s: %s (%v)", result.Rule.ID, result.Detail, result.Evidence)
			}
		}
	}
	if report.Grade != "A" {
		t.Errorf("grade = %q, want A", report.Grade)
	}
	if report.Counts.Passed == 0 || report.Counts.Failed != 0 {
		t.Errorf("counts = %+v", report.Counts)
	}
	if report.GeneratedAt.Year() != 2026 {
		t.Errorf("GeneratedAt = %v", report.GeneratedAt)
	}
	if !report.ExternalLinksChecked {
		t.Error("ExternalLinksChecked = false")
	}
}

func TestBadReadmeFailsCoreRules(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "bad.md")
	if report.Score >= 80 {
		t.Errorf("score = %d, want < 80", report.Score)
	}
	expectStatus(t, report, "installation-section", StatusFail)
	expectStatus(t, report, "usage-section", StatusPass)
	expectStatus(t, report, "code-example", StatusFail)
	expectStatus(t, report, "no-empty-sections", StatusFail)
	expectStatus(t, report, "no-placeholders", StatusFail)
	expectStatus(t, report, "contributing", StatusPass)
	expectStatus(t, report, "license-mentioned", StatusFail)
	if report.Counts.Failed == 0 {
		t.Fatal("no failures")
	}
}

func TestEmptyReadmeIsReportable(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "empty.md")
	expectStatus(t, report, "readme-exists", StatusFail)
	expectStatus(t, report, "top-level-title", StatusSkipped)
	expectStatus(t, report, "description-paragraph", StatusSkipped)
	expectStatus(t, report, "image-alt", StatusSkipped)
	if report.Repository.ReadmeKnown != true {
		t.Fatal("fixture readme known changed")
	}
}

func TestImagesOnlyReadme(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "images-only.md")
	expectStatus(t, report, "top-level-title", StatusFail)
	expectStatus(t, report, "description-paragraph", StatusFail)
	expectStatus(t, report, "visual", StatusPass)
	expectStatus(t, report, "image-alt", StatusPass)
	expectStatus(t, report, "no-empty-sections", StatusPass)
}

func TestGermanReadmeKeywords(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "german.md")
	for _, id := range []string{"installation-section", "usage-section", "prerequisites", "contributing", "license-mentioned"} {
		expectStatus(t, report, id, StatusPass)
	}
	if report.Score < 80 {
		t.Errorf("German README score = %d, failures: %+v", report.Score, report.Results)
	}
}

func TestRelativeLinksResolveFromNestedReadme(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Path = "docs/README.md"
	repository.Readme.Content = []byte("# Demo\n\n[Guide](guide.md)\n\n![Screenshot](screenshot.png)\n")
	repository.Files["docs/README.md"] = true
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	expectStatus(t, report, "relative-links", StatusPass)
}

func TestMissingRelativeImageFails(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Content = []byte("# Demo\n\n![Screenshot](docs/missing.png)\n")
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	result := resultByID(t, report, "relative-links")
	if result.Status != StatusFail {
		t.Fatalf("relative-links = %+v", result)
	}
	if !strings.Contains(strings.Join(result.Evidence, "\n"), "docs/missing.png") {
		t.Errorf("evidence = %v", result.Evidence)
	}
}

func TestRelativeLinkOutsideReadmeDirectory(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"../../secret.md", "../../../etc/passwd"} {
		if resolved, ok := resolveRelativeLinkForReadme(target, "docs/README.md"); ok {
			t.Errorf("resolveRelativeLinkForReadme(%q) = %q, true; want rejected", target, resolved)
		}
	}
	resolved, ok := resolveRelativeLinkForReadme("../CONTRIBUTING.md", "nested/deep/README.md")
	if !ok || resolved != "nested/CONTRIBUTING.md" {
		t.Errorf("nested relative link = %q, %v", resolved, ok)
	}
	root, ok := resolveRelativeLinkForReadme("../outside.md", "docs/README.md")
	if !ok || root != "outside.md" {
		t.Errorf("parent relative link = %q, %v", root, ok)
	}
}

func TestTitleMustBeFirstHeading(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Content = []byte("## Usage\n\nRun the tool.\n\n# Demo\n\nDemo turns text into diagrams for the whole team.\n")
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	expectStatus(t, report, "top-level-title", StatusFail)
}

func TestBrokenRelativeLinks(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "broken-links.md")
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	result := resultByID(t, report, "relative-links")
	if result.Status != StatusFail {
		t.Fatalf("relative-links = %+v", result)
	}
	if !strings.Contains(strings.Join(result.Evidence, "\n"), "docs/missing.md") {
		t.Errorf("evidence = %v", result.Evidence)
	}
}

func TestRelativeLinksSkippedWithoutTree(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "broken-links.md")
	repository.TreeKnown = false
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	expectStatus(t, report, "relative-links", StatusSkipped)
}

func TestRelativeLinkEscapesRepository(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"../secret.md", "../../etc/passwd", "docs/../../secret.md"} {
		if resolved, ok := resolveRelativeLink(target); ok {
			t.Errorf("resolveRelativeLink(%q) = %q, true; want rejected", target, resolved)
		}
	}
	for target, want := range map[string]string{
		"docs/guide.md":         "docs/guide.md",
		"./docs/guide.md#usage": "docs/guide.md",
		"/LICENSE":              "LICENSE",
		"docs/":                 "docs",
		"docs%2Fguide.md":       "docs/guide.md",
		"docs/guide.md?plain=1": "docs/guide.md",
	} {
		got, ok := resolveRelativeLink(target)
		if !ok || got != want {
			t.Errorf("resolveRelativeLink(%q) = %q, %v; want %q, true", target, got, ok, want)
		}
	}
}

func TestBadgeValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		readme   string
		workflow []github.Workflow
		known    bool
		want     Status
	}{
		{
			name:     "valid badge",
			readme:   "# Demo\n\n[![CI](https://github.com/owner/demo/actions/workflows/ci.yml/badge.svg)](https://github.com/owner/demo/actions)\n\nA sufficiently long description of this project for tests.\n",
			workflow: []github.Workflow{{Name: "CI", Path: ".github/workflows/ci.yml"}},
			known:    true,
			want:     StatusPass,
		},
		{
			name:     "missing workflow",
			readme:   "# Demo\n\n[![CI](https://github.com/owner/demo/actions/workflows/missing.yml/badge.svg)](https://github.com/owner/demo/actions)\n",
			workflow: []github.Workflow{{Name: "CI", Path: ".github/workflows/ci.yml"}},
			known:    true,
			want:     StatusFail,
		},
		{
			name:   "no badge",
			readme: "# Demo\n\nNo badge here.\n",
			known:  true,
			want:   StatusSkipped,
		},
		{
			name:     "workflow unknown",
			readme:   "# Demo\n\n[![CI](https://github.com/owner/demo/actions/workflows/ci.yml/badge.svg)](https://github.com/owner/demo/actions)\n",
			workflow: nil,
			known:    false,
			want:     StatusSkipped,
		},
		{
			name:     "badge for another repo",
			readme:   "# Demo\n\n[![CI](https://github.com/other/project/actions/workflows/ci.yml/badge.svg)](https://github.com/other/project/actions)\n",
			workflow: nil,
			known:    true,
			want:     StatusSkipped,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repository := repositoryFixture(t, "good.md")
			repository.Readme.Content = []byte(tt.readme)
			repository.Workflows = tt.workflow
			repository.WorkflowsKnown = tt.known
			report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
			expectStatus(t, report, "badges-valid", tt.want)
		})
	}
}

func TestExternalLinkRule(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	target := "https://github.com/owner/demo/actions"
	report := Analyze(context.Background(), repository, Options{
		CheckExternalLinks: true,
		LinkChecker: fakeLinkChecker{fail: map[string]LinkCheck{
			target: {OK: false, StatusCode: 404},
		}},
	})
	result := resultByID(t, report, "external-links")
	if result.Status != StatusFail {
		t.Fatalf("external-links = %+v", result)
	}
	if !strings.Contains(strings.Join(result.Evidence, "\n"), target) {
		t.Errorf("evidence = %v", result.Evidence)
	}
}

func TestExternalLinksCanBeDisabled(t *testing.T) {
	t.Parallel()
	report := Analyze(context.Background(), repositoryFixture(t, "good.md"), Options{CheckExternalLinks: false})
	expectStatus(t, report, "external-links", StatusSkipped)
	if report.ExternalLinksChecked {
		t.Error("ExternalLinksChecked = true")
	}
}

func TestImageAltMissing(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Content = []byte("# Demo\n\n![Screenshot](docs/screenshot.png)\n\n![ ](docs/second.png)\n\nThis is a description with enough words to pass the paragraph check.\n")
	report := Analyze(context.Background(), repository, Options{LinkChecker: fakeLinkChecker{}})
	expectStatus(t, report, "image-alt", StatusFail)
}

func TestPlaceholders(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{"TODO: write this", "Lorem ipsum dolor sit amet", "Project Title", "One Paragraph of project description goes here"} {
		if got := findPlaceholders("# Demo\n\n" + marker + "\n"); len(got) == 0 {
			t.Errorf("findPlaceholders(%q) found nothing", marker)
		}
	}
}

func TestCategoryScoresAndSkippedRules(t *testing.T) {
	t.Parallel()
	results := []Result{
		{Rule: Rule{Category: CategoryBasics, Weight: 10}, Status: StatusPass},
		{Rule: Rule{Category: CategoryBasics, Weight: 10}, Status: StatusFail},
		{Rule: Rule{Category: CategoryBasics, Weight: 50}, Status: StatusSkipped},
		{Rule: Rule{Category: CategoryLinks, Weight: 5}, Status: StatusPass},
	}
	categories := scoreCategories(results)
	basics := categories[0]
	if !basics.Applicable || basics.Score != 50 || basics.TotalWeight != 20 || basics.Earned != 10 {
		t.Fatalf("basics = %+v", basics)
	}
	if basics.Skipped != 1 {
		t.Fatalf("skipped = %d", basics.Skipped)
	}
	score, grade := overallScore(categories)
	if score != 60 || grade != "D" {
		t.Fatalf("overall = %d %s, want 60 D", score, grade)
	}
}

func TestGrades(t *testing.T) {
	t.Parallel()
	tests := map[int]string{100: "A", 90: "A", 89: "B", 80: "B", 79: "C", 70: "C", 69: "D", 60: "D", 59: "F", 0: "F"}
	for score, want := range tests {
		if got := GradeFor(score); got != want {
			t.Errorf("GradeFor(%d) = %q, want %q", score, got, want)
		}
	}
}

func TestResultOrderingPutsFailuresFirst(t *testing.T) {
	t.Parallel()
	report := analyzeFixture(t, "bad.md")
	for _, category := range Categories() {
		lastStatusRank := -1
		for _, result := range report.ResultsFor(category) {
			rank := statusRank(result.Status)
			if rank < lastStatusRank {
				t.Fatalf("%s results are not failed-first: %+v", category, report.ResultsFor(category))
			}
			lastStatusRank = rank
		}
	}
}

func expectStatus(t *testing.T, report Report, id string, want Status) {
	t.Helper()
	result := resultByID(t, report, id)
	if result.Status != want {
		t.Errorf("%s status = %q, want %q (detail: %s, evidence: %v)", id, result.Status, want, result.Detail, result.Evidence)
	}
}

func TestExternalLinkRuleSkipsLocalLinks(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Content = append(repository.Readme.Content,
		[]byte("\n\nOpen [the app](http://localhost:8080) or [127.0.0.1](http://127.0.0.1:8080/report).\n")...)

	checked := map[string]bool{}
	report := Analyze(context.Background(), repository, Options{
		CheckExternalLinks: true,
		LinkChecker:        recordingLinkChecker{seen: checked},
	})
	result := resultByID(t, report, "external-links")
	if result.Status != StatusPass {
		t.Fatalf("external-links = %+v", result)
	}
	if !strings.Contains(result.Detail, "2 local links") {
		t.Errorf("detail = %q, want a note about 2 skipped local links", result.Detail)
	}
	for target := range checked {
		if strings.Contains(target, "localhost") || strings.Contains(target, "127.0.0.1") {
			t.Errorf("local link %q was sent to the checker", target)
		}
	}
	if len(checked) == 0 {
		t.Error("public links were not checked")
	}
}

func TestExternalLinkRuleOnlyLocalLinks(t *testing.T) {
	t.Parallel()
	repository := repositoryFixture(t, "good.md")
	repository.Readme.Content = []byte("# Demo\n\nRun it, then open [localhost](http://localhost:8080).\n")
	report := Analyze(context.Background(), repository, Options{
		CheckExternalLinks: true,
		LinkChecker:        recordingLinkChecker{seen: map[string]bool{}},
	})
	result := resultByID(t, report, "external-links")
	if result.Status != StatusPass || !strings.Contains(result.Detail, "1 local link") {
		t.Fatalf("external-links = %+v", result)
	}
}

type recordingLinkChecker struct{ seen map[string]bool }

func (r recordingLinkChecker) Check(_ context.Context, urls []string) []LinkCheck {
	results := make([]LinkCheck, 0, len(urls))
	for _, target := range urls {
		r.seen[target] = true
		results = append(results, LinkCheck{URL: target, OK: true, StatusCode: 200})
	}
	return results
}
