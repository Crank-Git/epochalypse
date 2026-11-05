# Capsule foundation

The foundation provides public, server-held, and tlock HTTP interfaces, CLI commands, and owner controls.
The [tlock integration record](research/tlock-production.md) defines its current contract and remaining work.

## Content and trust

All three modes use the authenticated content object defined in [ADR 0008](adr/0008-capsule-authenticated-object.md).
Each draft receives a fresh identifier and 32-byte content key.
The record contains the object and its SHA-256 digest.
The private-key bucket stores the public or server-held key separately. Tlock retains only its preparation digest there.
Neither the manifest nor the record contains a key.

Public mode is public data after publication. Its key becomes available immediately at publication.
Its requested release time is advisory. It does not withhold content until that date.
Server-held mode trusts the server operator. The operator holds the key and can read the content.
A server outage or lost key can delay or prevent availability. A lost server-held key has no automatic recovery.

Web preparation passes plaintext through the server in v1. The web interface must state this fact before upload.
CLI tlock preparation occurs on the owner's machine. It uploads neither plaintext nor an unwrapped content key.

## Bounded preparation

`capsule.Input` supplies mode, content kind, title, filename, and requested release time.
Text content must be UTF-8. File content remains opaque bytes.
An empty filename becomes `capsule.txt` for text or `capsule.bin` for a file.
Titles permit 256 UTF-8 bytes. Filenames permit 255 UTF-8 bytes.
Both fields exclude control characters. Filenames also exclude path separators.

`Store.Create` reads at most the configured plaintext limit plus one byte before it rejects excessive content.
The default plaintext limit is 10 MiB. The extra byte detects an excessive upload without an unbounded read.
The content object has the independent encoded limits from ADR 0008.
The encoded record permits another 8192 bytes for bounded metadata and JSON structure.
Creation rejects past requested times. Stored times use UTC seconds.

The content reader's errors can contain private data. The public domain error excludes those source error strings.
Preparation exposes no plaintext through errors or returned records.

## State and lifetime

[ADR 0009](adr/0009-shared-installation-state.md) defines the shared layout, budget, and migration limits.
`capsule.Open` owns the shared installation handle for an offline command.
`capsule.Attach` shares an existing installation handle. It does not open another writer.
Closing an attached capsule store stops capsule access and preserves other domain handles.

Public and server-held creation commits the record and its private key in one transaction.
Admission charges one object, the exact encoded record bytes, and 32 private-state bytes.
Internal tlock creation stores its complete wrapper in the record. It stores a preparation digest instead of a private content key.
Publication verifies that digest against the immutable draft. It authenticates the JWE object during preparation, before it discards the key.
The digest detects draft changes. It does not authenticate a future wrapper independently of its preparation path.
A failed quota check retains neither a partial record nor an orphan key.
Publication replaces the draft in the same budget transaction.
If publication exceeds the byte allowance, the transaction restores the private draft.

Record reads validate the stored size and profile independently of the current upload limit.
Lowering the upload limit affects new preparation. It preserves access to larger retained capsules.
The version 1 record fixture preserves release metadata and the authenticated content profile.

## Producer contracts

| Operation | Result |
|---|---|
| `Create(input, reader, now)` | A private `Record` after bounded preparation and atomic persistence |
| `Get(id)` | An owner record, including an unpublished draft |
| `Published(id)` | A published record; drafts return `ErrNotFound` |
| `Publish(id, now)` | A published record after mode-specific preparation verification and atomic budget admission |
| `Key(id, now)` | A key after publication and the selected release condition |
| `Bundle(id, now)` | A typed recovery bundle after publication; an early server-held bundle excludes its key |
| `OpenObject(id, version, object, key, limit)` | Plaintext after complete authentication; no plaintext on failure |

Domain methods do not authenticate HTTP requests. Owner transport adapters must apply the existing authorization safeguards.
Use `Published` for anonymous object reads. Use `Get` only through an owner-authorized interface.
Record reads verify the object profile and stored digest. Consumers must use `OpenObject` before they expose plaintext.

`Manifest` contains version, identifier, mode, content kind, labels, size, digest, and these times:

| Field | Meaning |
|---|---|
| `created_at` | The accepted draft time |
| `requested_release_at` | The owner's requested time |
| `effective_release_at` | Server-held release time; public publication time; absent for a public-mode draft |
| `published_at` | The publication time; absent for a private draft |

For server-held mode, the effective time equals the requested time.
For public mode, publication sets the effective time to the publication time and preserves the advisory requested time.
Repeated publication preserves both stored times.

Before publication, `Key` returns `ErrNotFound`, including after the requested date.
Before the server-held release time, it returns `ErrTooEarly` without a key.
At the exact release time, it can return the authenticated key.
A missing or malformed private key returns `ErrUnavailable` after release becomes eligible.
A key that fails object authentication returns `ErrIntegrity` without a key.
These results let later interfaces distinguish release eligibility from content availability.

The later capsule slices consume this producer for public pages, downloads, and recovery bundle export.
They must preserve bounded authenticated object handling and the shared installation transaction.

### Recovery producer

`RecoveryBundle` version 1 contains a complete `Record` and an optional `released_key`.
The key uses canonical base64url without padding and decodes to exactly 32 bytes.
Tlock bundles always exclude this key. Their records retain the complete pinned `Timelock`.
Public bundles include their authenticated key after publication.
Server-held bundles include their authenticated key only at or after the effective release time.
`Store.Bundle` returns `ErrNotFound` for a draft. It reads the record and permitted key in one transaction.
It returns no bundle if a required released key is unavailable or fails authentication.

Serialize the typed bundle with `json.Marshal` for the version 1 canonical envelope.
`DecodeBundle(reader, limit)` bounds the input before JSON allocation.
Its allowance combines `EncodedObjectLimit(limit)`, 8192 metadata bytes, and 256 envelope bytes.
It rejects unknown fields, duplicate fields, noncanonical envelopes, unsupported versions, and excessive declared plaintext size.
Callers must pass an explicit content limit. A changed upload limit does not change retained-record access through `Store.Bundle`.

`RecoveryBundle.Open(ctx, source, now)` authenticates the complete JWE object before it returns plaintext.
Tlock recovery verifies the pinned beacon and authenticates the complete key wrapper first.
Recovery uses the stored content size rather than a current upload setting.
It requires no store access after bundle creation.
Before the effective release time, encrypted modes return `ErrTooEarly` without a beacon request.
Missing recovery material returns `ErrUnavailable`. Invalid content or recovery material returns `ErrIntegrity`.
Every failure returns no plaintext. Cancellation also returns no plaintext.
HTTP consumers must retain the four shared capsule slots and a bounded request context.

[ADR 0010](adr/0010-tlock-preparation-proof.md) records the preparation digest and its trust boundary.

## HTTP interfaces

Mutations and owner metadata reads accept an owner session or an exact `capsule:write` bearer scope.
Owner session mutations require the existing CSRF proof header and cookie.
Only a validated bearer token bypasses the cookie CSRF proof. An invalid bearer never falls back to an owner session.
All routes require TLS outside the explicit loopback lab.

| Request | Result |
|---|---|
| `POST /api/v1/capsules` | 201 with a private draft manifest |
| `GET /api/v1/capsules/{id}` | 200 with owner-authorized manifest metadata |
| `POST /api/v1/capsules/{id}/publish` | 200 with the published manifest |
| `GET /api/v1/public/capsules/{id}/key` | 200 with an authenticated released key; 425 before server-held release |

Creation uses `multipart/form-data`. Supply one `mode`, one `release_at`, and one `content` part.
The optional `title` part contains at most 256 UTF-8 bytes. The optional `kind` part selects `text` or `file`.
If `kind` is absent, a content filename selects opaque file content. Otherwise, the content is text.
The requested time uses RFC 3339 with whole seconds. The server stores it in UTC.
Part headers exclude transfer encodings and unknown header names. Duplicate or unknown parts fail validation.

The complete upload allowance is the plaintext limit plus 65536 bytes for multipart structure and metadata.
The parser also checks each field's own bound. It bounds unknown-length bodies and trailing multipart data before preparation starts.
Configure the plaintext allowance through `capsule_plaintext_limit` in the version 1 local JSON configuration.
An absent or zero configuration value selects the 10 MiB default. Invalid or overflowing limits prevent startup.

Publication accepts an empty body or one empty JSON object, with at most 1024 bytes.
GET requests accept no body. Capsule routes accept no query parameters.
Creation, owner metadata, and publication return manifests without content objects or keys.
The public key response contains `version`, `id`, and a 43-character unpadded base64url `key` for its 32 bytes.
It returns no key after an integrity, availability, or release error.
All responses retain the shared error shape, request identifier, `no-store` cache policy, and response safeguards.

The service shares four work slots across capsule preparation, metadata, publication, and key authentication.
If all slots are busy, a request returns 429 with `Retry-After: 1` before it reads content or authenticates an object.
Each admitted request has a ten-second body deadline and work context. Its response deadline permits five more seconds.
The process also bounds request headers, body reads, and response writes.
Cancellation before preparation retains no draft. A mutation that already commits remains durable after a client disconnects.

| Status | Meaning |
|---|---|
| 400 | Invalid metadata, content, identifier, or request shape |
| 401 or 403 | Missing credentials, wrong scope, or rejected owner CSRF proof |
| 404 | Missing capsule or unpublished public key |
| 408 | Canceled or timed-out request |
| 409 | Full shared installation budget |
| 413 | Excessive plaintext, metadata, or complete request bytes |
| 425 | Server-held key remains before its release time |
| 426 | TLS is required |
| 429 | All capsule work slots are busy |
| 503 | Unavailable state or failed integrity verification |

The public recovery bundle, page, attachment, and export interfaces remain with the later capsule slices.
The key-free `Record` is a ciphertext producer. It is not a self-contained recovery bundle.

## Owner controls

Open `/owner/capsules` from the owner dashboard after sign-in.
Review the content and trust disclosures before upload. Choose a supported mode, requested release time, and text or a file.
Select **Create draft**. Review the returned metadata, then select **Publish capsule**.
The controls show the configured plaintext limit. The server enforces the limit independently of browser checks.
The controls clear the content fields after successful preparation. They render metadata as text.
Owner sessions authorize both the page and script. Mutations use the shared CSRF proof and existing HTTP work bounds.
The tlock option uses the pinned quicknet round at or after the requested time.
Beacon failure can delay availability. Published ciphertext cannot be recalled.
These controls do not offer the later public page or recovery bundle.

## Local CLI

Stop the main server before a local capsule command. Host access authorizes these offline commands.
Preparation reads a regular file on the owner's machine. It sends no plaintext over a network.
Set the same installation limits that the server uses. Both commands retain the shared installation budget.

```sh
epochalypse capsule create --state-dir var/state --mode server-held \
  --kind text --content-file capsule.txt --title "Synthetic capsule" \
  --release-at 2099-01-01T00:00:00Z
epochalypse capsule publish --state-dir var/state --id <capsule-id>
```

Use `--kind file` for opaque bytes. Use `--plaintext-limit` to set the allowance for new content.
The default remains 10 MiB. Both commands return manifest JSON without content or keys.
Publication in public mode exposes its key immediately. The requested date remains advisory.
Server-held publication preserves the requested release boundary and operator trust.
Export, local open, and deletion remain with later slices.

Verified against Go 1.27.3 shipped documentation for `os.Open`, `flag.FlagSet.Int`, `time.Parse`, and `filepath.Base`.
The browser adapters use the documented [FormData interface](https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest_API/Using_FormData_Objects)
and [Fetch interface](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch), retrieved 2026-11-05 UTC.

## Local tlock draft import

Use `capsule create --mode tlock --prepared-output prepared.json` with the other creation flags.
The command creates a local draft and writes its canonical `Record` to a new owner-only file.
An existing output file causes failure. A failed file write can leave the local draft retained.
The output contains ciphertext and pinned wrapper metadata. It contains neither plaintext nor the unwrapped content key.
The command still returns manifest JSON on standard output.

Upload the exact file bytes to `POST /api/v1/capsules/prepared` with `Content-Type: application/json`.
The route requires owner authorization or the exact `capsule:write` bearer scope.
Owner sessions also require the existing CSRF proof. TLS, four shared work slots, and request deadlines apply.
The response is 201 with manifest metadata and a `Location` header.
Publish the imported draft through the existing publication route.

`ImportPrepared(ctx, reader, now)` bounds the encoded input before JSON allocation.
It accepts only the exact canonical tlock draft record. It rejects publication state, keys, unknown fields, and duplicate fields.
It validates the pinned identity, ceiling round, object profile, digest, labels, and configured plaintext allowance.
The requested release time must remain at or after import time. The creation time cannot exceed import time.
Duplicate identifiers fail without replacement. The shared transaction charges the full record and its separate 32-byte preparation digest.
The transaction checks cancellation at entry and after both writes. An observed cancellation rolls back both values.
Commit can complete if cancellation follows the final check. A disconnected client must inspect the identifier before it retries.

Import validates structure. It cannot authenticate future wrapper decryption or content without the discarded key.
The preparation digest detects later draft changes. It does not prove that an external producer prepares valid encrypted content.
Recovery verifies the beacon signature and authenticates both encrypted layers before it exposes plaintext.
ADR 0010 defines this trust boundary.

Verified against Go 1.27.3 shipped documentation for `os.OpenFile`, `io.LimitReader`, and `context.Context` (2026-11-05 UTC).
