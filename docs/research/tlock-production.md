# Tlock adapter checkpoint

This checkpoint supplies the key adapter and durable draft producer for #20. Transport and recovery integration remain incomplete.
The production service still rejects tlock creation. This checkpoint does not complete AC-capsules-1 or AC-capsules-4.

## Dependency review

The root module pins tlock v1.2.0 with drand v2.1.8 cryptographic interfaces.
The root module selects age v1.3.2 instead of the release's age v1.1.1 dependency.
The age patch fixes [GHSA-32gq-x56h-299c](https://github.com/FiloSottile/age/security/advisories/GHSA-32gq-x56h-299c).
The advisory concerns attacker-controlled plugin names that can execute a binary.
The adapter supplies only the tlock identity. Its dependency graph excludes `filippo.io/age/plugin`.
The patched dependency provides another safeguard without a format change.

`TestPreservesLegacyAgeCompatibility` decrypts the saved age v1.1.1 fixture through the root module's age v1.3.2 dependency.
`TestWrapsFreshKeyForPinnedRound` verifies a new wrapper with the selected production dependencies.
The separate research module retains its original pins for reproduction.

The drand security policy supports the most recent minor version. The selected v2.1.8 belongs to that supported line at this review.
The age v1.3.2 release adds parser bounds and other hardening. The adapter's independent 2048-byte wrapper limit remains stricter.
The complete capsule race suite passes with both updates and their selected cryptographic dependencies.
These compatibility results support the minor updates instead of retaining unsupported drand v2.0.2 interfaces.
They do not approve a drand node deployment or guarantee future dependency support.

The [drand post-mortem](https://docs.drand.love/blog/2025/03/21/drand-v2-0-postmortem/) describes node control commands exposed through a private RPC interface.
Drand v2.1.0 separates the control and public DKG interfaces.
The adapter imports the `common` and `crypto` packages. It starts no node or RPC listener.
`go list -deps ./internal/capsule` includes no drand node-control or internal network implementation.
The module graph includes RPC dependencies through upstream common types. That fact does not expose an RPC service.
This reachability result does not approve a drand node.

The vendor advisory feeds for tlock and drand return empty arrays at this review.
The age feed lists the plugin advisory above. The selected age version includes its fix.
An empty feed does not prove the absence of undisclosed vulnerabilities.
Production integration still requires review of the final compiled service and dependency graph.

Verified against these primary interfaces on 2026-11-05 UTC:

- [tlock v1.2.0 source](https://github.com/drand/tlock/blob/v1.2.0/tlock.go): `Network`, `Strict`, `Encrypt`, `Decrypt`, and local `TimeUnlock` verification.
- [tlock v1.2.0 identity](https://github.com/drand/tlock/blob/v1.2.0/tlock_age.go): fixed-chain behavior and signature-error conversion.
- [age v1.2.1 release](https://github.com/FiloSottile/age/releases/tag/v1.2.1): plugin security fix.
- [age v1.3.2 release](https://github.com/FiloSottile/age/releases/tag/v1.3.2): parser hardening and current compatibility candidate.
- [drand v2.1.8 security policy](https://github.com/drand/drand/blob/v2.1.8/SECURITY.md): supported minor line.
- [drand v2.1.8 scheme interfaces](https://github.com/drand/drand/blob/v2.1.8/crypto/schemes.go): local beacon verification.
- [tlock advisory feed](https://api.github.com/repos/drand/tlock/security-advisories).
- [drand advisory feed](https://api.github.com/repos/drand/drand/security-advisories).
- [age advisory feed](https://api.github.com/repos/FiloSottile/age/security-advisories).
- Go 1.27.3 shipped `net/http` interfaces: `Client.Timeout`, `CheckRedirect`, and `NewRequestWithContext`.

## Producer contract

`SelectRound` accepts whole UTC seconds and returns the first quicknet round at or after that second.
It preserves exact round times. A between-round request advances to the next round.
Creation must separately reject past requests against its injected clock.
The adapter rejects invalid, subsecond, negative, and overflowing effective dates.

`WrapKey` accepts the fresh 32-byte key from `SealObject`. It sends no network request.
It returns a `Timelock` with the complete pinned quicknet identity, requested time, effective time, round, and base64url age bytes.
The constants match the full public key and chain in [the compatibility report](tlock-compatibility.md).
The binary wrapper permits at most 2048 bytes. The encoded wrapper permits at most 2731 characters.
The adapter does not encrypt content directly. The existing JWE object must continue to bind the capsule identifier and format version.

`Timelock.OpenKey` validates the identity and ceiling round before it retrieves a beacon.
It returns `ErrTooEarly` before the effective time without a request.
It verifies the exact round signature locally before it supplies that signature to strict tlock decryption.
The signature provider rejects every other round. The adapter never switches chains.
It authenticates the complete age stream into a fixed 32-byte buffer before it returns a key.
Every failure returns a zero key. Consumers must authenticate the JWE object before they expose plaintext.

`QuicknetRelay` supplies one fixed HTTPS relay at `api.drand.sh` with an explicit chain path.
It has a five-second client timeout and a five-second request context. It rejects redirects and performs no fallback.
It reads at most 8193 response bytes and rejects responses larger than 8192 bytes.
Network, read, and non-200 responses produce `ErrUnavailable`. Malformed responses produce `ErrIntegrity`.
The relay supplies untrusted bytes. Local signature verification supplies authenticity.
The open adapter classifies availability before upstream tlock can replace signature errors with `ErrTooEarly`.
Injected `BeaconSource` implementations must honor their context and bound their response.
Later HTTP handlers must retain the existing four shared capsule slots and request deadlines.

## Verification and remaining work

### Durable draft producer

`Store.Create` now accepts tlock internally. The HTTP and CLI adapters retain explicit tlock gates until the remaining interfaces are complete.
Preparation authenticates the JWE object with its fresh content key before it wraps that key.
The record retains the complete `Timelock`. Its effective time equals the selected ceiling round time.
Reads reject a missing wrapper, mismatched mode or times, an unknown version, and any changed quicknet identity.
The encoded record's existing 8192-byte metadata allowance also bounds the wrapper metadata.
The shared transaction charges every encoded record byte and the separate 32-byte preparation digest.

The existing `capsule-keys-v1` bucket stores a preparation digest for tlock records instead of a content key.
The digest covers the canonical encoded draft, including the object, wrapper, metadata, and effective time.
Publication compares this digest before it changes the publication time. Repeated publication preserves the object and wrapper.
This proof detects a changed draft. It is not a signature against an attacker who controls both database buckets.
It does not independently prove future wrapper decryption. Complete authenticated recovery must verify that relation after the beacon becomes available.
The code creates both encrypted layers from the same fresh key before it discards the key.
No import interface accepts an externally prepared record at this checkpoint.

Keeping the key until publication would defeat local tlock preparation. A digest permits the publication check without retaining that secret.
This choice uses the existing private-state allowance and preserves the fixed database layout.
The key producer returns `ErrTooEarly` before the tlock boundary and `ErrUnavailable` afterwards.
It never returns the digest as a content key. The next recovery producer must use `Timelock.OpenKey` and authenticate the complete JWE object.

`TestTimelockDraftRetainsNoContentKey` checks the retained digest and rejects its use as a content key.
`TestTimelockPublicationPreservesCeilingRoundAfterRestart` checks restart, reduced upload limits, ceiling time, repeated publication, and key boundaries.
`TestTimelockPublicationRejectsChangedDraft` checks the immutable preparation proof.
`TestTimelockWrapperConsumesSharedBudget` checks atomic quota rejection without partial state.
`TestTimelockRecordRejectsMismatchedReleaseMetadata` checks corrupt wrappers, swapped wrappers, missing private state, and absence of returned keys.
Committed test state `9143331` demonstrates four behavioral failures before this implementation.

Committed stub `9b47f6a` records three failing behavior tests, including all ten failure subtests.
The focused capsule race suite passes after implementation. The focused linter reports zero issues.
These checks do not replace the final aggregate coverage gate or full required suite.
The final focused profile measures 93.9 percent for the capsule package. It is not the project's aggregate coverage result.
The merge-base check removes both new implementation files while it retains the tests.
The tests fail with missing adapter symbols. The restore leaves a clean Git status.

The next slice must add CLI local preparation, the web disclosure and mode, typed recovery values, and authenticated content open.
It must test all three modes through the real CLI and browser interfaces.
It must preserve the legacy record fixture and classify release separately from availability.
The final verification must run the full required suite with zero required skips and at least 95 percent aggregate coverage.
