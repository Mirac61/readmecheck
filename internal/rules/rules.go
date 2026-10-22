package rules

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"readmecheck/internal/github"
	"readmecheck/internal/markdown"
)

var ruleSet = []Rule{
	{
		ID:       "readme-exists",
		Title:    "README exists and has content",
		Category: CategoryBasics,
		Severity: SeverityError,
		Weight:   12,
		Why:      "The README is the front page of a repository. GitHub renders it below the file list, and many visitors decide whether to try a project from this page alone.",
		Fix:      "Add a README.md at the repository root with a title, a one-paragraph description, installation steps, and a usage example.",
		Check:    checkReadmeExists,
	},
	{
		ID:       "top-level-title",
		Title:    "README starts with a top-level title",
		Category: CategoryBasics,
		Severity: SeverityError,
		Weight:   6,
		Why:      "A clear H1 establishes the project name and gives the rendered README a meaningful heading for readers and assistive technology.",
		Fix:      "Start the file with `# Project name` (or an HTML `<h1>`), before badges, images, or prose.",
		Check:    checkTitle,
	},
	{
		ID:       "description-paragraph",
		Title:    "Opening paragraph explains the project",
		Category: CategoryBasics,
		Severity: SeverityWarning,
		Weight:   7,
		Why:      "Visitors should understand what the project does and who it is for within seconds. Badges and logos do not explain the purpose.",
		Fix:      "Directly below the title, add one to three plain-language sentences describing the problem the project solves and its intended audience.",
		Check:    checkDescriptionParagraph,
	},
	{
		ID:       "repo-description",
		Title:    "GitHub repository description is set",
		Category: CategoryBasics,
		Severity: SeverityWarning,
		Weight:   5,
		Why:      "GitHub shows the repository description in search results, repository lists, and social previews, often before anyone opens the README.",
		Fix:      "Set a concise description in the repository's About panel. Describe the project rather than repeating the repository name.",
		Check:    checkRepoDescription,
	},
	{
		ID:       "topics",
		Title:    "Repository topics are set",
		Category: CategoryBasics,
		Severity: SeverityInfo,
		Weight:   2,
		Why:      "Topics make the project discoverable through GitHub search and topic pages, and they signal its technology and domain at a glance.",
		Fix:      "Add three to eight specific topics in the About panel, such as `go`, `cli`, and the problem domain.",
		Check:    checkTopics,
	},
	{
		ID:       "installation-section",
		Title:    "Installation or setup section is present",
		Category: CategoryGettingStarted,
		Severity: SeverityError,
		Weight:   8,
		Why:      "People need a reliable path from cloning the repository to running the project before they can evaluate it.",
		Fix:      "Add a non-empty Installation, Setup, or Getting Started section with exact commands, package names, and any operating-system caveats.",
		Check:    checkInstallation,
	},
	{
		ID:       "usage-section",
		Title:    "Usage section is present",
		Category: CategoryGettingStarted,
		Severity: SeverityError,
		Weight:   8,
		Why:      "Installation alone does not show how to use the project. A minimal working invocation is the fastest way to turn an installer into a user.",
		Fix:      "Add a Usage (or Verwendung) section with the smallest useful command or code call, plus the expected result.",
		Check:    checkUsage,
	},
	{
		ID:       "code-example",
		Title:    "At least one fenced code example has a language tag",
		Category: CategoryGettingStarted,
		Severity: SeverityWarning,
		Weight:   6,
		Why:      "Fenced, language-tagged examples render with syntax highlighting and can be copied accurately, unlike pseudo-code or inline prose.",
		Fix:      "Add a fenced block such as ` ```bash ` or ` ```go ` containing a runnable snippet.",
		Check:    checkCodeExample,
	},
	{
		ID:       "prerequisites",
		Title:    "Prerequisites are mentioned",
		Category: CategoryGettingStarted,
		Severity: SeverityInfo,
		Weight:   3,
		Why:      "Runtime versions, required services, and credentials are common reasons a first run fails. Stating them up front prevents avoidable frustration.",
		Fix:      "Add a Prerequisites or Requirements section naming supported language versions, system packages, and external services.",
		Check:    checkPrerequisites,
	},
	{
		ID:       "visual",
		Title:    "README includes a screenshot, GIF, or Mermaid diagram",
		Category: CategoryVisuals,
		Severity: SeverityInfo,
		Weight:   5,
		Why:      "A visual shows the interface, output, or architecture faster than paragraphs and helps people decide whether the project matches their need.",
		Fix:      "Add a real screenshot or short GIF, or use a Mermaid diagram for architecture and flows. Store large assets under `docs/`.",
		Check:    checkVisual,
	},
	{
		ID:       "image-alt",
		Title:    "Images have alternative text",
		Category: CategoryVisuals,
		Severity: SeverityWarning,
		Weight:   5,
		Why:      "Alt text is what screen-reader users hear when an image cannot be perceived, and it keeps meaning available when images fail to load.",
		Fix:      "Give every Markdown or HTML image a short, descriptive alt attribute. Avoid filename-only alt text such as \"image\".",
		Check:    checkImageAlt,
	},
	{
		ID:       "license-file",
		Title:    "A license file exists",
		Category: CategoryTrust,
		Severity: SeverityError,
		Weight:   7,
		Why:      "Without an explicit license, potential users do not have clear permission to use, modify, or redistribute the code.",
		Fix:      "Add a LICENSE (or COPYING) file at the repository root using a standard license, and let GitHub detect it.",
		Check:    checkLicenseFile,
	},
	{
		ID:       "license-mentioned",
		Title:    "README mentions the license",
		Category: CategoryTrust,
		Severity: SeverityWarning,
		Weight:   4,
		Why:      "A License section or sentence makes reuse terms visible to readers and reduces ambiguity about how the project can be used.",
		Fix:      "Add a License section naming the license and linking to the LICENSE file, for example \"Released under the MIT License\".",
		Check:    checkLicenseMentioned,
	},
	{
		ID:       "ci-workflow",
		Title:    "A CI workflow exists",
		Category: CategoryTrust,
		Severity: SeverityWarning,
		Weight:   5,
		Why:      "Automated checks signal that changes are tested and make it easier for contributors to trust the default branch.",
		Fix:      "Add a GitHub Actions workflow under `.github/workflows/` that runs the test suite on pushes and pull requests.",
		Check:    checkCIWorkflow,
	},
	{
		ID:       "badges-valid",
		Title:    "CI badges point to existing workflows",
		Category: CategoryTrust,
		Severity: SeverityWarning,
		Weight:   5,
		Why:      "A badge that references a missing workflow becomes a false claim about project health and undermines otherwise useful signals.",
		Fix:      "Point the badge at an existing workflow file, or remove stale CI badges. The badge URL should contain the same filename as the workflow.",
		Check:    checkBadges,
	},
	{
		ID:       "not-archived",
		Title:    "Repository is not archived",
		Category: CategoryTrust,
		Severity: SeverityInfo,
		Weight:   1,
		Why:      "Archived repositories are read-only. Visitors should know immediately whether the project is maintained or preserved for reference.",
		Fix:      "Unarchive the repository if it is actively maintained. If it is intentionally dormant, state that prominently and link to a successor.",
		Check:    checkNotArchived,
	},
	{
		ID:       "contributing",
		Title:    "Contribution guidance is available",
		Category: CategoryContribution,
		Severity: SeverityInfo,
		Weight:   4,
		Why:      "Clear contribution guidance reduces friction for bug reports and pull requests, and tells people how to participate respectfully.",
		Fix:      "Add CONTRIBUTING.md (including `.github/CONTRIBUTING.md`) or a short Contributing section covering issues, PRs, tests, and conduct.",
		Check:    checkContributing,
	},
	{
		ID:       "no-empty-sections",
		Title:    "No headings are followed directly by another heading",
		Category: CategoryHygiene,
		Severity: SeverityWarning,
		Weight:   4,
		Why:      "Empty sections look unfinished and force readers to guess whether content is missing or the heading is merely decorative.",
		Fix:      "Fill each heading with useful content or remove it. At minimum, add a sentence explaining what belongs in the section.",
		Check:    checkEmptySections,
	},
	{
		ID:       "no-placeholders",
		Title:    "No leftover template placeholders",
		Category: CategoryHygiene,
		Severity: SeverityError,
		Weight:   6,
		Why:      "Placeholders such as TODO, Lorem ipsum, or \"Project Title\" signal an unedited template and can embarrass a project before anyone runs the code.",
		Fix:      "Replace every placeholder with project-specific content, then search the README for TODO, Lorem ipsum, Project Title, and similar template strings.",
		Check:    checkPlaceholders,
	},
	{
		ID:       "reasonable-length",
		Title:    "README length is appropriate",
		Category: CategoryHygiene,
		Severity: SeverityWarning,
		Weight:   3,
		Why:      "A very short README leaves essential questions unanswered, while an extremely long one is hard to scan and often duplicates generated documentation.",
		Fix:      "Aim for a focused README covering purpose, installation, usage, configuration, and contribution. Move deep reference material into `docs/`.",
		Check:    checkLength,
	},
	{
		ID:       "table-of-contents",
		Title:    "Long READMEs have a table of contents",
		Category: CategoryHygiene,
		Severity: SeverityInfo,
		Weight:   2,
		Why:      "Readers scan long documents. A table of contents exposes the structure early and makes sections reachable without a long scroll.",
		Fix:      "Add a Table of Contents section with anchor links to the main H2/H3 headings, or use a maintained TOC generator.",
		Check:    checkTableOfContents,
	},
	{
		ID:       "relative-links",
		Title:    "Relative links point to existing repository paths",
		Category: CategoryLinks,
		Severity: SeverityError,
		Weight:   8,
		Why:      "Relative links are the main way READMEs point to guides, images, examples, and policy files. A moved file silently turns them into dead ends.",
		Fix:      "Update each relative path to match the current repository tree. Remember that README links are resolved from the repository root.",
		Check:    checkRelativeLinks,
	},
	{
		ID:       "external-links",
		Title:    "External links are reachable",
		Category: CategoryLinks,
		Severity: SeverityWarning,
		Weight:   5,
		Why:      "Dead external links hurt trust and hide useful references. Link checks are optional because they are slower than local analysis.",
		Fix:      "Remove or redirect unreachable links. Prefer stable project documentation, archive links, and canonical homepages.",
		Check:    checkExternalLinks,
	},
}

func Rules() []Rule {
	return append([]Rule(nil), ruleSet...)
}

func passed(detail string, evidence ...string) Result {
	return Result{Status: StatusPass, Detail: detail, Evidence: evidence}
}

func failed(detail string, evidence ...string) Result {
	return Result{Status: StatusFail, Detail: detail, Evidence: evidence}
}

func skipped(detail string) Result {
	return Result{Status: StatusSkipped, Detail: detail}
}

func hasReadme(input Input) bool {
	return input.Repository != nil && input.Repository.Readme != nil && strings.TrimSpace(string(input.Repository.Readme.Content)) != ""
}

func checkReadmeExists(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if !input.Repository.ReadmeKnown {
		return skipped("README availability could not be determined.")
	}
	if input.Repository.Readme == nil {
		return failed("GitHub returned no README for this repository.")
	}
	if strings.TrimSpace(string(input.Repository.Readme.Content)) == "" {
		return failed("The README file is empty.")
	}
	return passed(fmt.Sprintf("%s contains %d words.", input.Repository.Readme.Name, input.Document.Words))
}

func checkTitle(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if strings.TrimSpace(input.Document.Title) == "" {
		return failed("No H1 title was found in the README.")
	}
	return passed(fmt.Sprintf("Found the title %q.", input.Document.Title))
}

func checkDescriptionParagraph(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	paragraph, ok := input.Document.FirstIntroParagraph()
	if !ok {
		return failed("No paragraph was found between the title and the first subheading.")
	}
	if !paragraph.HasProse() {
		return failed("The opening paragraph is too short or contains only badges/links.", excerpt(paragraph.Text))
	}
	return passed("The opening paragraph explains the project.", excerpt(paragraph.Text))
}

func checkRepoDescription(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if strings.TrimSpace(input.Repository.Description) == "" {
		return failed("The GitHub repository description is empty.")
	}
	return passed("Repository description is set.", excerpt(input.Repository.Description))
}

func checkTopics(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if len(input.Repository.Topics) == 0 {
		return failed("No repository topics are set.")
	}
	return passed(fmt.Sprintf("Found %d topics.", len(input.Repository.Topics)), strings.Join(input.Repository.Topics, ", "))
}

var (
	installationKeywords = []string{
		"installation", "install", "setup", "set up", "getting started", "quickstart", "quick start",
		"einrichtung", "installation", "installieren", "erste schritte", "schnellstart",
	}
	usageKeywords = []string{
		"usage", "how to use", "using", "use", "verwendung", "benutzung", "nutzung",
		"anwendung", "gebrauch", "verwenden", "so funktioniert",
	}
	prerequisiteKeywords = []string{
		"prerequisites", "prerequisite", "requirements", "requirement", "dependencies",
		"before you begin", "what you need", "voraussetzungen", "voraussetzung",
		"anforderungen", "abhangigkeiten", "abhängigkeiten", "benotigt", "benötigt",
	}
	contributingKeywords = []string{
		"contributing", "contribution", "contributions", "development", "developing",
		"mitwirken", "beitragen", "beitrag", "entwicklung", "entwickeln",
	}
	licenseKeywords = []string{
		"license", "licence", "licensing", "lizenz", "rechtliches", "rechtliche hinweise",
	}
	tocKeywords = []string{"table of contents", "contents", "inhalt", "inhaltsverzeichnis"}
)

func checkInstallation(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	heading, ok := firstMatchingHeading(input.Document, installationKeywords)
	if !ok {
		return failed("No installation, setup, or getting-started heading was found.")
	}
	if heading.Empty {
		return failed(fmt.Sprintf("The %q section has no content.", heading.Text))
	}
	return passed(fmt.Sprintf("Found the %q section.", heading.Text))
}

func checkUsage(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	heading, ok := firstMatchingHeading(input.Document, usageKeywords)
	if !ok {
		return failed("No usage or how-to-use heading was found.")
	}
	if heading.Empty {
		return failed(fmt.Sprintf("The %q section has no content.", heading.Text))
	}
	return passed(fmt.Sprintf("Found the %q section.", heading.Text))
}

func checkCodeExample(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if !input.Document.HasCodeExample() {
		return failed("No non-empty fenced code block with a language tag was found.")
	}
	language := ""
	for _, block := range input.Document.CodeBlocks {
		if block.Fenced && block.Language != "" && !strings.EqualFold(block.Language, "mermaid") {
			language = block.Language
			break
		}
	}
	return passed(fmt.Sprintf("Found a fenced %s example.", language))
}

func checkPrerequisites(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if heading, ok := firstMatchingHeading(input.Document, prerequisiteKeywords); ok && !heading.Empty {
		return passed(fmt.Sprintf("Found the %q section.", heading.Text))
	}
	if prerequisitesMentionedPattern.MatchString(input.Document.Raw) {
		return passed("Prerequisites or runtime requirements are mentioned in the README.")
	}
	return failed("No prerequisites or runtime requirements are mentioned.")
}

var prerequisitesMentionedPattern = regexp.MustCompile(`(?i)\b(?:prerequisites?|requirements?|dependencies|voraussetzungen?|anforderungen?|abhängigkeiten?|benötigt)\b|\brequires?\s+(?:go|node(?:\.js)?|python|rust|java|docker|git|a\s+(?:recent|modern)|at\s+least)\b`)

func checkVisual(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if input.Document.HasVisual() {
		detail := "Found a screenshot, GIF, or Mermaid diagram."
		if input.Document.HasMermaid {
			detail = "Found a Mermaid diagram."
		}
		return passed(detail)
	}
	return failed("No screenshot, GIF, or Mermaid diagram was detected.")
}

func checkImageAlt(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if len(input.Document.Images) == 0 {
		return skipped("The README has no images.")
	}
	var missing []string
	for _, image := range input.Document.Images {
		if strings.TrimSpace(image.Alt) == "" {
			missing = append(missing, image.Destination)
		}
	}
	if len(missing) > 0 {
		return failed(fmt.Sprintf("%d of %d images have no alt text.", len(missing), len(input.Document.Images)), limitEvidence(missing, 5)...)
	}
	return passed(fmt.Sprintf("All %d images have alt text.", len(input.Document.Images)))
}

func checkLicenseFile(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if !input.Repository.RootKnown {
		return skipped("The repository root listing could not be loaded.")
	}
	if hasLicenseFile(input.Repository) {
		name := licenseFileName(input.Repository)
		if input.Repository.License != nil && input.Repository.License.SPDXID != "" {
			return passed(fmt.Sprintf("Found %s (%s).", name, input.Repository.License.SPDXID))
		}
		return passed(fmt.Sprintf("Found %s.", name))
	}
	return failed("No LICENSE, LICENCE, or COPYING file was found at the repository root.")
}

func checkLicenseMentioned(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if heading, ok := firstMatchingHeading(input.Document, licenseKeywords); ok && !heading.Empty {
		return passed(fmt.Sprintf("Found the %q section.", heading.Text))
	}
	if licenseMentionPattern.MatchString(input.Document.Raw) {
		return passed("The README mentions licensing terms.")
	}
	if input.Repository != nil && input.Repository.License != nil && input.Repository.License.Name != "" &&
		strings.Contains(strings.ToLower(input.Document.Raw), strings.ToLower(input.Repository.License.Name)) {
		return passed(fmt.Sprintf("The README mentions %s.", input.Repository.License.Name))
	}
	return failed("The README does not contain a License section or mention the license.")
}

var licenseMentionPattern = regexp.MustCompile(`(?i)\b(?:licen[cs]e|licensing|lizenz|rechtliche\s+hinweise)\b`)

func checkCIWorkflow(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if !input.Repository.WorkflowsKnown {
		return skipped("GitHub Actions workflow information is unavailable.")
	}
	if len(input.Repository.Workflows) == 0 {
		return failed("No GitHub Actions workflows were found.")
	}
	names := make([]string, 0, len(input.Repository.Workflows))
	for _, workflow := range input.Repository.Workflows {
		names = append(names, workflow.Name)
	}
	return passed(fmt.Sprintf("Found %d workflow(s).", len(names)), strings.Join(names, ", "))
}

func checkBadges(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	references := input.Document.CIWorkflowReferences()
	if len(references) == 0 {
		return skipped("No GitHub Actions CI badge was found.")
	}
	if input.Repository == nil || !input.Repository.WorkflowsKnown {
		return skipped("Workflow information is unavailable, so badge targets cannot be verified.")
	}

	ownReferences := make([]string, 0)
	for _, reference := range references {
		parts := strings.SplitN(reference, "/", 3)
		if len(parts) < 3 {
			continue
		}
		if !strings.EqualFold(parts[0], input.Repository.Ref.Owner) || !strings.EqualFold(parts[1], input.Repository.Ref.Repo) {
			continue
		}
		ownReferences = append(ownReferences, parts[2])
	}
	if len(ownReferences) == 0 {
		return skipped("No CI badge for this repository was found.")
	}

	var missing []string
	for _, workflowPath := range ownReferences {
		if _, ok := input.Repository.WorkflowByPath(workflowPath); !ok {
			missing = append(missing, workflowPath)
		}
	}
	if len(missing) > 0 {
		return failed("CI badges reference workflow paths that do not exist.", missing...)
	}
	return passed(fmt.Sprintf("All %d CI badge workflow target(s) exist.", len(ownReferences)))
}

func checkNotArchived(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	if input.Repository.Archived {
		return failed("The repository is archived and read-only.")
	}
	return passed("The repository is not archived.")
}

func checkContributing(_ context.Context, input Input, _ Options) Result {
	if input.Repository == nil {
		return skipped("No repository snapshot is available.")
	}
	result := Result{}
	severity := SeverityInfo
	if input.Repository.Stars >= 1000 {
		severity = SeverityWarning
	}
	if hasContributingFile(input.Repository) {
		result = passed("A CONTRIBUTING file is present.")
	} else if hasReadme(input) {
		if heading, ok := firstMatchingHeading(input.Document, contributingKeywords); ok && !heading.Empty {
			result = passed(fmt.Sprintf("Found the %q section.", heading.Text))
		}
	}
	if result.Status == "" {
		result = failed("No CONTRIBUTING file or contribution section was found.")
	}
	result.Severity = severity
	return result
}

func checkEmptySections(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	empty := input.Document.EmptySections()
	if len(empty) == 0 {
		return passed("No empty sections were found.")
	}
	evidence := make([]string, 0, len(empty))
	for _, heading := range empty {
		evidence = append(evidence, fmt.Sprintf("%q is followed directly by another heading.", heading))
	}
	return failed(fmt.Sprintf("%d section(s) are empty.", len(empty)), limitEvidence(evidence, 6)...)
}

func checkPlaceholders(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	markers := findPlaceholders(input.Document.Raw)
	if len(markers) == 0 {
		return passed("No template placeholders were found.")
	}
	return failed(fmt.Sprintf("Found %d template placeholder marker(s).", len(markers)), limitEvidence(markers, 6)...)
}

func checkLength(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	if input.Document.Words < 60 {
		return failed(fmt.Sprintf("Only %d words. Add enough context to install and use the project.", input.Document.Words))
	}
	if input.Document.Words > 20000 || input.Document.Chars > 150000 {
		return failed(fmt.Sprintf("Very long: %d words and %d characters. Move reference material into docs/.", input.Document.Words, input.Document.Chars))
	}
	return passed(fmt.Sprintf("%d words and %d characters.", input.Document.Words, input.Document.Chars))
}

func checkTableOfContents(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	sections := input.Document.HeadingCountAtLeast(2, 3)
	if sections < 8 {
		return passed(fmt.Sprintf("The README has %d main sections; a TOC is not necessary.", sections))
	}
	if input.Document.HasTOC() {
		return passed(fmt.Sprintf("A table of contents covers the README's %d main sections.", sections))
	}
	return failed(fmt.Sprintf("The README has %d main sections but no table of contents.", sections))
}

func checkRelativeLinks(_ context.Context, input Input, _ Options) Result {
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	links := input.Document.RelativeLinks()
	if len(links) == 0 {
		return passed("No relative links to verify.")
	}
	if input.Repository == nil || !input.Repository.TreeKnown {
		return skipped("The repository file tree is unavailable, so relative links cannot be verified.")
	}
	var missing []string
	for _, link := range links {
		target, ok := resolveRelativeLink(link.Destination)
		if !ok {
			missing = append(missing, link.Destination+" (invalid path)")
			continue
		}
		if target == "." {
			continue
		}
		if !input.Repository.HasFile(target) {
			missing = append(missing, link.Destination)
		}
	}
	if len(missing) > 0 {
		return failed(fmt.Sprintf("%d of %d relative links point to missing paths.", len(missing), len(links)), limitEvidence(missing, 6)...)
	}
	return passed(fmt.Sprintf("All %d relative links point to existing paths.", len(links)))
}

func checkExternalLinks(ctx context.Context, input Input, options Options) Result {
	if !options.CheckExternalLinks {
		return skipped("External link checking is disabled for this run.")
	}
	if !hasReadme(input) {
		return skipped("No readable README to inspect.")
	}
	links := input.Document.ExternalLinks()
	if len(links) == 0 {
		return passed("No external links to check.")
	}
	urls := make([]string, 0, len(links))
	for _, link := range links {
		urls = append(urls, link.Destination)
	}
	capped := 0
	if len(urls) > DefaultMaxLinkChecks {
		capped = len(urls) - DefaultMaxLinkChecks
		urls = urls[:DefaultMaxLinkChecks]
	}
	if options.LinkChecker == nil {
		return skipped("No external link checker is configured.")
	}
	checks := options.LinkChecker.Check(ctx, urls)
	var failures []LinkCheck
	for _, check := range checks {
		if !check.OK {
			failures = append(failures, check)
		}
	}
	sort.SliceStable(failures, func(i, j int) bool { return failures[i].URL < failures[j].URL })
	if len(failures) > 0 {
		evidence := make([]string, 0, len(failures))
		for _, failure := range failures {
			evidence = append(evidence, failure.String())
		}
		detail := fmt.Sprintf("%d of %d external links were unreachable.", len(failures), len(urls))
		if capped > 0 {
			detail += fmt.Sprintf(" %d additional links were not checked.", capped)
		}
		return failed(detail, limitEvidence(evidence, 6)...)
	}
	detail := fmt.Sprintf("All %d external links were reachable.", len(urls))
	if capped > 0 {
		detail += fmt.Sprintf(" %d additional links were not checked.", capped)
	}
	return passed(detail)
}

func firstMatchingHeading(document markdown.Document, keywords []string) (markdown.Heading, bool) {
	for _, heading := range document.Headings {
		if markdown.HeadingMatches(heading.Text, keywords...) {
			return heading, true
		}
	}
	return markdown.Heading{}, false
}

func hasLicenseFile(repository *github.Repository) bool {
	return repository.RootEntryPrefix("license") || repository.RootEntryPrefix("licence") ||
		repository.RootEntryPrefix("copying") || repository.HasRootEntry("COPYRIGHT")
}

func licenseFileName(repository *github.Repository) string {
	for _, entry := range repository.Root {
		lower := strings.ToLower(entry.Name)
		if strings.HasPrefix(lower, "license") || strings.HasPrefix(lower, "licence") || strings.HasPrefix(lower, "copying") {
			return entry.Name
		}
	}
	return "a license file"
}

func hasContributingFile(repository *github.Repository) bool {
	if repository.RootEntryPrefix("contributing") {
		return true
	}
	for file := range repository.Files {
		base := strings.ToLower(path.Base(file))
		if strings.HasPrefix(base, "contributing") {
			return true
		}
	}
	return false
}

func resolveRelativeLink(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", false
	}
	value := parsed.Path
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return "", false
	}
	cleaned := path.Clean(value)
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", false
	}
	return cleaned, true
}

func findPlaceholders(raw string) []string {
	var markers []string
	if regexp.MustCompile(`(?im)\bTODO\b`).MatchString(raw) {
		markers = append(markers, lineWithMarker(raw, "TODO"))
	}
	lower := strings.ToLower(raw)
	for _, marker := range []string{"lorem ipsum", "project title", "one paragraph of project description", "insert description here", "your project name"} {
		if strings.Contains(lower, marker) {
			markers = append(markers, lineWithMarker(raw, marker))
		}
	}
	result := make([]string, 0, len(markers))
	seen := make(map[string]bool)
	for _, marker := range markers {
		if marker != "" && !seen[marker] {
			seen[marker] = true
			result = append(result, marker)
		}
	}
	return result
}

func lineWithMarker(raw, marker string) string {
	lowerMarker := strings.ToLower(marker)
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(strings.ToLower(line), lowerMarker) {
			return excerpt(line)
		}
	}
	return marker
}

func excerpt(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:157]) + "..."
	}
	return value
}

func limitEvidence(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	result := append([]string(nil), values[:limit]...)
	result = append(result, fmt.Sprintf("…and %d more.", len(values)-limit))
	return result
}
