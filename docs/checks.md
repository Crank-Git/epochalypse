# Local checks

Use Go 1.27.3, golangci-lint 2.15.0, Python 3, and GNU Make.
The project uses the Go standard library only.
The linter checks complexity, nesting, unused code, and security.
Each function permits at most 15 complexity points and four control-flow levels.

Run `make check` from the repository root.
The command requires every currently applicable check to pass:

- Go unit tests and check-script tests.
- CLI E2E tests through an instrumented executable.
- Go race checks.
- Aggregate coverage across every first-party Go package.
- A 30-second command-parser fuzz test.
- Go format, vet, and configured lint checks.
- Linux amd64 and arm64 builds.

Coverage combines unit tests and CLI executions.
The CLI profile includes the executable entry point.
The gate compares integer statement totals before it formats the percentage.
An absent, empty, malformed, or insufficient profile fails the gate.
Use `python3 scripts/coverage_gate.py <profile> [<profile> ...]` to validate profiles directly.

The current scaffold has no service, browser interface, or resolver lab.
`make test-integration` and `make test-resolvers` fail with an explicit unavailable message.
These targets enter `make check` when their features supply real suites.
The service issue supplies listener tests.
The DNS feature supplies tests against the three pinned resolver configurations.
The current CLI E2E suite does not claim browser coverage.

Make exports repository-local cache paths under `var/`.
These paths keep local checks inside the workspace.
Set `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, or `GOLANGCI_LINT_CACHE` to override a path.
The repository ignores these caches.
Build output uses `bin/`.
Test artifacts use `.test-artifacts/`.

Verified interfaces, retrieved 2026-11-04:

- [Go 1.27.3 release history](https://go.dev/doc/devel/release).
- [Go module configuration](https://go.dev/ref/mod#go-mod-file).
- [Go executable coverage](https://go.dev/doc/build-cover).
- [golangci-lint 2.15.0 release](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0).
- [golangci-lint 2.15.0 configuration](https://github.com/golangci/golangci-lint/blob/v2.15.0/.golangci.reference.yml).

The installed Go help documents the test, fuzz, race, and build flags.
The installed linter help documents `golangci-lint run`.
