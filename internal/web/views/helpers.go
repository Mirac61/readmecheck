package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/a-h/templ"

	"readmecheck/components/badge"
	"readmecheck/components/chart"
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
	// PreviousScore is the score of the last stored check for the same
	// repository, when one is available. It powers the delta indicator.
	PreviousScore *int
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
		return "size-5 text-success"
	case rules.StatusFail:
		return "size-5 text-destructive"
	default:
		return "size-5 text-muted-foreground"
	}
}

func statusPillClass(status rules.Status) string {
	switch status {
	case rules.StatusPass:
		return "border-success/25 text-success"
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
		return "border-warning/30 text-warning"
	default:
		return "text-muted-foreground"
	}
}

func severityDotClass(severity rules.Severity) string {
	switch severity {
	case rules.SeverityError:
		return "bg-destructive"
	case rules.SeverityWarning:
		return "bg-warning"
	default:
		return "bg-primary"
	}
}

func scoreTextClass(score int) string {
	switch {
	case score >= 90:
		return "text-success"
	case score >= 70:
		return "text-warning"
	default:
		return "text-destructive"
	}
}

func scoreDeltaLabel(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("+%d", delta)
	case delta < 0:
		return fmt.Sprintf("%d", delta)
	default:
		return "0"
	}
}

func scoreDeltaText(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("+%d points since the last check", delta)
	case delta < 0:
		return fmt.Sprintf("%d points since the last check", delta)
	default:
		return "No change since the last check"
	}
}

func scoreDeltaClass(delta int) string {
	switch {
	case delta > 0:
		return "text-success"
	case delta < 0:
		return "text-destructive"
	default:
		return "text-muted-foreground"
	}
}

func gradePanelClass(score int) string {
	switch {
	case score >= 90:
		return "border-success/30 bg-success/10 text-success"
	case score >= 80:
		return "border-info/30 bg-info/10 text-info"
	case score >= 70:
		return "border-warning/30 bg-warning/10 text-warning"
	case score >= 60:
		return "border-caution/30 bg-caution/10 text-caution"
	default:
		return "border-destructive/25 bg-destructive/10 text-destructive"
	}
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

func checkDistributionChartData(counts rules.Counts) chart.Data {
	return chart.Data{
		Labels: []string{"Passed", "Failed", "Skipped"},
		Datasets: []chart.Dataset{
			{
				Label:           "Checks",
				Data:            []float64{float64(counts.Passed), float64(counts.Failed), float64(counts.Skipped)},
				BackgroundColor: []string{"#4f9a74", "#cf6b6b", "#8d8d97"},
				BorderColor:     []string{"#4f9a74", "#cf6b6b", "#8d8d97"},
				BorderWidth:     1,
			},
		},
	}
}

func categoryRadarChartData(categories []rules.CategoryScore) chart.Data {
	labels := make([]string, 0, len(categories))
	values := make([]float64, 0, len(categories))
	for _, cat := range categories {
		if !cat.Applicable {
			continue
		}
		label := cat.Label
		if label == "Trust & maintenance" {
			label = "Trust"
		}
		labels = append(labels, label)
		values = append(values, float64(cat.Score))
	}
	return chart.Data{
		Labels: labels,
		Datasets: []chart.Dataset{
			{
				Label:           "Category score",
				Data:            values,
				BackgroundColor: "rgba(113, 113, 122, 0.18)",
				BorderColor:     "#52525b",
				BorderWidth:     2,
				Fill:            true,
			},
		},
	}
}

func categoryBreakdownChartData(report rules.Report) chart.Data {
	labels := make([]string, 0, len(report.Categories))
	passed := make([]float64, 0, len(report.Categories))
	failed := make([]float64, 0, len(report.Categories))
	skipped := make([]float64, 0, len(report.Categories))
	for _, cat := range report.Categories {
		label := cat.Label
		if label == "Trust & maintenance" {
			label = "Trust"
		}
		labels = append(labels, label)
		passed = append(passed, float64(cat.Passed))
		failed = append(failed, float64(cat.Failed))
		skippedCount := 0
		for _, r := range report.ResultsFor(cat.Category) {
			if r.Status == rules.StatusSkipped {
				skippedCount++
			}
		}
		skipped = append(skipped, float64(skippedCount))
	}
	return chart.Data{
		Labels: labels,
		Datasets: []chart.Dataset{
			{
				Label:           "Passed",
				Data:            passed,
				BackgroundColor: "#4f9a74",
			},
			{
				Label:           "Failed",
				Data:            failed,
				BackgroundColor: "#cf6b6b",
			},
			{
				Label:           "Skipped",
				Data:            skipped,
				BackgroundColor: "#8d8d97",
			},
		},
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

// exampleRule returns catalog metadata for one rule so landing-page copy is
// drawn from the same source as the audit itself.
func exampleRule(id string) (rules.RuleSummary, bool) {
	for _, group := range rules.Catalog() {
		for _, rule := range group.Rules {
			if rule.ID == id {
				return rule, true
			}
		}
	}
	return rules.RuleSummary{}, false
}

// scoreBarClass colours a progress bar with the same natural tones as
// scoreTextClass, so a perfect score reads as green at a glance.
func scoreBarClass(score int) string {
	switch {
	case score >= 90:
		return "bg-success"
	case score >= 70:
		return "bg-warning"
	default:
		return "bg-destructive"
	}
}

func scoreTagClass(score int) string {
	switch {
	case score >= 90:
		return "border-success/30 bg-success/10 text-success"
	case score >= 70:
		return "border-warning/30 bg-warning/10 text-warning"
	default:
		return "border-destructive/30 bg-destructive/10 text-destructive"
	}
}

// dialogSizeClass sizes the feature dialogs. The rules dialog gets a fixed
// height so it does not jump when switching between tabs with different
// numbers of rules, and enough width for all seven tabs on one row.
func dialogSizeClass(id string) string {
	if id == "rules-dialog" {
		return "h-[min(85vh,42rem)] sm:max-w-4xl"
	}
	return "sm:max-w-2xl"
}
