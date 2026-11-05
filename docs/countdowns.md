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
Its initial TTL is 2147483647. The signer must use that value as the signed Original TTL.
The authoritative adapter then lowers the emitted TTL without changing the event data.
The producer preserves the current time's subsecond precision for the later TTL calculation.

The remaining #36 work adds the signed producer, HTTP adapters, service configuration, and owner controls.
Issue #37 supplies the final authoritative TTL and resolver proof.
Issue #38 supplies the public page and advisory labels.
The batch ships these slices together.

Verified interfaces, retrieved 2026-11-04:

- [bbolt v1.5.0](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0), transaction and bucket methods; checked against the downloaded module source.
- [DNS adapter v0.6.128](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128), TXT headers and RRSIG methods; checked against the downloaded module source.
- [RFC 4035 section 5.3.3](https://www.rfc-editor.org/rfc/rfc4035.html#section-5.3.3), validation limits on cached TTL values.
