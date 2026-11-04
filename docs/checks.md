# Local checks

Use Go 1.27.3, golangci-lint 2.15.0, Python 3, and GNU Make.
The owner store uses pinned bbolt and Argon2id dependencies. See `go.mod` and the owner storage decision.
Browser tests require Docker. The pinned Linux image supplies Chromium and Node.js.
The linter checks complexity, nesting, unused code, and security.
Each function permits at most 15 complexity points and four control-flow levels.

Run `make check` from the repository root.
The command requires every currently applicable check to pass:

- Go unit tests and check-script tests.
- Live HTTP and DNS listener tests with three agent instances.
- CLI E2E tests through an instrumented executable.
- Browser dashboard states, keyboard access, scoped summary access, sign-in, sign-out, CSRF rejection, token administration, and cooldown tests through Chromium.
- Go race checks.
- Aggregate coverage across every first-party Go package.
- A 30-second command-parser fuzz test.
- Go format, vet, and configured lint checks.
- Linux amd64 and arm64 builds.

The Go test runner limits each command to 120 seconds.
If a command exceeds this limit, the runner fails with the command to inspect.
Malformed JSON events fail with the event line number and output requirements.
Empty suites, skipped tests, and nonzero command results fail the check.

Coverage combines unit tests, CLI executions, and browser executions.
Instrumentation uses the explicit first-party module path to exclude dependencies.
The CLI profile includes the executable entry point.
The gate compares integer statement totals before it formats the percentage.
An absent, empty, malformed, or insufficient profile fails the gate.
Use `python3 scripts/coverage_gate.py <profile> [<profile> ...]` to validate profiles directly.

`make test-integration` starts five local listeners and verifies HTTP health.
The DNS listener refuses queries over UDP and TCP.
The CLI E2E suite verifies process startup, configuration errors, occupied ports, and signal shutdown.
`make test-resolvers` fails with an explicit unavailable message until the DNS feature supplies its suite.
The DNS feature supplies tests against the three pinned resolver configurations.
`scripts/test_browser.py` runs the required browser flow. A missing Docker daemon or a failed browser fails the check.
The browser test uses synthetic owner state and a temporary profile.
The dashboard checks verify the empty feature lists, the switch production block, and unavailable agent observations.
The browser checks the section links with real Tab and Enter key events.
The container has no external network access. The server listens on loopback inside the container.
The test mounts the repository read-only and its own temporary directory with write access.
The container removes itself after the test. The test stops its server and browser.
The image runs trusted test code as root, as the official Playwright test guidance permits.
Native macOS Chrome exits with SIGABRT in the current restricted environment.
The container test provides the required browser gate on macOS and Linux.

Make exports repository-local cache paths under `var/`.
These paths keep local checks inside the workspace.
Set `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, or `GOLANGCI_LINT_CACHE` to override a path.
Make exports `BUILDX_CONFIG` under `var/cache/buildx` for Docker build state.
Set `BUILDX_CONFIG` to override that path.
The repository ignores these caches.
Build output uses `bin/`.
Test artifacts use `.test-artifacts/`.

Verified interfaces, retrieved 2026-11-04:

- [Go 1.27.3 release history](https://go.dev/doc/devel/release).
- [Go module configuration](https://go.dev/ref/mod#go-mod-file).
- [Go executable coverage](https://go.dev/doc/build-cover).
- [Playwright Docker image and trusted test guidance](https://playwright.dev/docs/docker), v1.64.1-noble.
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/), Page and Runtime methods.
- [Node.js WebSocket](https://nodejs.org/api/globals.html#class-websocket), built-in API.
- [golangci-lint 2.15.0 release](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0).
- [golangci-lint 2.15.0 configuration](https://github.com/golangci/golangci-lint/blob/v2.15.0/.golangci.reference.yml).

The installed Go help documents the test, fuzz, race, and build flags.
The installed linter help documents `golangci-lint run`.

See [the local lab](lab.md) for container startup and smoke checks.

The browser image index is `mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f`.
The image supports Linux amd64 and arm64. The test builds the instrumented executable for the Docker server architecture.
