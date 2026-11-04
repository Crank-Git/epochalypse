# Signed TXT windows

Window content is public data. A future DNSSEC signature does not keep content secret.
The current build stores signed records through HTTP and the local CLI.
The main process serves authoritative UDP/TCP DNS from the same committed state.
The [Compose lab](lab.md) uses explicit synthetic DNS keys and self-signed TLS.

## Configuration

Supply an algorithm 13 BIND key pair for a dedicated experimental subdomain.
The `.private` file and its sibling `.key` file have the same basename.
The private file requires owner-only access. Use private-key format v1.3 for new files.
ADR 0006 records the key and time policies.

Set `EPOCHALYPSE_KEY_FILE` to the private file path.
Set the zone in the version 1 configuration:

```json
{"version":1,"zone":"windows.example.net.","nameserver":"ns.operator.example.org.","public_url":"https://admin.example.net"}
```

Replace the reserved example domains with your delegated production names.
The server accepts `--zone`, `--nameserver`, `--public-url`, and `--key-file`.
Flags override the corresponding file settings.
The nameserver must use a canonical name outside the experimental zone.
The public origin must keep administration and release discovery outside that zone.
Outside the loopback lab, supply `--tls-cert` and `--tls-key` for owner HTTP access.
The `--local-lab` flag permits HTTP owner access only on a loopback address.
A server without a zone and key retains existing owner routes. Authenticated window creation then returns 503.
The DNS-only scaffold rejects signed-window configuration.

`EPOCHALYPSE_ENV` selects `lab` or `production`. The CLI default is `lab`, as the project spec requires.
An absent environment in the service configuration uses production restrictions for key material.
Unknown environment values fail validation.
Production rejects reserved lab zones, synthetic nameserver targets, and keys marked `epochalypse-lab-only`.
Explicit `ENV=lab` admits synthetic DNS material independently of the HTTP transport policy.
It does not permit non-loopback plaintext owner access.

## Creation API

Send `POST /api/v1/windows` with `Content-Type: application/json`.
Use an authenticated owner session cookie and the current `X-CSRF-Token` proof.
`GET /api/v1/csrf` supplies the proof and matching proof cookie.
Bearer tokens cannot create windows. An Authorization header cannot fall back to the session cookie.

```json
{
  "content": "public experiment",
  "inception": "2026-11-04T13:00:00Z",
  "expiration": "2026-11-04T14:00:00Z"
}
```

All three fields are required strings. Empty content is valid.
Content permits at most 512 UTF-8 bytes. The request body permits at most 8192 bytes.
The API rejects unknown fields, malformed UTF-8, and multiple JSON documents.
Use whole-second UTC timestamps with the `Z` suffix.
Expiration must follow inception within the supported serial-time range.

A successful response returns 201 with the committed public `Window` document.
Its fields are `version`, `id`, `content`, `zone`, `name`, `inception`, `expiration`, `dnskey`, `txt`, and `rrsig`.
Errors use `{error:{code,message,fields,request_id}}`.

| Status | Result |
|---|---|
| 400 | The JSON document or a named window field is invalid. |
| 401 | The owner session fails. A bearer credential also fails. |
| 403 | The CSRF proof or origin fails. |
| 409 | The retained window quota rejects creation. |
| 413 | The request exceeds 8192 bytes. |
| 426 | Owner access requires HTTPS. |
| 503 | The signer or store is unavailable. |

## Local CLI

Stop the main process before a local window command.
The database lock rejects an independent CLI reader or writer while the server owns the store.
Use the authenticated API for live creation.
For inspection, set `WINDOW_ID` to the identifier returned by creation.

```sh
epochalypse window create \
  --state-dir var/state --zone windows.example.net. \
  --key-file /protected/window.private --content 'public experiment' \
  --inception 2026-11-04T13:00:00Z --expiration 2026-11-04T14:00:00Z

epochalypse window inspect \
  --state-dir var/state --zone windows.example.net. \
  --key-file /protected/window.private --id "$WINDOW_ID"
```

Both commands print public JSON and require host access to the state and key files.
Inspection verifies the stored records. It retains expired records and the original signature.

## Retention limits

`EPOCHALYPSE_MAX_OBJECTS` defaults to 100. `EPOCHALYPSE_QUOTA_BYTES` defaults to 1073741824.
The server and window commands accept corresponding `--max-objects` and `--quota-bytes` flags.
Each override must be a positive integer.
The store atomically checks its document count and encoded JSON bytes before creation.
A quota failure leaves accepted records unchanged.
Database page overhead and the fixed configuration record do not enter the JSON-byte count.

Windows are the first implemented retained content type. These counters cover window documents only.
Future capsules, switches, countdowns, and observations must contribute to one installation budget before they admit retained state.
Their implementation must define the shared transaction or reservation mechanism.
Do not give each domain store a separate full installation allowance.

## Producer contracts

`window.Store.Get` and `List` return verified immutable `Window` values.
`Window.Records` returns independent TXT, RRSIG, and DNSKEY values.
Observation runs and public pages consume these reads without another TXT signature.
The store binds its zone and DNSKEY at first access. A changed DNSKEY requires an explicit migration.

The main service owns store lifetime in `internal/service/state.go`.
`mainState.routes` registers owner routes and window creation.
New owner mutations use `owner.RequireOwner` inside `owner.SecureHandler`.
`owner.APIError` supplies the shared error shape.
Issues #9 through #11 retain observation routes, public pages, and trust diagnostics.

`authority.New` consumes the store, configured signer, and external nameserver name.
`Authority.Metadata` returns independent public DNSKEY and derived `ParentDS` values even before the first window exists.
`ParentDS` is parent-publication metadata. The child authority does not publish a DS RRset at its own apex.
`authority.Query` applies a two-second bound across UDP and explicit TCP fallback.
It returns the response and the final transport without an interpretation of validation flags.
Observation and diagnostic CLIs use this network path while the main process owns the database.
`Store.Revision` and `Snapshot` support an ephemeral authority cache. The revision is not a recovery or observation sequence.

## Authority behavior

TXT and RRSIG answers preserve the original signature before inception and after expiration.
The infrastructure signer accepts DNSKEY, NS, SOA, and NSEC records. It rejects TXT records.
The authority refreshes infrastructure signatures when the UTC hour changes.
Window creation invalidates the ephemeral record cache after the durable commit.
The NSEC chain covers every existing name and proves absent names, wildcards, and types.
An apex DS query receives authoritative NODATA with a signed denial.
Publish the derived DS at the delegating parent when you configure a public trust chain.
The operator must keep the nameserver address and parent NS/DS records valid during the experiment.
The project changes no external delegation automatically.

The listener refuses recursion, zone transfers, ANY queries, and names outside the experiment.
TCP permits 64 active clients and 64 queries per connection.
Each connection operation has a one-second deadline. Shutdown closes active clients before the store closes.
UDP responses respect the request limit up to 1232 bytes. Truncation requires an explicit TCP retry.

Question labels support wire bytes 33 through 126, except literal dots and backslashes inside a label.
The wire guard checks these bytes before the adapter decodes the name.
Unsupported octets receive REFUSED without an authoritative answer or denial proof.
The guard bounds compression walks to 128 steps and complete names to 255 wire bytes.
Malformed names receive FORMERR.
These restrictions prevent the adapter's dotted text representation from changing wire-label boundaries.

Direct access exposes public content. Resolver acceptance remains an observation, rather than an exact-time or confidentiality guarantee.
## Deployment trust check

Run the check while the main process serves DNS. The check requires no private key or database access.
Obtain the public anchor through a trusted operator channel before the check.
The anchor file contains one algorithm 13 DNSKEY or one SHA-256 DS record for the experimental zone.
Use one record on one line. Directives, comments, and record groups are invalid.

```sh
epochalypse lab verify --zone windows.example.net. \
  --address 127.0.0.1:1053 --anchor-file /protected/window.key
```

The address requires an IP address and port. The zone requires a canonical lowercase subdomain with a final dot.
The check requests DNSKEY and SOA records with DO and CD. It verifies signatures locally, independently of AD.
A DNSKEY anchor must match a key in the signed DNSKEY set.
A DS anchor must match the derived SHA-256 digest of a key in that set.
The authenticated set then verifies the current SOA signature.
This permits an additional key in the signed set. It does not implement production key rotation or a store migration.

The JSON result names `status`, `trust_source`, `zone`, and `detail`.
Only `verified` returns exit code 0. Invalid command input returns 2. Other diagnostic results return 1.

| Status | Meaning |
|---|---|
| `verified` | The explicit anchor verifies current DNSKEY and SOA infrastructure. |
| `bogus` | The supplied anchor or an infrastructure signature fails verification. |
| `insecure` | The authority supplies no DNSKEY. Public delegation remains unverified. |
| `unreachable` | The bounded DNS exchange fails. |
| `indeterminate` | The response, clock, target, or unsupported public chain prevents verification. |

An explicit anchor uses `trust_source: explicit-local-anchor`. It establishes no complete public delegation chain.
Without `--anchor-file`, the check reports `indeterminate` with `trust_source: none` and makes no DNS queries.
Complete public delegation verification remains unsupported. AD alone cannot establish that chain.
The child apex correctly supplies signed NODATA for DS. A supplied DS anchor represents trusted parent-publication metadata.
The check does not query the child apex for DS or change any parent delegation.

The check separates current infrastructure from deliberately future or expired experimental TXT signatures.
It makes no claim about exact resolver acceptance or confidentiality.
Signature times use whole UTC seconds from 1970 through the final unsigned 32-bit timestamp.
The check rejects wrapped intervals and intervals of 2147483648 seconds or more.
Both signature boundaries are inclusive. The check applies no clock skew allowance.

Anchor input permits at most 4096 bytes. Each DNS answer permits at most 16 records.
Two lookups share a four-second deadline. Each lookup uses the authority's two-second UDP/TCP fallback bound.
The check bounds signature attempts through the record limit. Parser errors exclude file contents.

Verified interfaces, retrieved 2026-11-04:
[codeberg.org/miekg/dns v0.6.128](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128) supplies `RRSIG.Verify`, `DNSKEY.ToDS`, and public record parsers.
Its shipped documentation requires separate RRset, covered-type, and time checks around `RRSIG.Verify`.
[RFC 4035, sections 4.4 and 5.3](https://www.rfc-editor.org/rfc/rfc4035.html#section-5.3) defines explicit anchors and signature verification.

Protocol sources:
[RFC 4034, section 6.1](https://www.rfc-editor.org/rfc/rfc4034.html#section-6.1) defines canonical name order.
[RFC 4035, section 3.1.4.1](https://www.rfc-editor.org/rfc/rfc4035.html#section-3.1.4.1) defines the child-apex DS response.

Verified fixture interface, retrieved 2026-11-04:
[Node.js v26.13.0 crypto](https://nodejs.org/api/crypto.html#cryptogeneratekeypairsynctype-options) supplies synthetic keys for the pinned Chromium container.
