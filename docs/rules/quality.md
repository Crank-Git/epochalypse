# Source quality rules

## Scope

Apply these rules to first-party Go, web source, scripts, and tests.
The approved spec defines required behavior. The rules below define pass or fail for a change.

## Behavior and tests

- A behavior change includes a test that fails against the previous behavior.
- The pull request records the failing command and the passing command.
- The initial failure demonstrates the intended behavior, rather than a syntax error.
- Every required test passes before merge.
- A skipped required test fails the check.
- An unexplained flaky failure blocks merge.
- First-party Go statement coverage reaches 95.00 percent before rounding.
- The coverage profile includes CLI and HTTP adapters.
- The coverage check rejects an absent, empty, malformed, or insufficient profile.
- Coverage excludes only dependencies and generated third-party code.
- Every changed authorization or deadline boundary has an explicit assertion.
- Tests use fake clocks for deterministic boundaries.
- Agent durability tests exercise real restart behavior.
- Tests cover the failure branches of each changed public contract.
- The race check reports no race.

## Maintainability

- The format check reports no unformatted Go file.
- The vet check reports no finding.
- The configured linter reports no unused first-party code.
- Cyclomatic complexity remains at or below 15 for each first-party function.
- Control-flow nesting remains at or below four levels.
- A dependency has a documented purpose and a pinned version.
- Code preserves an error's cause with useful context.
- Logs and errors exclude secrets, tokens, key shares, and plaintext content.
- An external call has an explicit timeout and a bounded response.
- An upload has a byte limit before an allocation from untrusted length data.
- A request validates identifiers and state before a mutation.

## State and security

- An agent persists its accepted state before it signs a receipt.
- A direct check-in verifies the owner signature and increasing sequence.
- A protocol change preserves versioned fixtures or supplies a reviewed migration.
- A state rollback cannot silently restore a live agent's old deadline.
- Switch production use remains disabled until the protocol review passes.
- A public download verifies authenticated content before it exposes plaintext.
- A changed secret-handling path has a negative test for accidental disclosure.
- A production deployment verifies its commit and required recovery endpoints.

## Check scope

Run the checks required by the changed contracts.
Run the full required suite on the epic integration pull request.
Once the required checks pass, repeat them only after a relevant change or new failure evidence.
The foundation epic implements the commands before product behavior depends on them.
