# readmecheck — build plan

A local, deterministic GitHub README auditor. No LLM or other AI service is used.

## Milestones

1. **Project skeleton and toolchain**
   - Go module, templ, templUI CLI workflow (`templui init`, `templui add`), Tailwind
     standalone CLI, htmx, Makefile.
   - Server with a templ + templUI hello page on `localhost:8080`.
   - Baseline `go build`, `go vet`, and `go test` are green.

2. **GitHub URL parsing**
   - Accept `owner/repo`, `github.com/owner/repo`, HTTPS URLs, trailing slashes, `.git`,
     `/tree/<branch>/...`, query strings, and fragments.
   - Reject non-GitHub hosts, incomplete paths, reserved owner names, and unsupported
     repository subpages. Table-driven tests.

3. **GitHub provider**
   - REST client behind a provider interface, optional `GITHUB_TOKEN`, per-repo in-memory
     TTL cache, friendly typed errors (404, missing README, private, rate limit reset).
   - Metadata, raw README, root listing, workflow list, latest release, and optional
     recursive tree for link validation.
   - `httptest.Server` tests; no production endpoint in tests.

4. **Markdown document model**
   - goldmark + GFM parsing into headings, sections, paragraphs, code blocks, links,
     images, HTML snippets, and Mermaid markers.
   - English/German heading helpers.

5. **Rules, scoring, and link checking**
   - At least 20 documented rules in seven categories, with weight, severity, status,
     rationale, fix advice, and evidence.
   - Weighted 0–100 score, A–F grade, category scores; skipped rules excluded.
   - Concurrent, bounded external-link checker with timeout and fixture-based tests.
   - `docs/criteria.md` with sources for every rule.

6. **Web UI**
   - templUI input, switch, button, card, badge, progress, alert, accordion, separator,
     and icon components.
   - htmx URL submission, loading state, inline validation, dark mode.
   - Repo header, overall and category scores, failed-first checklist, why/fix panels.

7. **History and export**
   - SQLite via `modernc.org/sqlite`; last checked repos with score/timestamp; re-run links.
   - Copy report as Markdown and `.md` download.

8. **Hardening and handoff**
   - Route and handler tests, full test suite, vet, screenshots/manual smoke test.
   - Verify one real public repository against the GitHub API.
   - Project README, NOTES.md, final polish and commits.

## Constraints

- Standard library `net/http` and Go 1.22+ routing patterns only.
- htmx is loaded from a local vendored file; the app has no CDN dependency.
- Tests never call the real network. The only manual network smoke test happens at the end.
- Small conventional commits after each green milestone.

## Non-goals

- GitLab or local-folder providers (the provider interface is the extension point).
- AI-generated suggestions or any remote analysis service.
- User accounts, background workers, or report persistence beyond history metadata.
