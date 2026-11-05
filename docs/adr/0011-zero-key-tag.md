# ADR 0011: Accept a valid zero DNSSEC key tag

Status: Accepted by the batch PM.

## Context

RFC 4034 appendix B defines a 16-bit key tag checksum. Zero is a valid result.
The shared signer loader rejects a matching algorithm 13 key when its key tag equals zero.
The pinned DNS library also rejects zero in `RRSIG.Sign` before it creates a signature.
Its `RRSIG.Verify` implementation already accepts matching zero tags.

On 2026-11-05, `v0.6.128` remains the latest upstream release.
Upstream main commit `0de345a5202af2f44d35008d86712e3c1dac9e80` retains the same rejection.
An upgrade therefore does not correct this defect.

## Decision

Keep a complete local copy of `codeberg.org/miekg/dns v0.6.128` in `third_party/miekg-dns`.
Keep its module identity, license, source files, and tests.
Select that module with a root `replace` directive.
Remove only `rr.KeyTag == 0 ||` from the initial `RRSIG.Sign` guard.
Remove the equivalent loader rejection. Preserve all other loader checks.
Compute the key tag once before the signer accepts concurrent calls.
The library mutates its key tag cache when it computes zero.
The signer uses the retained value for each signature to avoid concurrent cache writes.
Use the existing library implementation for signatures and wire records.

The module remains a dependency outside the first-party coverage and lint scope.
`third_party/miekg-dns/EPOCHALYPSE-PATCH.md` records the source, checksums, and exact patch.
The synthetic fixture requires owner-only private access during tests.
The real resolver matrix verifies window and countdown signatures with key tag zero.

## Alternatives

A new local signing implementation would duplicate cryptographic and canonical wire behavior.
This correction requires only the removal of an invalid guard.
An external fork would require a separate source and publication policy.
The local module makes the patch visible without that dependency.

## Removal condition

Check the guard when the DNS dependency changes.
After an upstream release accepts zero tags, remove the local module and the `replace` directive.
Keep the deterministic regression fixture and tests.
Require the full quality gate and the real resolver matrix before that transition.

## Evidence

Verified against [RFC 4034 appendix B](https://www.rfc-editor.org/rfc/rfc4034.html#appendix-B), retrieved 2026-11-05.
Verified against [DNS library v0.6.128](https://codeberg.org/miekg/dns/src/tag/v0.6.128/dnssec.go), retrieved 2026-11-05.
The preserved fixture produces `algorithm=13 tag=0 Sign error=dns: bad key` with the unpatched library.
Issue #92 records the initial loader failure.
