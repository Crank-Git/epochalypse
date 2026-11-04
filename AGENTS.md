# epochalypse

## Project state

epochalypse provides DNSSEC window experiments, time capsules, dead man’s switches, and DNS countdowns.
The owner approved the spec on 2026-11-04. The foundation and baseline research are implemented. The tracker records feature work still in progress.
Read docs/specs/spec.md before work. Read the relevant docs/specs/features/ file before a feature change.
The spec is public. Keep credentials, real secrets, and private infrastructure values outside tracked files.

## Stack and layout

Use Go with the toolchain from the spec. Use standard HTTP, templates, JSON, and CLI flags where practical.
Product source belongs in cmd/epochalypse, internal/, and web/.
Tests belong beside their Go packages or in tests/ for cross-service checks.
Deployment files belong in deploy/. Automation belongs in scripts/.
Verify dependencies before implementation uses them.
Keep the generated spec site and its vendored assets tracked.

## Commands

The foundation provides the commands below. Read docs/checks.md for the current required suites.
Standalone resolver feature checks remain gated until their implementation exists; the isolated compatibility research has its own runner.

- Build: `make build`.
- Run the main service: `make run`.
- Start the local lab: `make lab`.
- Create synthetic records: `make demo`.
- Run unit tests: `make test-unit`.
- Run integration tests: `make test-integration`.
- Run browser and CLI tests: `make test-e2e`.
- Run resolver tests: `make test-resolvers`.
- Run race checks: `make test-race`.
- Check coverage: `make coverage`.
- Run bounded fuzz tests: `make fuzz-smoke`.
- Run source checks: `make lint`.
- Run every required check: `make check`.
- Render the current spec: `python3 docs/specs/render-spec.py`.

## Required rules

Read docs/rules/ste.md before prose, code comments, docstrings, test names, issues, or pull-request text.
Read docs/rules/quality.md before source or test changes and their review.
Read docs/rules/external-apis.md before code that uses an external library, protocol adapter, service, or CLI.
Use the controlled vocabulary in the spec's Terms table.
Use idiomatic Go identifiers and explicit errors with context. Keep secret values outside errors.
Use short imperative commit subjects. Describe the behavior and verification in each pull request.

## Release rules

Use dev for integration. Promote dev to main only through a separate owner-approved step.
Each epic produces one integration pull request with required CI.
Use test-first behavior changes and the exact quality gates from the spec.
Every required test passes. First-party Go statement coverage reaches at least 95.00 percent.
Keep switch production use disabled until the protocol review and maintainer decision pass.
Approval of the project plan does not approve an unreviewed switch protocol.

## Issue filing gate

A finding earns a tracker issue in these five cases:

- Behavior.
- A user-visible output.
- A guard that guards nothing.
- A blocked epic.
- A question the maintainer must rule.

A spec that describes the wrong product earns a spec-update issue.
Repair every other finding in the change set that finds it. File no issue for that repair.
Examples include stale counts, moved citations, missing terms, and prose drift.
If a repair changes behavior, stop the repair and file a behavior issue.

Route each repair as follows:

- During issue work, use that issue's pull request and its already touched files.
- During batch review, use a documentation commit on the batch integration branch before the CI-trigger commit.
- With no open work, use one documentation maintenance branch with a pull request into dev.

A changelog entry records a decision at a batch gate.
Do not require a changelog row, spec edit, or documentation accuracy pass for each change set or batch member.
A batch that ships as planned needs no changelog entry.
