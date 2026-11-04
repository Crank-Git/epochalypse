# Signed TXT windows

Window content is public data. A future DNSSEC signature does not keep content secret.
The current build stores signed records through HTTP and the local CLI.
Live authoritative service remains in progress in issue #8.

## Configuration

Supply an algorithm 13 BIND key pair for a dedicated experimental subdomain.
The `.private` file and its sibling `.key` file have the same basename.
The private file requires owner-only access. Use private-key format v1.3 for new files.
ADR 0006 records the key and time policies.

Set `EPOCHALYPSE_KEY_FILE` to the private file path.
Set the zone in the version 1 configuration:

```json
{"version":1,"zone":"windows.example.net."}
```

The server accepts `--zone` and `--key-file`. The zone flag overrides the file setting.
Outside the loopback lab, supply `--tls-cert` and `--tls-key` for owner HTTP access.
The `--local-lab` flag permits HTTP owner access only on a loopback address.
A server without a zone and key retains existing owner routes. Authenticated window creation then returns 503.
The DNS-only scaffold rejects signed-window configuration.

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
The authority query interface and empty-zone key metadata arrive with the listener slice.

Verified fixture interface, retrieved 2026-11-04:
[Node.js v26.13.0 crypto](https://nodejs.org/api/crypto.html#cryptogeneratekeypairsynctype-options) supplies synthetic keys for the pinned Chromium container.
