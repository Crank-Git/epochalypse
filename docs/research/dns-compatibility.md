# DNS adapter compatibility

Status: verified synthetic compatibility, 2026-11-04.
Select `codeberg.org/miekg/dns v0.6.128` for the production adapter in #8.
The owner-access dependency still applies to production mutations.

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

`TestResolverMatrix` also uses the successor for every authority and client operation.
It generates synthetic ECDSAP256SHA256 keys and signs each RRset through the successor.
The pinned resolvers validate its current TXT signature through a local DNSKEY trust anchor.
The compatibility gate for #8 passes for this configuration.
Keep the legacy adapter in the research module for interoperability checks only.

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
go -C tests/compatibility/dns test -race -count=1 ./...
go -C tests/compatibility/dns vet ./...
```

The module checksum database verifies both downloads.
The separate module keeps research dependencies outside the production module.
The root `make check` does not discover this nested module.
Run the compatibility command separately.
The matrix requires Docker with Linux arm64 support and Linux amd64 emulation.
It compiles a static Linux arm64 authority from the test binary.
It creates a unique Docker network and seven containers.
It publishes DNS ports on host loopback with automatic port allocation.
Test cleanup removes only those containers and that network.
A missing Docker prerequisite fails the test rather than skips it.

## Resolver pins

These image versions report their identity through their own executables.
The matrix uses these exact images for every run.

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

## Resolver observations

The fixture uses only generated synthetic keys and `demo.example.`.
Each DNSKEY signature covers the interval from 24 hours before the fixture time to 24 hours after it.
Each TXT RRset has one signature and no valid alternate signature.
The current signature covers one hour before and after the fixture time.
The near-future signature starts 20 minutes after the fixture time and lasts one hour.
The far-future signature starts 24 hours after the fixture time and lasts one hour.
The expired signature ends 24 hours before the fixture time.
The changed-content case retains the original signature over different TXT content.

The recorded run uses fixture time `2026-11-04T20:22:21Z`.
Every resolver uses the Docker VM clock with configured offset zero.
A container clock sample bounds its offset from the host between -6.173 ms and +30.579 ms.
Each run records a fresh clock sample and executable identity.
The test rejects a clock interval wholly outside one second from the host.
This bound supports the experiment's wide intervals; it does not establish exact boundary acceptance.

With the correct trust anchor and CD clear:

| TXT case | Default Unbound 1.22.0 | Zero-skew Unbound 1.22.0 | BIND 9.20.24 |
|---|---|---|---|
| Current | NOERROR, AD | NOERROR, AD | NOERROR, AD |
| Near-future | NOERROR, AD | SERVFAIL, no AD | SERVFAIL, no AD |
| Far-future | SERVFAIL, no AD | SERVFAIL, no AD | SERVFAIL, no AD |
| Expired | SERVFAIL, no AD | SERVFAIL, no AD | SERVFAIL, no AD |
| Changed content | SERVFAIL, no AD | SERVFAIL, no AD | SERVFAIL, no AD |

Every CD query returns NOERROR with the TXT and RRSIG.
CD queries retain AD for the previously validated current and accepted near-future answers in this query order.
Every other CD answer has AD clear.
Every recursive response has RA set.
The test repeats all cases with a different generated trust anchor.
With that wrong anchor, every CD-clear query returns SERVFAIL without AD.
Every CD query with that wrong anchor returns the public TXT without AD.
The current-signature positive control prevents a broken chain from masquerading as correct window rejection.

The default Unbound configuration retains its signature skew defaults.
The version-specific source defines a minimum of 3600 seconds and a maximum of 86400 seconds.
The strict configuration sets both `val-sig-skew-min` and `val-sig-skew-max` to zero.
Both configurations use a stub zone and an explicit local DNSKEY trust anchor.
BIND uses a static-stub zone, `dnssec-validation yes`, and a `static-key` trust anchor.
The fixtures retain the complete generated configurations in the test source.

Direct UDP and TCP queries return the far-future TXT and RRSIG before inception.
Both responses have AA set and RA clear.
The authority refuses names and types outside the fixture records.
A 512-byte TXT uses strings of 255, 255, and 2 bytes.
Its 512-byte UDP limit produces TC.
An explicit TCP retry returns all 512 bytes and the RRSIG.
The successor's client requires the caller to perform TCP fallback.
The successor's server requires the handler to unpack the full request before it reads the DO flag.

Public TXT content remains visible before validation accepts its signature.
SERVFAIL and AD do not establish confidentiality.
These observations cover the configured local trust anchor, not a public delegation chain.
Failure-cache transitions across inception remain the responsibility of #9.
Production authenticated denial, rollover, and authority lifecycle remain the responsibility of #8 and #11.

## Verified interfaces

Retrieved 2026-11-04:

- [v1.1.73 README](https://github.com/miekg/dns/blob/v1.1.73/README.md): maintenance scope and successor location.
- [v1.1.73 DNSSEC source](https://github.com/miekg/dns/blob/v1.1.73/dnssec.go): signing and verification contracts.
- [Successor v0.6.128 module](https://codeberg.org/miekg/dns/src/tag/v0.6.128/go.mod): toolchain and dependencies.
- [Successor migration guide](https://codeberg.org/miekg/dns/src/tag/v0.6.128/_doc/README-v1-to-v2.md): source compatibility changes.
- [Successor DNSSEC source](https://codeberg.org/miekg/dns/src/tag/v0.6.128/dnssec.go): `Sign`, `Verify`, and `ValidPeriod` contracts.
- [Successor server interface](https://codeberg.org/miekg/dns/src/tag/v0.6.128/serve_mux.go): full request decoding in a handler.
- [Successor client interface](https://codeberg.org/miekg/dns/src/tag/v0.6.128/client.go): bounded context and explicit TCP fallback.
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
The resolver matrix supplies the synthetic acceptance evidence for #7.
It does not add production routes or change the production module.
