package markdown

import (
	"fmt"
	htmlstd "html"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

type Document struct {
	Raw        string
	Title      string
	Headings   []Heading
	Paragraphs []Paragraph
	CodeBlocks []CodeBlock
	Links      []Link
	Images     []Image
	HasMermaid bool
	HTML       string
	Words      int
	Lines      int
	Chars      int
}

type Heading struct {
	Text  string
	Level int
	Empty bool
}

type Paragraph struct {
	Text       string
	Section    string
	Intro      bool
	ImageCount int
	LinkCount  int
	Words      int
}

type CodeBlock struct {
	Language string
	Content  string
	Section  string
	Fenced   bool
}

type Link struct {
	Text        string
	Destination string
	Section     string
	FromHTML    bool
}

type Image struct {
	Alt         string
	Destination string
	Section     string
	FromHTML    bool
}

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

var (
	imgTagPattern    = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	anchorTagPattern = regexp.MustCompile(`(?is)<a\b[^>]*>`)
	htmlTagPattern   = regexp.MustCompile(`(?is)<[^>]+>`)
	h1Pattern        = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	attrPattern      = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>]+))`)
)

func Parse(source []byte) Document {
	raw := string(source)
	doc := Document{
		Raw:   raw,
		Chars: len(source),
		Lines: countLines(raw),
		Words: len(strings.Fields(raw)),
	}
	root := markdownParser.Parser().Parse(text.NewReader(source))

	currentSection := ""
	seenSubheading := false
	htmlParts := make([]string, 0)

	pendingHeading := -1
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		if heading, ok := node.(*ast.Heading); ok {
			if pendingHeading >= 0 {
				doc.Headings[pendingHeading].Empty = true
			}
			text := cleanInlineText(visibleText(heading, source))
			doc.Headings = append(doc.Headings, Heading{
				Text:  text,
				Level: heading.Level,
			})
			pendingHeading = len(doc.Headings) - 1
			currentSection = text
			if heading.Level >= 2 {
				seenSubheading = true
			}
			if doc.Title == "" && heading.Level == 1 {
				doc.Title = text
			}
			collectInline(heading, source, &doc, nil, currentSection)
			continue
		}

		if isContentNode(node) {
			pendingHeading = -1
		}

		switch n := node.(type) {
		case *ast.Paragraph:
			text := cleanInlineText(visibleText(n, source))
			paragraph := Paragraph{
				Text:    text,
				Section: currentSection,
				Intro:   !seenSubheading,
				Words:   len(strings.Fields(text)),
			}
			collectInline(n, source, &doc, &paragraph, currentSection)
			doc.Paragraphs = append(doc.Paragraphs, paragraph)
		case *ast.List:
			collectInline(n, source, &doc, nil, currentSection)
		case *ast.Blockquote:
			collectInline(n, source, &doc, nil, currentSection)
		case *extast.Table:
			collectInline(n, source, &doc, nil, currentSection)
		case *ast.FencedCodeBlock:
			code := CodeBlock{
				Language: normaliseLanguage(string(n.Language(source))),
				Content:  string(n.Lines().Value(source)),
				Section:  currentSection,
				Fenced:   true,
			}
			doc.CodeBlocks = append(doc.CodeBlocks, code)
			if strings.EqualFold(code.Language, "mermaid") {
				doc.HasMermaid = true
			}
		case *ast.CodeBlock:
			doc.CodeBlocks = append(doc.CodeBlocks, CodeBlock{
				Content: string(n.Lines().Value(source)),
				Section: currentSection,
			})
		}
	}

	ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			collectHTML(node, source, &htmlParts)
		}
		return ast.WalkContinue, nil
	})

	if pendingHeading >= 0 {
		doc.Headings[pendingHeading].Empty = true
	}

	doc.HTML = strings.Join(htmlParts, "\n")
	parseHTML(&doc)

	if doc.Title == "" {
		doc.Title = htmlTitle(doc.HTML)
	}
	doc.HasMermaid = doc.HasMermaid || hasMermaidHTML(doc.HTML)
	return doc
}

func isContentNode(node ast.Node) bool {
	if _, ok := node.(*ast.Heading); ok {
		return false
	}
	if _, ok := node.(*ast.ThematicBreak); ok {
		return false
	}
	return true
}

func collectInline(node ast.Node, source []byte, doc *Document, paragraph *Paragraph, section string) {
	ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := child.(type) {
		case *ast.Link:
			doc.Links = append(doc.Links, Link{
				Text:        cleanInlineText(visibleText(n, source)),
				Destination: string(n.Destination),
				Section:     section,
			})
			if paragraph != nil {
				paragraph.LinkCount++
			}
		case *ast.AutoLink:
			doc.Links = append(doc.Links, Link{
				Text:        cleanInlineText(string(n.Label(source))),
				Destination: string(n.URL(source)),
				Section:     section,
			})
			if paragraph != nil {
				paragraph.LinkCount++
			}
		case *ast.Image:
			doc.Images = append(doc.Images, Image{
				Alt:         cleanInlineText(imageAltText(n, source)),
				Destination: string(n.Destination),
				Section:     section,
			})
			if paragraph != nil {
				paragraph.ImageCount++
			}
		}
		return ast.WalkContinue, nil
	})
}

func collectHTML(node ast.Node, source []byte, parts *[]string) {
	switch n := node.(type) {
	case *ast.RawHTML:
		var value strings.Builder
		for i := 0; i < n.Segments.Len(); i++ {
			segment := n.Segments.At(i)
			value.Write(segment.Value(source))
		}
		*parts = append(*parts, value.String())
	case *ast.HTMLBlock:
		for i := 0; i < n.Lines().Len(); i++ {
			segment := n.Lines().At(i)
			(*parts) = append(*parts, string(segment.Value(source)))
		}
		if n.HasClosure() {
			segment := n.ClosureLine
			(*parts) = append(*parts, string(segment.Value(source)))
		}
	}
}

func visibleText(node ast.Node, source []byte) string {
	var builder strings.Builder
	var walk func(ast.Node)
	walk = func(current ast.Node) {
		switch n := current.(type) {
		case *ast.Image:
			return
		case *ast.RawHTML:
			return
		case *ast.HTMLBlock:
			return
		case *ast.Text:
			builder.Write(n.Segment.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				builder.WriteByte(' ')
			}
			return
		case *ast.String:
			builder.Write(n.Value)
			return
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func imageAltText(node ast.Node, source []byte) string {
	var builder strings.Builder
	var walk func(ast.Node)
	walk = func(current ast.Node) {
		switch n := current.(type) {
		case *ast.Text:
			builder.Write(n.Segment.Value(source))
			return
		case *ast.String:
			builder.Write(n.Value)
			return
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		walk(child)
	}
	return builder.String()
}

func cleanInlineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func normaliseLanguage(info string) string {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.Trim(fields[0], "{}. "))
}

func countLines(raw string) int {
	if raw == "" {
		return 0
	}
	return strings.Count(raw, "\n") + 1
}

func parseHTML(doc *Document) {
	for _, tag := range imgTagPattern.FindAllString(doc.HTML, -1) {
		attrs := parseAttributes(tag)
		doc.Images = append(doc.Images, Image{
			Alt:         strings.TrimSpace(htmlstd.UnescapeString(attrs["alt"])),
			Destination: strings.TrimSpace(htmlstd.UnescapeString(attrs["src"])),
			FromHTML:    true,
		})
	}
	for _, tag := range anchorTagPattern.FindAllString(doc.HTML, -1) {
		attrs := parseAttributes(tag)
		href := strings.TrimSpace(htmlstd.UnescapeString(attrs["href"]))
		if href == "" {
			continue
		}
		doc.Links = append(doc.Links, Link{
			Destination: href,
			FromHTML:    true,
		})
	}
}

func parseAttributes(tag string) map[string]string {
	attributes := make(map[string]string)
	for _, match := range attrPattern.FindAllStringSubmatch(tag, -1) {
		name := strings.ToLower(match[1])
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if value == "" {
			value = match[4]
		}
		attributes[name] = value
	}
	return attributes
}

func htmlTitle(htmlText string) string {
	match := h1Pattern.FindStringSubmatch(htmlText)
	if len(match) < 2 {
		return ""
	}
	text := htmlTagPattern.ReplaceAllString(match[1], " ")
	return cleanInlineText(htmlstd.UnescapeString(text))
}

func hasMermaidHTML(htmlText string) bool {
	for _, tag := range regexp.MustCompile(`(?is)<(?:div|pre|code)\b[^>]*>`).FindAllString(htmlText, -1) {
		attrs := parseAttributes(tag)
		if strings.Contains(strings.ToLower(attrs["class"]), "mermaid") {
			return true
		}
	}
	return false
}

// HasHeading returns true when at least one heading matches a keyword. Matching
// is case-insensitive, ignores punctuation, and supports English/German terms.
func (d Document) HasHeading(keywords ...string) bool {
	for _, heading := range d.Headings {
		if HeadingMatches(heading.Text, keywords...) {
			return true
		}
	}
	return false
}

func HeadingMatches(text string, keywords ...string) bool {
	normalised := normaliseHeading(text)
	if normalised == "" {
		return false
	}
	padded := " " + normalised + " "
	for _, keyword := range keywords {
		needle := normaliseHeading(keyword)
		if needle == "" {
			continue
		}
		if strings.Contains(padded, " "+needle+" ") {
			return true
		}
	}
	return false
}

func normaliseHeading(text string) string {
	var builder strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastSpace = false
		} else if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func (p Paragraph) HasProse() bool {
	return p.Words >= 5 && containsLetter(p.Text)
}

func containsLetter(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func (d Document) FirstIntroParagraph() (Paragraph, bool) {
	for _, paragraph := range d.Paragraphs {
		if paragraph.Intro && paragraph.Text != "" {
			return paragraph, true
		}
	}
	return Paragraph{}, false
}

func (i Image) IsBadge() bool {
	lower := strings.ToLower(i.Destination)
	if strings.Contains(lower, "badge") || strings.Contains(lower, "shields.io") ||
		strings.Contains(lower, "badgen.net") || strings.Contains(lower, "goreportcard.com") ||
		strings.Contains(lower, "codecov.io") || strings.Contains(lower, "coveralls.io") ||
		strings.Contains(lower, "travis-ci") || strings.Contains(lower, "circleci.com") ||
		strings.Contains(lower, "appveyor.com") || strings.Contains(lower, "badge.fury.io") {
		return true
	}
	return strings.Contains(strings.ToLower(i.Alt), "badge")
}

func (i Image) IsLikelyVisual() bool {
	if i.Destination == "" || i.IsBadge() {
		return false
	}
	lowerDest := strings.ToLower(i.Destination)
	lowerAlt := strings.ToLower(i.Alt)
	for _, marker := range []string{"screenshot", "screen shot", "demo", "preview", "example", "usage", "showcase", "terminal", "screencast", "tui"} {
		if strings.Contains(lowerAlt, marker) || strings.Contains(lowerDest, marker) {
			return true
		}
	}
	if strings.Contains(lowerDest, ".gif") || strings.Contains(lowerDest, "img.youtube.com") ||
		strings.Contains(lowerDest, "youtu.be") {
		return true
	}
	for _, marker := range []string{"logo", "icon", "badge", "shield", "avatar"} {
		if strings.Contains(lowerAlt, marker) || strings.Contains(lowerDest, marker) {
			return false
		}
	}
	extension := strings.ToLower(path.Ext(strings.Split(lowerDest, "?")[0]))
	switch extension {
	case ".png", ".jpg", ".jpeg", ".webp", ".avif":
		return true
	default:
		return false
	}
}

func (d Document) HasVisual() bool {
	if d.HasMermaid {
		return true
	}
	for _, image := range d.Images {
		if image.IsLikelyVisual() {
			return true
		}
	}
	return false
}

func (d Document) HasCodeExample() bool {
	for _, code := range d.CodeBlocks {
		if code.Fenced && code.Language != "" && !strings.EqualFold(code.Language, "mermaid") && strings.TrimSpace(code.Content) != "" {
			return true
		}
	}
	return false
}

func (d Document) HeadingCountAtLeast(levels ...int) int {
	wanted := make(map[int]bool, len(levels))
	for _, level := range levels {
		wanted[level] = true
	}
	count := 0
	for _, heading := range d.Headings {
		if wanted[heading.Level] {
			count++
		}
	}
	return count
}

func (d Document) HasTOC() bool {
	for _, heading := range d.Headings {
		if HeadingMatches(heading.Text, "table of contents", "contents", "inhalt", "inhaltsverzeichnis") {
			return true
		}
	}
	anchorLinks := 0
	for _, link := range d.Links {
		if strings.HasPrefix(strings.TrimSpace(link.Destination), "#") {
			anchorLinks++
		}
	}
	return anchorLinks >= 3
}

func (d Document) CIWorkflowReferences() []string {
	seen := make(map[string]bool)
	var references []string
	collect := func(raw string) {
		for _, reference := range githubWorkflowBadgePattern.FindAllString(raw, -1) {
			reference = strings.TrimSuffix(reference, "/badge.svg")
			reference = strings.TrimPrefix(reference, "https://github.com/")
			reference = strings.TrimPrefix(reference, "http://github.com/")
			if !seen[reference] {
				seen[reference] = true
				references = append(references, reference)
			}
		}
	}
	for _, image := range d.Images {
		collect(image.Destination)
	}
	for _, link := range d.Links {
		collect(link.Destination)
	}
	return references
}

var githubWorkflowBadgePattern = regexp.MustCompile(`(?i)(?:https?://)?github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/(?:actions/workflows/[^\s")]+/badge\.svg|workflows/[^\s")]+/badge\.svg)`)

func (d Document) ExternalLinks() []Link {
	seen := make(map[string]bool)
	result := make([]Link, 0)
	for _, link := range d.Links {
		dest := strings.TrimSpace(link.Destination)
		if dest == "" || strings.HasPrefix(dest, "#") {
			continue
		}
		parsed, err := url.Parse(dest)
		if err != nil {
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		key := parsed.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, link)
	}
	return result
}

func (d Document) RelativeLinks() []Link {
	seen := make(map[string]bool)
	result := make([]Link, 0)
	for _, link := range d.Links {
		raw := strings.TrimSpace(link.Destination)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" {
			continue
		}
		pathValue := strings.TrimSpace(parsed.Path)
		if pathValue == "" {
			continue
		}
		if seen[pathValue] {
			continue
		}
		seen[pathValue] = true
		result = append(result, link)
	}
	return result
}

func (d Document) EmptySections() []string {
	var result []string
	for _, heading := range d.Headings {
		if heading.Empty {
			result = append(result, heading.Text)
		}
	}
	return result
}

func (d Document) String() string {
	return fmt.Sprintf("%s (%d headings, %d words)", d.Title, len(d.Headings), d.Words)
}
