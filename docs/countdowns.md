# Countdown foundation

The countdown store shares the installation database with windows, observations, and capsules.
The `countdowns-v1` bucket retains version 1 JSON records.
Admission charges one object and the exact encoded record bytes before the transaction writes the record.
Deletion releases both charges in the same transaction.
An empty countdown zone disables creation. It preserves inspection and deletion.

Each record contains an opaque identifier, title, absolute event time, creation time, zone, and actual DNS owner name.
The DNS owner name is `c<id>.<configured countdown zone>`.
The event uses whole UTC seconds. Admission compares the event against the precise current time.
The event must be in the future and within 2147483647 seconds.
The store preserves the event timestamp after completion until owner deletion.

Stop the main server before a local command opens the installation database.

```sh
epochalypse countdown create --state-dir var/state \
  --zone countdown.example. --key-file /protected/countdown.private \
  --title 'Synthetic event' --event-at 2026-12-01T12:00:00Z
epochalypse countdown inspect --state-dir var/state --id <id>
```

Creation verifies the configured key reference. It creates no key or delegation.
Inspection requires no active zone or key reference.
Both commands return the retained public metadata as JSON.
The commands use the shared `--max-objects` and `--quota-bytes` limits.

The typed producer for the next countdown slices is `internal/countdown.Store`.
`Get(id)` returns one immutable `Record`. `List()` returns a snapshot for an authoritative adapter.
`Record.TXT()` returns an independent TXT record with `v=epochalypse1 event=<UTC timestamp>`.
Its initial TTL is 2147483647. The signer uses that value as the signed Original TTL.
The authoritative adapter then lowers the emitted TTL without changing the event data.
The producer preserves the current time's subsecond precision for the later TTL calculation.

`countdown.LoadSigner(zone, path)` verifies the separate protected countdown key reference.
`Signer.Records(record, now)` validates the retained metadata and returns independent TXT, RRSIG, and DNSKEY records.
Each call signs the same absolute event data with ordinary validity from `now - 5 minutes` through `now + 48 hours`.
Call this producer before signature expiration. It renews validity after the event without rewriting the event time.
The producer uses the existing protected key loader. It preserves explicit signature windows in the window experiment.
It rejects records from another zone and clocks outside the supported DNSSEC timestamp range.
No signature cache retains deleted countdowns.

The main server accepts `--countdown-zone` and `--countdown-key-file`.
The version 1 configuration accepts `countdown_zone`.
`EPOCHALYPSE_COUNTDOWN_KEY_FILE` supplies the protected key reference when the flag is absent.
The countdown authority runs with window DNS disabled.
It uses a separate zone and signing material. Equal or nested zones fail configuration validation.
Equivalent key paths fail configuration validation. Reused public key material fails startup.
Production configuration requires TLS, a separate public HTTPS origin, and an explicit external nameserver.
The server rejects explicit lab keys outside the lab environment.
The service attaches countdown state before it tests whether window DNS is enabled.
It keeps the same installation writer and quota across all content domains.

`POST /api/v1/countdowns` accepts a JSON object with `title` and `event_at` strings.
It returns 201, retained metadata, and the actual DNS name.
`GET /api/v1/countdowns/{id}` returns retained metadata.
`DELETE /api/v1/countdowns/{id}` accepts an empty body and returns 204.
Each route requires the owner session or a `countdown:write` token.
Session mutations require CSRF proof. Production HTTP requests require TLS.
Creation limits the JSON body to 4096 bytes. Unknown fields and additional JSON documents fail.
Inspection and deletion accept no request body. Each route rejects query parameters.
Four request slots and explicit deadlines bound concurrent work.
The shared error response reports invalid input, unavailable state, cancellation, and exhausted quota.
The store checks cancellation before the transaction commits each mutation.

The owner dashboard links to `/owner/countdowns`.
The page accepts an absolute UTC event time, shows retained metadata, and supports deletion.
The page escapes retained titles and DNS names. Its script inserts returned metadata as text.
The Chromium check creates an event with keyboard input, checks rejected times, and deletes an event.
Offline CLI inspection verifies the retained event after service shutdown.

`authority.NewCountdown` serves the separate signed zone through the main DNS listeners.
Each query reads a current store snapshot and renews ordinary signatures.
The authority serves DNSKEY, NS, SOA, and signed denial records for this zone.
Window TXT records retain their owner's explicit signature window.
Issue #37 supplies the final authoritative TTL and resolver proof.
Issue #38 supplies the public page and advisory labels.
The batch ships these slices together.

Verified interfaces, retrieved 2026-11-04:

- [bbolt v1.5.0](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0), transaction and bucket methods; checked against the downloaded module source.
- [DNS adapter v0.6.128](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128), TXT headers and RRSIG methods; checked against the downloaded module source.
- [RFC 4035 section 5.3.3](https://www.rfc-editor.org/rfc/rfc4035.html#section-5.3.3), validation limits on cached TTL values.
- [Go HTTP package](https://pkg.go.dev/net/http), Go 1.27.3 request limits and response deadlines; checked with `go doc`.
