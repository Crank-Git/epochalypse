# ADR 0006: Keep signed windows in the main process

Date: 2026-11-04
Status: accepted technical choice for issue #8

## Service and state

The main process owns HTTP and authoritative UDP/TCP DNS listeners.
This process opens owner state and window state separately.
The three agent processes retain their independent volumes.
The owner session protects window creation through the existing CSRF and TLS controls.
The local CLI uses host access while the main process is stopped.
No window token scope has approval.

Use bbolt v1.5.0 for `v1/windows.db` under the main state directory.
The database file requires mode 0600. The versioned directory requires mode 0700.
The database lock has a one-second timeout.
A second process cannot mutate live state through an independent database handle.
A separate DNS process would require replication and conflict rules.
One process makes a committed window immediately available to its authoritative listener.

The `windows-v1` bucket contains immutable JSON documents with `version: 1`.
The `configuration-v1` bucket binds the database to its original zone and DNSKEY.
The store refuses a different zone or DNSKEY.
A future key migration requires an explicit migration contract.
This slice supplies no migration from an earlier window format because none exists.
The public fixture in `internal/window/testdata/window-v1.json` fixes the initial format.
The fixture contains no private key.

Each document contains public content, an identifier, the zone, explicit boundaries, and the signed records.
The store commits the signature before it returns the window.
Reads verify the signature and its agreement with the public metadata.
Reads retain expired records for experiments.
Restart does not sign the TXT RRset again.
The store has no update operation and no alternate TXT signature.

## Key policy

Use DNSSEC algorithm 13, ECDSAP256SHA256.
The compatibility experiment verifies this algorithm against both adapters and the pinned resolver matrix.
The selected adapter supports BIND private-key formats v1.2 and v1.3.
Use the v1.3 format for new installations.

`EPOCHALYPSE_KEY_FILE` references a `.private` file.
Its sibling `.key` file contains the public DNSKEY record with the same basename.
The private file requires owner-only access.
Both files must be regular files of at most 16384 bytes.
The loader rejects symbolic links and mismatched key pairs.
The DNSKEY uses flags 257, protocol 3, and the configured canonical zone.
The zone must contain at least two nonempty labels.
The operator assigns a dedicated experimental subdomain.
Do not reuse the administration or release-discovery zone.

Production never generates a key implicitly.
Lab and test key generation stays explicit and isolated from production configuration.
Parser errors exclude private key material.
Only the public DNSKEY appears in window state.

## Content and time policy

Accept at most 512 UTF-8 bytes of public content.
Split TXT strings at 255 wire bytes.
The adapter uses DNS presentation escapes in its TXT fields.
Escape each content byte before the adapter packs the record.
The full content remains UTF-8 even when a character crosses a TXT string boundary.

Use whole-second UTC timestamps with the `YYYY-MM-DDTHH:MM:SSZ` format.
Support Unix timestamps from 0 through 4294967295.
Require expiration after inception by less than 2147483648 seconds.
At creation, each boundary must remain less than 2147483648 seconds from the supplied clock.
These restrictions reject ambiguous serial-time comparisons instead of silently wrapping timestamps.
The caller supplies the clock for deterministic tests.
The authoritative listener serves the stored signature before inception and after expiration.
This behavior exposes public data. It does not provide confidentiality.

## Verified interfaces

Retrieved 2026-11-04:

- [dns v0.6.128](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128): shipped `DNSKEY.NewPrivate`, `PrivateKeyString`, `NewRRSIG`, `Sign`, `Verify`, and TXT types.
- [bbolt v1.5.0](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0): shipped `Open`, `Options.Timeout`, transactions, and bucket operations.
- [Go ECDSA](https://pkg.go.dev/crypto/ecdsa): Go 1.27.3 shipped `PrivateKey.Bytes` and `ParseRawPrivateKey` validate the private scalar and derive its public key.
- [RFC 4034, section 3.1.5](https://www.rfc-editor.org/rfc/rfc4034.html#section-3.1.5): inception and expiration use unsigned 32-bit serial time.
- [Compatibility evidence](../research/dns-compatibility.md): algorithm 13 interoperates with the pinned resolver configurations.

## Authority integration

Use the selected adapter's codec and signature APIs with the project's bounded listener transport.
The adapter's shipped `Server.Shutdown` documentation states that its context deadline is not implemented.
The project instead bounds active TCP clients, query counts, and connection operations.
Shutdown closes these clients before it closes the database.

The authority caches one verified snapshot per database revision and UTC hour.
Infrastructure signatures span the current hour and refresh on later queries.
Only the owner's original signature covers the TXT RRset.
The infrastructure signer rejects TXT records.

DS records belong at the delegating parent.
The child-only authority returns signed NODATA for an apex DS query.
Its typed metadata exposes the public DNSKEY and derived parent DS.
See [RFC 4035, section 3.1.4.1](https://www.rfc-editor.org/rfc/rfc4035.html#section-3.1.4.1).

An explicit external nameserver separates authoritative infrastructure from future-signature records.
The configured HTTPS origin keeps administration and release discovery outside the experimental zone.
Production rejects reserved lab names and marked synthetic keys.
Explicit `EPOCHALYPSE_ENV=lab` classifies synthetic material separately from HTTP transport safeguards.
Non-loopback owner access still requires TLS in that environment.
The existing loopback-only insecure HTTP option retains its original guard.

The adapter does not preserve arbitrary wire-label octets in its dotted name representation.
A literal dot inside one label can otherwise change canonical NSEC order or alias an existing window.
The authority checks label octets before the adapter decodes the question.
Unsupported forms receive a non-authoritative refusal without an answer or denial proof.
`docs/windows.md` defines the supported wire bytes and parser bounds.
