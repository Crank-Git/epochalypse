# Switch drafts

The owner configures an unarmed switch at `/owner/switches`.
The owner dashboard links to these controls.
A draft stores a check-in interval and grace period.
It has no content, agent copy, accepted deadline, or release authority.
Production switch use remains disabled under ADR 0013.

The default interval is 86400 seconds (24 hours).
The default grace is 3600 seconds (one hour).
The interval permits 3600 through 31536000 whole seconds, inclusive.
Grace permits zero through 2592000 whole seconds, inclusive.
The page shows proposed deadline and release boundary values in UTC and local browser time.
These values illustrate an arm at the current time. Agents do not accept these preview values.
Grace remains a separate duration.

## Owner API

`POST /api/v1/switches` creates a draft and returns 201.
An empty JSON object selects the defaults.
The optional `interval_seconds` and `grace_seconds` fields accept integer seconds.

```json
{"interval_seconds":86400,"grace_seconds":3600}
```

`GET /api/v1/switches/{id}` returns the retained draft and status 200.
Both routes require an owner session or an exact `switch:write` token over HTTPS.
Session mutations also require CSRF proof.
The routes reject query parameters and bodies above 4096 bytes.
Creation rejects malformed JSON, unknown fields, and invalid ranges.
Inspection requires an empty body.
The shared error format reports invalid requests, missing drafts, quota limits, and unavailable storage.
A draft response contains these fields:

- `version`: record schema version 1.
- `id`: a random 32-character lowercase hexadecimal identifier.
- `interval_seconds`: the configured check-in interval.
- `grace_seconds`: the configured grace period.
- `created_at`: the creation time in whole UTC seconds.
- `state`: `draft`, which means unarmed and unreplicated.

## Local CLI

Stop the main server before the CLI opens its state directory.

```sh
epochalypse switch create --state-dir var/state --interval-seconds 86400 --grace-seconds 3600
epochalypse switch status --state-dir var/state --id <id>
```

Both commands return the same JSON draft record.
Creation without duration flags selects the defaults.
These commands do not arm a switch.

## State contract

Drafts use the shared `v1/windows.db` installation database and the `switches-v1` bucket.
Each draft consumes one retained object and its encoded record length in bytes.
All content domains share the installation retention limits.
`switches.ValidateRecord` validates the version-one draft representation for main-state backup consumers.
Consumers must also reject unknown JSON fields and verify that the bucket key equals the record identifier.
The main database contains no authoritative agent state.
Public arm and renewal evidence extend this versioned contract. Backup validation authenticates the retained evidence.

## Direct check-ins

The CLI sends signed check-ins directly to the three enrolled HTTPS agents.
The main service holds no owner signing key, private plan, ciphertext, or key share.
The client journal preserves exact signed intent before transmission. A new CLI process retries the same incomplete request.

```sh
epochalypse switch checkin --plan plan.json --owner-key owner.key --pins pins.json --journal recovery.json.checkins --evidence renewal.json
epochalypse switch status --plan plan.json --owner-key owner.key --journal recovery.json.checkins
```

These commands require no main service. The optional evidence export occurs after direct agent requests.
An export error cannot prevent those requests. The journal retains the evidence for a later retry.
The JSON result contains each agent receipt and the public `evidence` object.
The optional export contains only that public object.
Import it on the owner switch page, or submit it to `POST /api/v1/switches/{id}/checkins`.
This route requires the same owner authorization and CSRF proof as arm evidence.
Its body limit is 65536 bytes. Unknown fields, invalid signatures, and evidence regressions return 400.
The route returns 202 for incomplete receipts. It returns 200 only for three authenticated matching durable receipts.
It sends no agent request and creates no owner signature.

The proof has four fields: `confirmed_request`, `confirmed_receipts`, `request`, and `receipts`.
The confirmed request carries three matching receipts. The current request carries each available receipt.
Main derives the disclosure bounds from these authenticated requests. It rejects caller-supplied bounds.
`GET /api/v1/switches/{id}` includes the retained `renewal` proof and derived `renewal_status`.
Status contains `confirmed`, `b0`, `bmax`, `deadline`, `grace_seconds`, `allowance_assumptions`, and `agents`.
Each agent has an `id` and a `state` of `durable` or `uncertain`.

Three matching durable receipts confirm renewal. Incomplete receipts leave an uncertain outcome.
B0 identifies the earliest possible disclosure boundary after an uncertain outcome.
Bmax identifies the latest already-issued signed release boundary. E assumptions remain separate.
The owner page shows deadline, grace, B0, and Bmax with UTC and browser-local times.
An absent receipt does not prove rejection. With the default interval, uncertainty can approach 24 hours.
Actual hosts have no numeric E acceptance. Production switch use remains disabled.

The store uses the pinned bbolt v1.5.0 managed transactions.
Verified against the [bbolt v1.5.0 transaction source](https://github.com/etcd-io/bbolt/blob/v1.5.0/db.go) and bucket source, retrieved 2026-11-05.
The owner page uses the existing bounded Fetch and CSRF interfaces.
