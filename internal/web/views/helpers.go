package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/a-h/templ"

	"readmecheck/components/badge"
	"readmecheck/components/icon"
	"readmecheck/internal/github"
	"readmecheck/internal/rules"
)

// HomeData is the state of the landing page and its check form.
type HomeData struct {
	Title         string
	URL           string
	ExternalLinks bool
	Error         string
}

// ReportData bundles a report with the export formats used by the page.
type ReportData struct {
	Report        rules.Report
	Markdown      string
	Filename      string
	ExternalLinks bool
}

// NewReportData prepares the view model, including the Markdown export.
func NewReportData(report rules.Report, external bool) ReportData {
	name := "report"
	if repository := report.Repository; repository != nil {
		if repository.Ref.Owner != "" && repository.Ref.Repo != "" {
			name = repository.Ref.Owner + "-" + repository.Ref.Repo
		}
	}
	return ReportData{
		Report:        report,
		Markdown:      rules.ReportMarkdown(report),
		Filename:      "readmecheck-" + name + ".md",
		ExternalLinks: external,
	}
}

// HistoryData is the state of the local check history page.
type HistoryData struct {
	Checks []CheckSummary
	Error  string
}

// CheckSummary is the presentation form of a stored check.
type CheckSummary struct {
	Repository    string
	Description   string
	License       string
	Stars         int
	Score         int
	Grade         string
	CheckedAt     time.Time
	ExternalLinks bool
	Path          string
}

func repositoryName(report rules.Report) string {
	repository := report.Repository
	if repository == nil {
		return "repository"
	}
	if repository.FullName != "" {
		return repository.FullName
	}
	if repository.Ref.Owner != "" {
		return repository.Ref.FullName()
	}
	return "repository"
}

func repositoryURL(report rules.Report) string {
	repository := report.Repository
	if repository == nil {
		return ""
	}
	if repository.HTMLURL != "" {
		return repository.HTMLURL
	}
	return repository.Ref.URL()
}

func reportTitle(report rules.Report) string {
	return repositoryName(report) + " · README check"
}

func featureIcon(name string) templ.Component {
	return icon.Icon(name)(icon.Props{Class: "size-4"})
}

func statusLabel(status rules.Status) string {
	switch status {
	case rules.StatusPass:
		return "Passed"
	case rules.StatusFail:
		return "Failed"
	default:
		return "Skipped"
	}
}

func statusIconClass(status rules.Status) string {
	switch status {
	case rules.StatusPass:
		return "size-5 text-emerald-700 dark:text-emerald-400"
	case rules.StatusFail:
		return "size-5 text-destructive"
	default:
		return "size-5 text-muted-foreground"
	}
}

func statusPillClass(status rules.Status) string {
	switch status {
	case rules.StatusPass:
		return "border-emerald-700/20 text-emerald-700 dark:border-emerald-400/25 dark:text-emerald-400"
	case rules.StatusFail:
		return "border-destructive/20 text-destructive"
	default:
		return "text-muted-foreground"
	}
}

func statusItemClass(status rules.Status) string {
	return ""
}

func severityLabel(severity rules.Severity) string {
	if severity == "" {
		return "info"
	}
	return string(severity)
}

func severityClass(severity rules.Severity) string {
	switch severity {
	case rules.SeverityError:
		return "border-destructive/25 text-destructive"
	case rules.SeverityWarning:
		return "border-amber-600/30 text-amber-700 dark:text-amber-400"
	default:
		return "text-muted-foreground"
	}
}

func severityDotClass(severity rules.Severity) string {
	switch severity {
	case rules.SeverityError:
		return "bg-destructive"
	case rules.SeverityWarning:
		return "bg-amber-600 dark:bg-amber-400"
	default:
		return "bg-primary"
	}
}

func scoreTextClass(score int) string {
	switch {
	case score >= 90:
		return "text-emerald-700 dark:text-emerald-400"
	case score >= 70:
		return "text-amber-700 dark:text-amber-400"
	default:
		return "text-destructive"
	}
}

func gradePanelClass(score int) string {
	return "bg-muted/50"
}

func gradeDescription(grade string) string {
	switch grade {
	case "A":
		return "Excellent. The README answers the important questions and the repository shows healthy maintenance signals."
	case "B":
		return "Good. A few targeted fixes would make the README notably stronger."
	case "C":
		return "Usable, but important information or trust signals are missing."
	case "D":
		return "Weak. New visitors will struggle to understand, install, or trust the project."
	default:
		return "Critical gaps. Start with the failed error-severity checks below."
	}
}

func categoryScoreLabel(category rules.CategoryScore) string {
	if !category.Applicable {
		return "n/a"
	}
	return fmt.Sprintf("%d", category.Score)
}

func categoryCountsLabel(category rules.CategoryScore) string {
	if !category.Applicable {
		return "No applicable checks"
	}
	return fmt.Sprintf("%d of %d checks passed", category.Passed, category.Passed+category.Failed)
}

func categoryAnchorLabel(data ReportData, category rules.Category) string {
	for _, entry := range data.Report.Categories {
		if entry.Category == category {
			if !entry.Applicable {
				return "no applicable checks"
			}
			return fmt.Sprintf("%d/%d passed · %d%%", entry.Passed, entry.Passed+entry.Failed, entry.Score)
		}
	}
	return ""
}

func gradeScore(grade string) int {
	switch grade {
	case "A":
		return 95
	case "B":
		return 85
	case "C":
		return 75
	case "D":
		return 65
	default:
		return 45
	}
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func resultSubtitle(result rules.Result) string {
	return fmt.Sprintf("%s · %s severity · weight %d", result.Rule.ID, severityLabel(result.EffectiveSeverity()), result.Rule.Weight)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.Local().Format("2 Jan 2006, 15:04 MST")
}

func formatStars(stars int) string {
	switch {
	case stars >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(stars)/1_000_000)
	case stars >= 1_000:
		return fmt.Sprintf("%.1fk", float64(stars)/1_000)
	default:
		return fmt.Sprintf("%d", stars)
	}
}

func repositorySummary(repository *github.Repository) string {
	if repository == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if repository.License != nil && repository.License.Name != "" {
		parts = append(parts, repository.License.Name)
	}
	if repository.LatestRelease != nil && repository.LatestRelease.TagName != "" {
		parts = append(parts, "release "+repository.LatestRelease.TagName)
	}
	if repository.Archived {
		parts = append(parts, "archived")
	}
	return strings.Join(parts, " · ")
}

func historyPath(path string, external bool) string {
	if external {
		return path + "?external=1"
	}
	return path
}

func checkStatusSummary(result rules.Result) string {
	if result.Detail != "" {
		return result.Detail
	}
	return statusLabel(result.Status)
}

func statusBadgeVariant(status rules.Status) badge.Variant {
	switch status {
	case rules.StatusFail:
		return badge.VariantDestructive
	case rules.StatusPass:
		return badge.VariantSecondary
	default:
		return badge.VariantOutline
	}
}
