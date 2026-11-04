# DNS adapter compatibility

Status: partial evidence, 2026-11-04. Issue #7 remains incomplete.
Production DNS implementation in #8 remains gated.

## Adapter evidence

The synthetic compatibility module pins both candidates.
It changes no production dependency.

| Candidate | Exact version | Upstream commit | Maintenance evidence |
|---|---|---|---|
| github.com/miekg/dns | v1.1.73 | d854399da1ee385b432e8b07f79e53bbfc1ab1b0 | The version-specific README limits future work to specific fixes. |
| codeberg.org/miekg/dns | v0.6.128 | c4738ca893c8f36e43f3e128b08974734b321b9d | The upstream README identifies this repository as the successor. Its module requires Go 1.27.0. |

The successor has different source APIs.
`New` replaces `NewRR` for record text.
`RRSIG.Sign` and `RRSIG.Verify` accept `*SignOption`.
`ValidPeriod` checks the signature window separately from cryptographic verification.
The fixture passes explicit UTC inception and expiration values.
It checks each boundary and its adjacent seconds with fixed times.

`TestSuccessorVerifiesLegacySignature` passes on Go 1.27.3.
It verifies a v1 ECDSAP256SHA256 signature through the successor.
It verifies the successor signature through v1.
Both adapters produce the same SHA-256 DS digest.
The successor rejects changed TXT content.
These results establish signature interoperability for this fixture.
They do not establish recursive validation or authoritative visibility.

No adapter has final approval from this report yet.
The successor is the preferred candidate for the remaining resolver experiment.
The experiment must pass before #8 treats the compatibility gate as resolved.

## Local prerequisite

Set every Go state path inside the worktree.
Go uses GOPATH for checksum state even when GOMODCACHE has a separate location.
The default GOPATH causes a sandbox refusal on this machine.
Keep checksum verification enabled.

Run these commands from the worktree root:

```sh
mkdir -p var/go var/cache/go-mod var/cache/go-build var/tmp
export GOPATH="$PWD/var/go"
export GOMODCACHE="$PWD/var/cache/go-mod"
export GOCACHE="$PWD/var/cache/go-build"
export GOTMPDIR="$PWD/var/tmp"
go -C tests/compatibility/dns test -count=1 -v ./...
```

The module checksum database verifies both downloads.
The separate module keeps research dependencies outside the production module.
The root `make check` does not discover this nested module.
Run the compatibility command separately.

## Resolver pins

These image versions report their identity through their own executables.
The matrix experiment has not run yet.

| Resolver | Image | Platform | Observed executable version |
|---|---|---|---|
| Unbound | `mvance/unbound@sha256:76906da36d1806f3387338f15dcf8b357c51ce6897fb6450d6ce010460927e90` | linux/amd64 | 1.22.0 |
| BIND | `ubuntu/bind9@sha256:8f62529754757e8cb7bbf27f43a1d280f9c5f92c2d30704565242058d8d24872` | linux/arm64 | 9.20.24-1ubuntu0.4-Ubuntu |

```sh
docker run --rm --platform linux/amd64 --entrypoint /opt/unbound/sbin/unbound mvance/unbound@sha256:76906da36d1806f3387338f15dcf8b357c51ce6897fb6450d6ce010460927e90 -V
docker run --rm --platform linux/arm64 --entrypoint /usr/sbin/named ubuntu/bind9@sha256:8f62529754757e8cb7bbf27f43a1d280f9c5f92c2d30704565242058d8d24872 -V
```

The BIND image starts Pebble by default.
Use an explicit entrypoint for executable checks.
The host Unbound reports 1.24.2.
Neither observed Unbound version is the cited 1.26.2.
Use the pinned 1.22.0 documentation for the matrix.

## Remaining experiment

Use only generated synthetic keys and `demo.example.`.
Keep DNSKEY signatures valid while TXT signatures cover future, current, and expired windows.
Serve each signed TXT RRset without a valid alternate signature.
Verify direct authoritative TXT access before inception over UDP and TCP.
Compare default Unbound, zero-skew Unbound, and BIND through a local trust anchor.
Require a current-window AD result as the positive trust-anchor control.
Record future-window and expired-window results with CD clear and set.
Record versions, clock offsets, response codes, and flags.
Verify changed content and a wrong trust anchor fail validation.
Keep failure-cache observations separate from signature arithmetic.
Public TXT content remains visible before validation accepts its signature.

## Verified interfaces

Retrieved 2026-11-04:

- [v1.1.73 README](https://github.com/miekg/dns/blob/v1.1.73/README.md): maintenance scope and successor location.
- [v1.1.73 DNSSEC source](https://github.com/miekg/dns/blob/v1.1.73/dnssec.go): signing and verification contracts.
- [Successor v0.6.128 module](https://codeberg.org/miekg/dns/src/tag/v0.6.128/go.mod): toolchain and dependencies.
- [Successor migration guide](https://codeberg.org/miekg/dns/src/tag/v0.6.128/_doc/README-v1-to-v2.md): source compatibility changes.
- [Successor DNSSEC source](https://codeberg.org/miekg/dns/src/tag/v0.6.128/dnssec.go): `Sign`, `Verify`, and `ValidPeriod` contracts.
- [RFC 4034](https://www.rfc-editor.org/rfc/rfc4034.html): signature time fields and DS format.
- [RFC 4035](https://www.rfc-editor.org/rfc/rfc4035.html): resolver signature validation and CD behavior.
- [Unbound 1.22.0 configuration source](https://github.com/NLnetLabs/unbound/blob/release-1.22.0/doc/example.conf.in): trust anchors, stub zones, and signature skew.
- [BIND 9.20.24 reference](https://bind9.readthedocs.io/en/v9.20.24/reference.html): resolver configuration.

The installed Docker help verifies `run`, `--entrypoint`, image digests, and image inspection.

## Check scope

This change adds research tests against external adapters.
It adds no first-party implementation behavior or user-facing interface.
TDD fail/pass evidence and browser E2E do not apply to this research-only slice.
First-party production coverage remains a separate root-suite gate.
The resolver matrix and full acceptance evidence remain required before completion.
