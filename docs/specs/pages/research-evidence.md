---
id: research-evidence
title: Research evidence
---

# Research evidence

## DNSSEC behavior

Sources retrieved on 2026-11-04:

| Source | Version | Verified claim |
|---|---|---|
| [DNSSEC introduction](https://www.rfc-editor.org/rfc/rfc4033.html) | RFC 4033 | DNSSEC does not provide confidentiality. |
| [Record format](https://www.rfc-editor.org/rfc/rfc4034.html) | RFC 4034 | RRSIG carries 32-bit inception and expiration values. |
| [Validation](https://www.rfc-editor.org/rfc/rfc4035.html) | RFC 4035 | Validation checks signature time and the trust chain. |
| [Extended errors](https://www.rfc-editor.org/rfc/rfc8914.html) | RFC 8914 | Code 8 describes a signature not yet valid. |
| [Stale data](https://www.rfc-editor.org/rfc/rfc8767/) | RFC 8767 | Recursive TTL is not an exact countdown guarantee. |
| [Secondary zones](https://datatracker.ietf.org/doc/html/rfc1034) | RFC 1034 | A secondary zone has an expiry boundary during primary loss. |
| [Unbound manual](https://unbound.docs.nlnetlabs.nl/en/latest/manpages/unbound.conf.html) | 1.26.2 documentation | Clock allowances and bogus-cache configuration affect observations. |

Unbound documents a minimum skew setting of 3600 seconds and a maximum of 86400 seconds.
The configured values and signature lifetime affect the actual allowance.
The lab must measure acceptance rather than translate those defaults into an exact early-release interval.

A direct authoritative query can retrieve public data before inception.
Encryption does not help if the authority also publishes the content key early.
Inference: withhold the key or depend on a separate cryptographic release mechanism for confidentiality.
A failed decoy signature does not itself publish a secret. A surviving service must provide recovery material.

## Timelock encryption

[tlock v1.2.0](https://github.com/drand/tlock/blob/v1.2.0/README.md) supplies a Go library and CLI.
The pinned README contains both legacy fastnet and quicknet references.
The current [drand documentation](https://docs.drand.love/docs/timelock-encryption/) describes beacon-based release and its trust assumptions.
The [security assessment notice](https://docs.drand.love/blog/2023/05/26/tlock-security-assessment/) reports an external assessment.
An assessment does not establish the safety of this project's integration.

Inference: a retained fixed-time ciphertext remains decryptable at its original round despite a later owner check-in.
Use tlock for fixed capsules. A renewable switch needs a different protocol.
Do not assume a beacon offers cancellation, private recipients, or indefinite availability.

## Dependency review

[miekg/dns v1.1.73](https://github.com/miekg/dns/tree/v1.1.73) supports DNSSEC primitives.
Its README points to a successor at Codeberg and describes limited future maintenance.
The foundation compatibility work must compare the adapters before it chooses one.
[Go 1.27.3](https://go.dev/doc/devel/release) is the initial toolchain target.

Mermaid 11.4.1 supplies the vendored diagram runtime for this review package.
The runtime comes from `mermaid@11.4.1/dist/mermaid.min.js` in the published npm distribution.
The review pages need no external asset request.

## Protocol status

The two-of-three release design is a proposal, not a verified external protocol.
Three-agent renewal reduces the chance of an agent retaining an old accepted deadline after a successful renewal.
It does not by itself prove safe behavior during a partial renewal.
The dedicated review must resolve this case before implementation can make a production claim.

The total-loss research epic compares external execution, independent custody, and durable ciphertext distribution.
The project has approved no external execution provider or paid dependency.
