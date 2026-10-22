# Contributing to readmecheck

Thanks for taking the time to improve readmecheck. Small, focused changes are easiest to review.

## Development setup

You need Go 1.27.3 or newer, GNU Make, and a checkout of this repository.

```bash
make build
make test
```

`make build` regenerates templ code, compiles the Tailwind stylesheet, and builds every package. Generated `*_templ.go` files and `assets/css/output.css` are committed so the project can build without the optional tooling.

If you change a `.templ` file, regenerate before committing:

```bash
make generate
make css
```

The templ CLI and the Tailwind standalone binary are not required to use a prebuilt checkout. The Makefile falls back to the templ binary in `GOPATH/bin` and expects Tailwind at `bin/tailwindcss` when regeneration is requested.

## Tests

Run the complete verification suite before opening a pull request:

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Tests must not contact the real network. Use fixture files in `testdata/` and injected HTTP clients or `httptest` servers. The GitHub provider and the HTTP link checker both accept custom clients for this reason, and handler tests use fake providers, stores, and link checkers.

When adding a rule:

1. Add the implementation to the appropriate rule file.
2. Give it a stable, lowercase ID and place it in the most specific category.
3. Provide a rationale, a concrete fix, a severity, and a weight.
4. Add a focused unit test with positive, negative, and skipped cases where applicable.
5. Update [docs/criteria.md](docs/criteria.md) with the rule summary and sources.

When changing the UI:

1. Update the templ source, then run `make generate`.
2. Rebuild the stylesheet with `make css`.
3. Check both light and dark mode.
4. Confirm the htmx flow, copy button, and Markdown download still work.

## Code style

- Keep HTTP routing on `net/http` patterns; do not add a router dependency.
- Return typed errors from the GitHub provider so handlers can present useful messages.
- Never log the GitHub token.
- Keep vote-changing behavior deterministic. A rule must not require an LLM or a random source.
- Prefer table-driven tests with explicit expected values.
- Keep user-facing explanations short and actionable.

## Pull request checklist

- The change has a clear user-facing purpose.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass.
- New behavior has tests and, for rules, documentation.
- Generated files and compiled CSS are up to date.
- The change does not add network access to the normal test suite.
- The report markup remains usable with a keyboard and a screen reader.

## Reporting bugs

Include the repository URL or a minimal fixture, the rule ID, the observed result, and the expected result. For failures in fetching repository data, include the HTTP status and whether a `GITHUB_TOKEN` was configured. Do not include the token itself.
