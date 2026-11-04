# Signed window checkpoints

Issue: #8
Date: 2026-11-04
Initial domain commit: `694581e`
Baseline: `c47240b5e43f1667643d309b204a13d3cfd115f8`

## Scope

The initial checkpoint adds the durable domain and key policy.
The next slice adds owner-protected creation and offline CLI commands.
Live DNS behavior remains incomplete.

`internal/window` supplies these contracts:

- `Input.Validate(now)` checks content, UTC boundaries, and serial-time limits.
- `LoadSigner(zone, path)` loads the configured protected key pair.
- `Open(stateDir, signer)` opens the versioned window database.
- `Store.Create(input, now)` commits one signed public TXT record.
- `Store.Get(id)` and `Store.List()` read verified public records.
- `Window.Records()` returns independent TXT, RRSIG, and DNSKEY records.
- `Store.Close()` releases the database lock.

`POST /api/v1/windows` uses the shared TLS safeguards and `RequireOwner`, which verifies the CSRF proof.
`window create` and `window inspect` use host access while the main process is stopped.
The store atomically applies positive retention limits before creation.
`docs/windows.md` records the API, CLI, and producer contracts.

ADR 0006 records the technical policy.
The version 1 fixture fixes the initial public document format.
The fixture has no private key.

## TDD evidence

Before implementation, `go test ./internal/window` fails against minimal contract stubs.
The initial validation test reports `invalid input passes validation` for invalid boundaries.
The signer test reports `signer is unavailable`.
The store tests report these assertion failures:

- `creation does not return a durable window`
- `store accepts invalid content`
- `second writer opens the locked database`

After implementation, the domain tests pass.
The tests check exact inception, expiration, content bytes, signature verification, restart, and rejected state.
They also check explicit empty TXT content against the approved upper-bound policy.

The pre-patch check keeps the new tests and removes the three new implementation files.
The baseline has no implementation paths to restore from Git.
`go test ./internal/window` fails with undefined `Window`, `Signer`, and `Input` symbols.
The check restores all implementation files from `HEAD` in the same shell call.
`git status --short` then returns no output.

## Check results

The initial domain `make check` passes on 2026-11-04.
The final instrumented Go suite reports 263 passed tests and zero skips.
The command also passes integration, CLI, Chromium, race, fuzz, lint, vet, and Linux builds.
Aggregate first-party coverage is 1257 of 1303 statements, or 96.4697 percent.
The domain coverage check reports 95.1 percent before the final empty-content policy repair.
The full aggregate check includes that repair.
A scoped race check also passes after that repair.

The required resolver suite remains gated by the existing foundation command.
This checkpoint does not implement resolver behavior.
The later Chromium flow creates a signed window after owner sign-in.
It rejects missing CSRF proof, invalid boundaries, and signed-out creation.
The executable rejects offline access while the main process owns the database lock.
After shutdown, the executable inspects the identical browser-created signature and creates another record.
The real HTTP-to-DNS flow still needs its own E2E evidence.

## Creation interface TDD evidence

Before route or command integration, two new tests fail against the existing public interfaces:

- `TestWindowCreationRequiresOwner`: `window creation status = 404, want 401`.
- `TestWindowCommandCreatesAndInspectsSignedRecords`: `window create: exit=2 error=invalid command: use epochalypse help`.

The same tests pass after integration.
Retention-limit stubs also fail the quota and positive-bound assertions before implementation.

The creation interface `make check` passes on 2026-11-04.
Its Go suites report 292 passed tests and zero skips.
Aggregate coverage is 1427 of 1473 statements, or 96.8771 percent.
The full check includes Chromium creation, executable inspection, race, fuzz, lint, vet, and both Linux builds.

## Remaining work

Co-host authoritative UDP/TCP listeners with the main HTTP process.
Serve TXT, RRSIG, DNSKEY, DS, SOA, NS, and authenticated denial responses.
Keep infrastructure signatures valid without another TXT signature.
Test UDP truncation, explicit TCP fallback, no recursion, bind failure, and teardown.
Verify authenticated HTTP creation, immediate UDP/TCP access, and restart access through real listeners.
Update isolated lab routing while the three agents retain independent processes and volumes.
Finish authority-specific operator documentation and failure-path tests.
Run the full suite and retain at least 95.00 percent aggregate coverage before sub-merge.
