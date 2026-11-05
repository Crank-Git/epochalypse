# Local DNSSEC patch

This directory copies the complete Go module `codeberg.org/miekg/dns v0.6.128`.
The upstream BSD 3-Clause license remains in `LICENSE`.
The module keeps its upstream module path. The root module selects version `v0.6.128` through a version-specific `replace`.
First-party coverage and lint checks treat it as a dependency.

Source: https://codeberg.org/miekg/dns/src/tag/v0.6.128

Go module checksum: `h1:Zz0HdjD0+1qcwaxri0+NlRP5wf7MyE/OQCl/Im8ynZc=`.
Go module metadata checksum: `h1:CRdTEkDK9YPcVEH7sq/bwnnSz7syhHqbxfvY3IO0xlo=`.

The only upstream source change removes `rr.KeyTag == 0 ||` from the initial guard in `RRSIG.Sign`.
RFC 4034 appendix B permits the computed 16-bit key tag zero.
The upstream guard rejects that valid value before signature creation.
The patch preserves the algorithm, signer name, key policy, signature format, and cryptographic implementation.
`RRSIG.Verify` already compares the signature tag against the computed key tag without this rejection.

On 2026-11-05, release `v0.6.128` remains the latest release.
Upstream main commit `0de345a5202af2f44d35008d86712e3c1dac9e80` retains the same guard.
ADR 0011 records the decision and removal condition.
The first-party zero-tag tests and the real resolver matrix verify this patch.
