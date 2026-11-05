# Signed window checkpoints

Issue: #8
Date: 2026-11-04
Initial domain commit: `694581e`
Baseline: `c47240b5e43f1667643d309b204a13d3cfd115f8`

## Scope

The initial checkpoint adds the durable domain and key policy.
The next slice adds owner-protected creation and offline CLI commands.
The authority slice adds live UDP/TCP service and infrastructure signatures.
The lab slice completes Compose setup and executable-process DNS restart evidence.

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

After commit `691d1a8`, a second pre-patch check restores only the public dispatchers from the baseline.
The check retains the new tests and supporting domain contracts.
The two integration assertions again fail with 404 and `invalid command`, as quoted above.
The same shell call restores both dispatchers from `HEAD`.
`git status --short` then returns no output.

## Remaining work

All assigned implementation and local verification are complete.
Issues #9 through #11 retain resolver observations, public pages, and trust diagnostics.

## Authority evidence

The initial `TestAuthorityPublishesTheStoredFutureSignature` reports an actual REFUSED response before implementation.
Infrastructure-signature stubs also fail before implementation.
`TestMainServesHTTPCreatedWindowsBeforeInceptionAndAfterRestart` initially reports `main server does not bind authoritative DNS`.
That test first creates its window through authenticated HTTP.
After integration, the same test reads identical TXT/RRSIG records through UDP and TCP before and after service restart.

`TestAuthorityQueryFallsBackToTCP` initially receives the truncated UDP response without a TCP retry.
It passes after the explicit fallback implementation.
The transport tests cover persistent TCP queries, active-client shutdown, connection limits, and malformed frames.

Review identifies a lossy wire-label case in the adapter.
Before the wire guard, one label containing the full dotted window name receives that window's TXT answer.
`TestAuthorityRefusesWireLabelsBeforeLossyDecode` reproduces this failure before the fix.
The guard now refuses literal dots, backslashes, controls, and non-ASCII label octets before decoding.
Further tests cover hidden octets behind compression pointers, pointer loops, truncated names, and name-size bounds.
Each unsupported form lacks an authoritative answer or denial proof.

The final authority `make check` passes on 2026-11-04.
It reports 337 Go tests, zero skips, and 1724 of 1781 covered statements: 96.7996 percent.
Race, Chromium, CLI, fuzz, lint, vet, and both Linux builds pass.
The final run includes the raw-wire regression fix.

After commit `b60e49a`, the authority pre-patch check restores the service dispatcher from checkpoint `94da218`.
The new tests and authority implementation remain present.
The live test creates its window through authenticated HTTP, then fails with `main server does not bind authoritative DNS`.
The same shell call restores the dispatcher from `HEAD`.
`git status --short` then returns no output.

## Lab evidence

The new Compose test initially fails against the previous lab because `/state/lab/tls.crt` does not exist.
The old lab has no signed-window key or TLS setup.
The Dockerfile repair supplies the pinned module checksums and embedded web assets.

The pinned BIND image lacks the two proposed setup executables.
The existing pinned Playwright image instead supplies verified Node.js v26.13.0 and OpenSSL 3.0.13.
The setup uses Node crypto and the shipped OpenSSL `req` options.
Key generation has no network access. The generated files have mode 0600 and UID 65534.

`test_lab_preserves_signed_windows_after_process_restart` passes through real container executables.
It verifies the copied self-signed certificate before owner sign-in and window creation.
It immediately reads the committed 512-byte TXT and exact RRSIG through UDP and TCP.
It verifies UDP truncation and the explicit TCP retry.
It restarts the main container and checks that its host PID changes.
The new process serves the identical signature.

The test verifies four independent volumes, UID 65534, disabled setup networking, and three healthy agents.
It also verifies unsafe private-key mode rejection and unchanged DNSKEY material after repeated setup.
Its cleanup removes only its unique test project and volumes.

The final post-merge `make check` passes on 2026-11-04.
Go suites report 337 passed tests and zero skips.
Aggregate coverage is 1724 of 1781 statements, or 96.7996 percent.
CLI, Chromium, Compose, race, fuzz, lint, vet, and both Linux builds pass.
The required Compose flow runs in both the E2E and aggregate coverage stages.

After commit `375d373`, the lab pre-patch check restores the baseline Compose file from `21ef129`.
The new E2E test and build repair remain present.
The old lab again fails because its main container lacks `/state/lab/tls.crt`.
The test removes its own containers and volumes.
The same shell call restores the current Compose file from `HEAD`.
`git status --short` then returns no output.
