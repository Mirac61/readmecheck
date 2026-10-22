package rules

import (
	"context"
	"time"

	"readmecheck/internal/github"
	"readmecheck/internal/markdown"
)

type Category string

const (
	CategoryBasics         Category = "basics"
	CategoryGettingStarted Category = "getting-started"
	CategoryVisuals        Category = "visuals"
	CategoryTrust          Category = "trust"
	CategoryContribution   Category = "contribution"
	CategoryHygiene        Category = "hygiene"
	CategoryLinks          Category = "links"
)

var categoryOrder = []Category{
	CategoryBasics,
	CategoryGettingStarted,
	CategoryVisuals,
	CategoryTrust,
	CategoryContribution,
	CategoryHygiene,
	CategoryLinks,
}

var categoryLabels = map[Category]string{
	CategoryBasics:         "Basics",
	CategoryGettingStarted: "Getting started",
	CategoryVisuals:        "Visuals",
	CategoryTrust:          "Trust & maintenance",
	CategoryContribution:   "Contribution",
	CategoryHygiene:        "Hygiene",
	CategoryLinks:          "Links",
}

func Categories() []Category {
	return append([]Category(nil), categoryOrder...)
}

func CategoryLabel(category Category) string {
	if label, ok := categoryLabels[category]; ok {
		return label
	}
	return string(category)
}

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

type Result struct {
	Rule     Rule
	Status   Status
	Severity Severity
	Detail   string
	Evidence []string
}

func (r Result) EffectiveSeverity() Severity {
	if r.Severity != "" {
		return r.Severity
	}
	return r.Rule.Severity
}

type Rule struct {
	ID       string
	Title    string
	Category Category
	Severity Severity
	Weight   int
	Why      string
	Fix      string
	Check    func(context.Context, Input, Options) Result
}

// Analyzer is an extension point for analyses that are not deterministic
// rules, such as a future optional LLM reviewer. The rule engine never invokes
// an Analyzer; callers would run it alongside Analyze and merge the findings.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, input Input) ([]Result, error)
}

type Input struct {
	Repository *github.Repository
	Document   markdown.Document
}

type Options struct {
	CheckExternalLinks bool
	LinkChecker        LinkChecker
	Now                func() time.Time
}

type CategoryScore struct {
	Category    Category
	Label       string
	Score       int
	Applicable  bool
	Earned      int
	TotalWeight int
	Passed      int
	Failed      int
	Skipped     int
}

type Counts struct {
	Passed  int
	Failed  int
	Skipped int
}

type Report struct {
	Repository           *github.Repository
	Results              []Result
	Categories           []CategoryScore
	Score                int
	Grade                string
	GeneratedAt          time.Time
	ExternalLinksChecked bool
	Counts               Counts
}

// Analyze parses the README and runs the deterministic rule engine.
func Analyze(ctx context.Context, repository *github.Repository, options Options) Report {
	var document markdown.Document
	if repository != nil && repository.Readme != nil {
		document = markdown.Parse(repository.Readme.Content)
	}
	input := Input{Repository: repository, Document: document}

	if options.Now == nil {
		options.Now = time.Now
	}
	if options.LinkChecker == nil {
		options.LinkChecker = NewHTTPLinkChecker()
	}

	results := make([]Result, 0, len(ruleSet))
	for _, rule := range ruleSet {
		result := rule.Check(ctx, input, options)
		result.Rule = rule
		results = append(results, result)
	}
	sortResults(results)

	report := Report{
		Repository:           repository,
		Results:              results,
		Categories:           scoreCategories(results),
		GeneratedAt:          options.Now(),
		ExternalLinksChecked: options.CheckExternalLinks,
	}
	report.Score, report.Grade = overallScore(report.Categories)
	report.Counts = countResults(results)
	return report
}

func (r Report) ResultsFor(category Category) []Result {
	result := make([]Result, 0)
	for _, entry := range r.Results {
		if entry.Rule.Category == category {
			result = append(result, entry)
		}
	}
	return result
}

func (r Report) Errors() []Result {
	result := make([]Result, 0)
	for _, entry := range r.Results {
		if entry.Status == StatusFail && entry.EffectiveSeverity() == SeverityError {
			result = append(result, entry)
		}
	}
	return result
}
