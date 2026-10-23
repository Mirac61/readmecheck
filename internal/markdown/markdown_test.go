package markdown

import (
	"strings"
	"testing"
)

const goodFixture = "# Demo\n\n" +
	"Demo is a small command-line tool that turns text into colorful diagrams.\n\n" +
	"[![CI](https://github.com/owner/demo/actions/workflows/ci.yml/badge.svg)](https://github.com/owner/demo/actions)\n\n" +
	"## Installation\n\n" +
	"Requirements: Go 1.24 or newer.\n\n" +
	"~~~bash\ngo install example.com/demo@latest\n~~~\n\n" +
	"## Usage\n\n" +
	"![Screenshot of the demo output](docs/screenshot.png)\n\n" +
	"~~~go\npackage main\n\nfunc main() {}\n~~~\n\n" +
	"See [the guide](docs/guide.md) and [GitHub](https://github.com/owner/demo).\n\n" +
	"## Contributing\n\n" +
	"Pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).\n"

func TestParseGoodDocument(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(goodFixture))
	if doc.Title != "Demo" {
		t.Fatalf("Title = %q", doc.Title)
	}
	if len(doc.Headings) != 4 {
		t.Fatalf("Headings = %+v", doc.Headings)
	}
	if !doc.HasHeading("installation", "setup") {
		t.Fatal("installation heading not found")
	}
	if doc.HasHeading("Verwendung") {
		t.Fatal("unexpected German heading")
	}
	if !doc.HasCodeExample() {
		t.Fatal("fenced code example not found")
	}
	if !doc.HasVisual() {
		t.Fatal("screenshot not found")
	}
	if len(doc.Images) != 2 {
		t.Fatalf("Images = %+v", doc.Images)
	}
	if doc.Images[0].Alt != "CI" || !doc.Images[0].IsBadge() {
		t.Fatalf("badge image = %+v", doc.Images[0])
	}
	if doc.Images[1].Alt == "" || !doc.Images[1].IsLikelyVisual() {
		t.Fatalf("visual image = %+v", doc.Images[1])
	}
	if got := doc.RelativeLinks(); len(got) != 2 {
		t.Fatalf("RelativeLinks = %+v", got)
	}
	if got := doc.ExternalLinks(); len(got) != 2 {
		t.Fatalf("ExternalLinks = %+v", got)
	}
	intro, ok := doc.FirstIntroParagraph()
	if !ok || !strings.Contains(intro.Text, "command-line tool") || !intro.HasProse() {
		t.Fatalf("intro = %+v, ok=%v", intro, ok)
	}
	if got := doc.EmptySections(); len(got) != 0 {
		t.Fatalf("EmptySections = %v", got)
	}
	if refs := doc.CIWorkflowReferences(); len(refs) != 1 || refs[0] != "owner/demo/.github/workflows/ci.yml" {
		t.Fatalf("CIWorkflowReferences = %#v", refs)
	}
	if doc.HasTOC() {
		t.Fatal("unexpected table of contents")
	}
}

func TestParseGermanHeadings(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`# Werkzeug

Ein kleines Werkzeug für Texte.

## Einrichtung

Installiere es.

## Verwendung

~~~bash
werkzeug --help
~~~

## Voraussetzungen

Go 1.24.

## Lizenz

MIT
`))
	for _, keyword := range []string{"Einrichtung", "Verwendung", "Voraussetzungen", "Lizenz"} {
		if !doc.HasHeading(keyword) {
			t.Errorf("heading %q not matched", keyword)
		}
	}
	if doc.HasHeading("Installation") {
		t.Error("unexpected English heading match")
	}
}

func TestParseEmptySections(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`# Project

## Installation

## Usage

~~~bash
demo
~~~

### Details

## Contributing

Hello.
`))
	empty := doc.EmptySections()
	// A sub-heading does not make its parent empty, and code blocks count as
	// content. Installation is empty because Usage follows at the same level,
	// Details because Contributing follows at a higher level.
	want := map[string]bool{"Installation": true, "Details": true}
	if len(empty) != len(want) {
		t.Fatalf("EmptySections = %v, want %v", empty, want)
	}
	for _, heading := range empty {
		if !want[heading] {
			t.Errorf("unexpected empty heading %q; all: %v", heading, empty)
		}
	}
}

func TestParseHeadingWithOnlySubsectionsIsNotEmpty(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`# Project

## Usage

### Install

Run the installer.

### Configure

Edit the config.
`))
	if empty := doc.EmptySections(); len(empty) != 0 {
		t.Fatalf("EmptySections = %v, want none", empty)
	}
}

func TestParseIgnoresPlaceholdersInCode(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte("# Project\n\nRun the sample:\n\n```go\n// TODO: replace this example\n```\n\nInline `TODO` is also code.\n"))
	if strings.Contains(doc.Prose, "TODO") {
		t.Fatalf("Prose contains code placeholder: %q", doc.Prose)
	}
}

func TestParseHTMLIntroParagraph(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`<p align="center">
  <img src="logo.png" alt="logo">
</p>
<p align="center">Turns YAML into JSON, fast.</p>

## Usage

Run it.
`))
	if doc.Title != "" {
		t.Fatalf("unexpected title %q", doc.Title)
	}
	intro, ok := doc.FirstIntroParagraph()
	if !ok {
		t.Fatal("no HTML intro paragraph found")
	}
	if intro.Text != "Turns YAML into JSON, fast." {
		t.Fatalf("intro = %q", intro.Text)
	}
}

func TestParseEmptyReadme(t *testing.T) {
	t.Parallel()
	doc := Parse(nil)
	if doc.Title != "" || doc.Words != 0 || doc.Lines != 0 {
		t.Fatalf("empty doc = %+v", doc)
	}
	if _, ok := doc.FirstIntroParagraph(); ok {
		t.Fatal("empty document has intro paragraph")
	}
}

func TestParseBadgeOnlyIntro(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`# Project

[![Go Reference](https://pkg.go.dev/badge/example.com/project.svg)](https://pkg.go.dev/example.com/project)
[![Build](https://github.com/owner/project/workflows/ci.yml/badge.svg)](https://github.com/owner/project/actions)

Project turns YAML into JSON from the command line.

## Usage

~~~bash
project input.yaml
~~~
`))
	paragraphs := doc.Paragraphs
	if len(paragraphs) != 2 {
		t.Fatalf("paragraphs = %+v", paragraphs)
	}
	if paragraphs[0].Text != "" {
		t.Fatalf("badge paragraph has visible text: %q", paragraphs[0].Text)
	}
	intro, ok := doc.FirstIntroParagraph()
	if !ok || !strings.HasPrefix(intro.Text, "Project turns") {
		t.Fatalf("intro = %+v, ok=%v", intro, ok)
	}
}

func TestParseHTMLContent(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte(`<h1 align="center">HTML Project</h1>

<p align="center">
  <img src="docs/demo.gif" alt="Animated demo" width="600">
  <img src="docs/logo.png">
</p>

<p>Read the <a href="docs/guide.md">guide</a> or visit <a href="https://example.com">the website</a>.</p>
`))
	if doc.Title != "HTML Project" {
		t.Fatalf("Title = %q", doc.Title)
	}
	if len(doc.Images) != 2 {
		t.Fatalf("Images = %+v", doc.Images)
	}
	if doc.Images[0].Alt != "Animated demo" || !doc.Images[0].IsLikelyVisual() {
		t.Errorf("first image = %+v", doc.Images[0])
	}
	if doc.Images[1].Alt != "" {
		t.Errorf("second image should have empty alt, got %q", doc.Images[1].Alt)
	}
	if len(doc.RelativeLinks()) != 1 || len(doc.ExternalLinks()) != 1 {
		t.Fatalf("links relative=%+v external=%+v", doc.RelativeLinks(), doc.ExternalLinks())
	}
}

func TestParseMermaid(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte("# Diagram\n\n~~~mermaid\nflowchart LR\n  A --> B\n~~~\n\n<div class=\"mermaid\">graph TD; A-->B;</div>\n"))
	if !doc.HasMermaid || !doc.HasVisual() {
		t.Fatalf("Mermaid not detected: %+v", doc)
	}
	if doc.HasCodeExample() {
		t.Fatal("Mermaid block must not count as a usage code example")
	}
}

func TestHeadingMatchesIsWordAware(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		key  string
		want bool
	}{
		{"Installation", "install", false},
		{"Installation", "installation", true},
		{"Getting Started", "getting started", true},
		{"How to Use", "usage", false},
		{"Usage", "usage", true},
		{"Benutzung", "benutzung", true},
		{"Table of Contents", "contents", true},
		{"User Guide", "use", false},
		{"Install & Configure", "install", true},
	}
	for _, tt := range cases {
		if got := HeadingMatches(tt.text, tt.key); got != tt.want {
			t.Errorf("HeadingMatches(%q, %q) = %v, want %v", tt.text, tt.key, got, tt.want)
		}
	}
}

func TestParseAutolink(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte("# Links\n\nVisit https://example.com for details.\n"))
	if got := doc.ExternalLinks(); len(got) != 1 || got[0].Destination != "https://example.com" {
		t.Fatalf("ExternalLinks = %+v", got)
	}
}

func TestParseFencedLanguage(t *testing.T) {
	t.Parallel()
	doc := Parse([]byte("# Code\n\n~~~go title=main.go\nfmt.Println(\"hi\")\n~~~\n\n    indented\n"))
	if len(doc.CodeBlocks) != 2 {
		t.Fatalf("code blocks = %+v", doc.CodeBlocks)
	}
	if doc.CodeBlocks[0].Language != "go" || !doc.CodeBlocks[0].Fenced {
		t.Errorf("fenced block = %+v", doc.CodeBlocks[0])
	}
	if doc.CodeBlocks[1].Fenced || doc.CodeBlocks[1].Language != "" {
		t.Errorf("indented block = %+v", doc.CodeBlocks[1])
	}
}
