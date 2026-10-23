# Implementation notes

This file records the decisions, dependencies, limitations, and extension points behind readmecheck. It is intentionally more technical than the README.

## Product decisions

### Local-first, deterministic analysis

The application performs the analysis on the same machine that serves the UI. There is no account system and no analytics. Every built-in check is a pure Go function over a repository snapshot, so two runs with the same inputs and the same clock produce the same report.

The one exception is the optional external-link check, which depends on the current responses of third-party servers. When that option is disabled, the rule is marked skipped rather than failed, and skipped checks are excluded from scoring.

### README model instead of HTML scraping

goldmark parses the README with GFM enabled. The document model records headings, paragraphs, fenced code blocks, Markdown links, Markdown images, raw HTML images and anchors, Mermaid blocks, word count, empty sections, and a code-free prose view. Raw HTML is inspected for `<h1>`, `<p>`, `<img>`, and `<a>` because many real READMEs mix HTML into Markdown. A centered `<p>` before the first subheading is treated as the intro paragraph.

The model is deliberately broad rather than clever. Rules look for explicit evidence such as a heading or a fenced block. This keeps the behavior explainable and makes false positives easier to fix.

### 23 rules and seven categories

The implementation has 23 rules in the categories Basics, Getting started, Visuals, Trust and maintenance, Contribution, Hygiene, and Links. Each rule has a stable ID, severity, weight, rationale, and fix text. The checklist in the UI is sorted by category, then failed, skipped, and passed, then severity and weight. Within a category, failed checks therefore appear before passing ones.

The contribution rule is informational for repositories below 1,000 stars and a warning at or above that threshold. Large projects are expected to publish contribution guidance.

### Weighted scoring

Each failed or passed rule contributes its weight to the applicable total. A passed rule earns its weight. A skipped rule contributes nothing to either side. Category scores use the same formula, and the overall score is the weighted total across categories. Grades use fixed cuts: A at 90, B at 80, C at 70, D at 60, and F below 60. A report with no applicable checks receives 0 and grade F.

### English and German headings

Installation, usage, requirements, contribution, license, and table-of-contents detection use keyword lists that include German terms such as `Installation`, `Verwendung`, `Voraussetzungen`, `Mitwirken`, `Lizenz`, and `Inhaltsverzeichnis`. The report itself is in English.

### URL parsing

The parser accepts full GitHub URLs, `github.com/owner/repo`, bare `owner/repo`, `.git` suffixes, query strings and fragments, `tree` and `blob` URLs, and `git@github.com:owner/repo.git` style clone syntax. It rejects non-GitHub hosts, credentials in URLs, ports, reserved GitHub owners, repository subpages such as Issues or Actions, and unknown extra path segments. The canonical path is always `owner/repo`.

### GitHub provider

The provider makes five independent API requests: repository metadata, raw README, GitHub Actions workflows, latest release, and a recursive git tree. They run concurrently, and a per-repository in-memory cache stores up to 256 successful snapshots for ten minutes. The root directory listing is derived from the recursive tree, saving a separate `/contents` request; if the tree is truncated or unavailable, the provider falls back to `/contents`. The cache makes a report URL followed by a history re-run fast without hiding new data for long.

Once a snapshot expires, requests are conditional: responses with an `ETag` are kept in a second bounded cache (64 entries, 1 MiB per body, 8 MiB total) and replayed when GitHub answers `304 Not Modified`. Those responses do not count against the primary rate limit, so frequent re-runs of an unchanged repository are cheap. The README filename from `Content-Disposition` is stored with the body because a 304 omits it.

A missing README is not treated as a provider failure. The snapshot records that the README was checked and is absent, and the `readme-exists` rule fails so the user still gets a report. A missing workflow list or file tree marks the related rules as skipped.

The provider reads an optional `GITHUB_TOKEN` and sets the standard bearer authorization header. The token is never logged, stored, or rendered.

### Relative and external links

Relative links and images are resolved from the directory that contains the README, matching how GitHub renders nested README files; parent segments are allowed while the result stays inside the repository. A recursive git tree supplies existence information. If the tree response is truncated or unavailable, the rule is skipped rather than reporting false failures.

External checking is disabled by default at the rule-engine level and enabled by default in the web form. The HTTP checker deduplicates URLs, checks at most 50 links, uses eight workers, tries HEAD first and retries with GET whenever the HEAD result is unsuccessful, and treats 401, 403, and 429 as reachable because those responses usually indicate access policy rather than a dead page. Network failures and 4xx/5xx responses other than those exceptions fail the rule. The default client refuses loopback, link-local, and private network addresses to avoid server-side request forgery through README links. Links that point at such addresses (`localhost`, `127.0.0.1`, `10.x`, `192.168.x`, `.local`, `.internal`) are recognized before checking and skipped, because a README line such as "open http://localhost:8080" is not a dead link; the rule detail says how many were skipped, and the checker is never asked to fetch them.

### History

The History page displays the latest run for each owner/repository pair, and reports show the score delta against the previous run. Reloading an unchanged report within five minutes is coalesced, so a browser refresh does not create a duplicate row. Re-running a history entry opens the report route, which performs a fresh analysis and records a new result once it differs. The database is SQLite through `modernc.org/sqlite`, which keeps the project fully Go and avoids a C toolchain.

### UI and htmx

The UI is server-rendered with templ and templUI components. htmx submits the form and swaps the report into `#results`; the server sets `HX-Push-Url` so the report has a shareable URL. A normal form submission still works without JavaScript and renders the full report page.

The report exports Markdown through a hidden textarea. Copying uses the local templUI copy button, extended with an optional visible label. Downloading uses a small delegated Blob handler in the layout. The report also shows a check-distribution doughnut and a category radar chart using the templUI chart component, which vendors Chart.js under `assets/js`. Dark mode is a class on the root element with a `localStorage` preference and an operating-system fallback.

The server binds to `localhost:8080` and sets basic security headers. The link checker is intended for local use; see the known issues below before exposing the app on a network.

### Missing repository metadata

Several rules need GitHub fields rather than README text: repository description, topics, license detection, workflow list, archive status, and latest release. With a GitHub provider these are populated normally. A different provider can leave the corresponding `Known` flags false, in which case the rules skip instead of guessing.

## Dependencies

| Dependency | Version | Why it is used |
| --- | --- | --- |
| Go | 1.27.3 | Standard-library HTTP server, `net/http` routing patterns, and concurrency. |
| `github.com/a-h/templ` | v0.3.1070 | Type-safe server-side HTML templates. |
| `github.com/templui/templui` | v1.13.2 | Accessible UI primitives installed into `components/` with the templUI CLI. |
| htmx | 2.0.8, vendored | Partial page updates and the form progress state. |
| Tailwind CSS standalone | v4.3.3 | Local utility CSS build without a Node toolchain. |
| `github.com/yuin/goldmark` | v1.8.6 | CommonMark parser with GFM extensions. |
| `modernc.org/sqlite` | v1.61.0 | Pure-Go SQLite driver for local history. |
| `github.com/Oudwins/tailwind-merge-go` | v0.2.3 | Class merging used by the installed templUI components. |

The Tailwind binary is placed in `bin/` and excluded from git. `make tools` downloads the pinned standalone release for the current platform. The compiled `assets/css/output.css` is committed so a checkout can run without downloading the binary. templUI scripts are vendored under `assets/js`.

The local `components/copybutton` component has one small change from the generated templUI version: an optional `Label` field. It is used for the visible "Copy report as Markdown" button. Re-running the templUI installer may overwrite that file, so keep the patch in mind when upgrading components.

`components/dialog/dialog.templ` is patched too: a darker, slightly blurred backdrop, a `focus-visible` ring on the close button so mouse users do not see a ring on open, and `tabindex="-1" autofocus` on the `<dialog>` so the first focus lands on the dialog instead of the close button. Re-apply these when upgrading the component.

`components/progress/progress.templ` is patched as well: the track uses `bg-foreground/10` with an inset `ring-foreground/15` instead of `bg-secondary`, because the stock track is almost invisible on a dark card and the end of a bar could not be seen.

`assets/js/chart.js` and the bundled `chart.min.js` set `Chart.defaults.font.size = 14` and the default text colour to the theme foreground, because the Chart.js defaults (12px, muted) were hard to read. `chart.min.js` is an esbuild bundle, so the same two lines were patched there by hand; redo both when updating the chart component.

## Known issues and limitations

- Only github.com repositories are supported. GitLab, Codeberg, and self-hosted GitHub Enterprise URLs are rejected.
- Unauthenticated GitHub requests are limited to 60 per hour. The UI explains the reset time when the limit is reached. A token is the normal remedy.
- Badge validation covers GitHub Actions workflow badges, including shields.io workflow-status badges. Other badge services are not treated as CI claims and are not verified for correctness.
- External link checking can be slow on READMEs with many links, although the 50-link cap and worker limit bound the work. Redirects are followed by the HTTP client's default policy.
- The link checker blocks private, loopback, and link-local addresses, but it does not re-validate DNS answers between lookup and connect and it follows redirects with the default policy. Harden it further before exposing the app on a shared network or the public internet.
- Relative-link checking tests path existence, not anchor fragments inside target files.
- Repository descriptions, topics, and license names are taken from the GitHub API at check time. A stale token or an API outage can change which metadata rules are available.
- The in-memory provider cache is lost on restart. It is a performance aid, not a durable mirror.
- The history database stores scores and repository summaries, not full report JSON. Re-running a history entry performs a new analysis, so a later re-run may differ if the repository changed.
- SQLite uses a single connection to avoid lock contention. This is deliberate for a local single-user app; it is not designed for horizontal scaling.
- The application has no authentication or CSRF token. It is a local tool, not a multi-user service.
- The build expects Go 1.27.3 because the module declares that version. The generated templ files and compiled CSS are checked in to keep the initial build simple.

## AI extension point

`internal/rules/types.go` defines an `Analyzer` interface:

```go
type Analyzer interface {
    Name() string
    Analyze(ctx context.Context, input Input) ([]Result, error)
}
```

The deterministic rule engine never invokes an analyzer. A future optional feature could run one alongside `rules.Analyze` and merge its findings as a separate, clearly labeled section. Such an integration should:

- Be off by default and require an explicit opt-in.
- Never change the deterministic score or grade; LLM findings would be advisory.
- Receive only the README and repository metadata the user agreed to send.
- Return the same `Result` shape with an identifier outside the built-in rule namespace.
- Preserve reproducibility by storing the model name and a hash of its input with the finding.

No AI client, prompt, or implementation exists in this repository. The interface exists only to keep the boundary explicit.

## Verification summary

- Unit and handler tests use fixtures and injected fakes; the normal `go test ./...` run does not contact GitHub or any external link.
- `go build ./...`, `go vet ./...`, and `go test ./...` are required before each commit.
- The final manual smoke test was performed on 2026-10-22 with `make run` on `http://localhost:8080`. The target was `https://github.com/charmbracelet/bubbletea` with external link checking enabled.
- The smoke run returned HTTP 200, redirected the htmx push URL to `/report/charmbracelet/bubbletea?external=1`, and produced a score of 84 (grade B): 19 passed, 4 failed, and 0 skipped. The four failures were `installation-section`, `image-alt`, `no-empty-sections`, and `table-of-contents`, which is plausible for a large real-world README.
- The direct report route, Markdown export filename, SQLite history page, and re-run link all rendered correctly after the live run. The test database was a local `readmecheck.db` created at runtime and is ignored by git.
