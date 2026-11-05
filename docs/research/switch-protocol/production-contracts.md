# Switch contracts for independent review

Issue #107 supplies a concrete producer for #28 under ADR 0012.
This document proposes algorithm contracts. It records no protocol acceptance or production implementation.
Production switches remain disabled.
The historical reports and their counterexamples retain their original meaning.

## Scope and evidence

The isolated module at `tests/compatibility/switch-contract` implements bounded frames and standard Ed25519 verification.
It tests share-library compatibility and synthetic lifecycle predicates.
It changes no production dependency, route, store, or model in `internal/switchprotocol`.
Its lifecycle snapshots represent complete values. They do not prove disk durability or real process recovery.
Issue #106 supplies the separate clock algorithm and candidate numeric profile.

The accepted threshold remains three matching durable receipts for confirmation and two distinct shares for recovery.
Two colluding agents can disclose content early. A compromised preparation client can disclose the complete key.
One unavailable agent can block confirmation. It cannot block release by two intact, honest, reachable agents under the reviewed clock assumptions.
Main-server loss and permanent owner-client loss require no further coordination for release.

## Verified library candidates

Primary sources below were retrieved on 2026-11-05.

| Candidate | Exact compatibility result | Limit |
| --- | --- | --- |
| `github.com/hashicorp/vault/shamir`, Vault `v1.21.4` | `Split(secret []byte, parts, threshold int) ([][]byte, error)` and `Combine(parts [][]byte) ([]byte, error)` recover a 32-byte key from each pair. | The package accepts raw bytes. It provides no share authentication or verifiable distribution. |
| `github.com/drand/kyber`, `v1.3.3` | The [versioned source](https://github.com/drand/kyber/blob/v1.3.3/share/poly.go) exposes scalar shares and `RecoverSecret`. | The #28 report verifies its signatures. This issue does not execute it or select a scalar-to-key representation. |
| `filippo.io/age`, `v1.3.2` | `GenerateX25519Identity`, `Recipient`, `Encrypt`, and `Decrypt` isolate each encrypted share to its recipient. | This research pins the version already present indirectly in the root module. It adds no production adoption. |

The [Vault source](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/shamir.go) sets a maximum of 255 parts and a minimum threshold of two.
Each share adds one byte. Empty secrets, duplicate coordinates, unequal lengths, and fewer than two shares return errors.
`Combine` does not know the original threshold or reject every corrupt share.
The fixture checks authenticated ciphertext before it exposes plaintext after reconstruction.
The package uses cryptographic randomness for polynomial coefficients. Coordinate selection uses `math/rand`; coordinates do not supply secret entropy.

The [package license](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/LICENSE) states MPL-2.0.
Vault's top-level license differs. This package license does not authorize unrelated Vault code.
The [release](https://github.com/hashicorp/vault/releases/tag/v1.21.4) dates from 2026-03-05, commit `ffe7023c481dc1ea2d8550bbaca8d85f8e611e0b`.
The [repository metadata](https://api.github.com/repos/hashicorp/vault) reports an unarchived repository with activity on 2026-11-05.
Its latest reported release is v2.2.1. The compatibility module deliberately pins the verified v1 release.
This maintenance evidence is not a security audit or a claim that v1.21.4 is the latest release.
Future adoption requires a current security and license review of the actual imported package graph.

The [age API](https://pkg.go.dev/filippo.io/age@v1.3.2) documents streaming encryption and decryption.
The [age license](https://github.com/FiloSottile/age/blob/v1.3.2/LICENSE) states BSD-3-Clause.
The fixture closes the encryption writer before it uses the ciphertext.
It reads the complete, bounded decrypted share before acceptance.
Each agent receives only its own recipient identity. Enrollment pins its public recipient separately from its signature key.

The [Go 1.27.3 Ed25519 API](https://pkg.go.dev/crypto/ed25519@go1.27.3) supplies `Sign` and `Verify`.
The verifier checks public-key and signature lengths before the call.
All committed test keys and content are synthetic.

## Version two research frames

The historical `research-v1` proposal in `rules.md` remains unchanged.
This isolated revision uses `epochalypse/switch/research-v2/`, an ASCII operation, and one zero byte.
It requires protocol version 2. Unknown domains, operations, versions, flags, and trailing bytes fail closed.
The decoder bounds the body before field allocation or signature verification.

Every body contains these fields in this exact order:

| Field | Representation |
| --- | --- |
| Switch ID | Four-byte unsigned length, then 1–128 ASCII letters, digits, hyphens, or underscores. |
| Protocol version | Four-byte unsigned integer. |
| Sequence | Eight-byte unsigned integer, greater than zero. |
| Deadline | Eight-byte signed two's-complement UTC seconds, nonnegative. |
| Grace | Eight-byte unsigned seconds. Deadline plus grace must fit a signed 64-bit integer. |
| Content digest | Exactly 32 bytes; SHA-256 of the complete authenticated ciphertext object. |
| Key identity | Exactly 32 bytes; unique random identifier for this prepared content key, rather than a hash of its secret bytes. |
| Enrollment digest | Exactly 32 bytes; SHA-256 of the canonical enrollment certificate described below. |
| Signer identity | The same length and character rules as the switch ID. The owner uses `owner`. |
| Nonce | Exactly 32 bytes. |
| Durable sequence | Eight-byte unsigned integer. |
| Phase | One byte: 0 draft, 1 prepared, 2 armed, 3 renewal stopped. |
| Possible disclosure | One byte: 0 private, 1 possibly public. |
| Entry count | Four-byte unsigned count, at most three. |
| Each entry | Identity field, 32-byte public-key field, four-byte data length, then 1–4096 opaque bytes. |

All integers use most significant byte first. Entries use strictly increasing identity byte order.
The complete body contains at most 16,384 bytes. The signature contains exactly 64 bytes outside the body.
The transport envelope carries the body length, body, and signature without another signer override.
The complete envelope contains at most 16,452 bytes. Stream readers reject excess bytes before decoding.
The fixture verifies in-memory bounded bodies; no HTTP stream parser exists here.
The fixed check-in vector contains 231 bytes.
Its SHA-256 digest is `a258f5ccfb10673bc7a30c48e6b7643116a1173de11c9358f5ec66cadadd1497`.

| Operation | Signer | Entries and semantic requirement |
| --- | --- | --- |
| `prepare` | Owner | Three recipient-encrypted shares. Each entry identifies its enrolled agent and age recipient fingerprint. |
| `arm` | Owner | Three prepared receipt envelopes for the identical state and enrollment. |
| `checkin` | Owner | No entries. The signed deadline comes from the client issue time plus its configured interval. |
| `status-request` | Owner | No entries. A fresh random nonce binds the response to this request. |
| `status-response` | Agent | No entries. Echo the nonce and return the durable state and both lifecycle fields. |
| `receipt` | Agent | No entries. Durable sequence equals accepted sequence. The phase identifies prepare, arm, or stopped renewal. |
| `release` | Agent | One entry for the signer, containing its 33-byte share. Possible disclosure equals one. |
| `cancel` | Owner | No entries. Stop further renewal for this key; retain its release authority. |

The fixed 32-byte entry key field holds the signature key for arm receipt entries.
For prepare entries, it holds SHA-256 of the canonical age recipient string.
For release entries, it repeats the enrolled agent signature key.
An implementation must compare these fields to enrollment, rather than trust a supplied key.
The enrollment certificate starts with its domain and one zero byte.
Append the owner signature key, key identity, and content digest as three fixed 32-byte fields.
Append interval and grace as unsigned eight-byte integers, then protocol version as an unsigned four-byte integer.
Append a four-byte count of three, then three records in strictly increasing identity order.
Each record contains a length-prefixed identity, its fixed 32-byte signature key, its age recipient string, and its HTTPS recovery endpoint.
Use four-byte length prefixes for both strings. Limit the recipient string to 128 ASCII bytes and the endpoint to 2048 ASCII bytes.
Use exact configured endpoint bytes without URL normalization. Reject fragments, credentials, and non-HTTPS endpoints.
Limit the complete certificate to 8192 bytes before hashing or signature verification.
Its own domain is `epochalypse/switch/enrollment-v2/`.
The owner signs the certificate before preparation. Agents pin it through authenticated enrollment.
The certificate is immutable for one content key. Changes require a fresh switch and key.
The module treats its digest as a pinned input; it does not implement certificate or nested receipt decoding.

The frame fixtures check canonical syntax and the signed state association.
Semantic admission additionally checks the pinned enrollment, operation phase, exact requested state, and operation-specific certificate contents.
`confirm` tests three distinct authenticated receipt identities against the exact expected state.
Nested arm receipts must all use prepared phase. Successful arm needs three later armed receipts.
Prepare and arm certificate validation remain runtime obligations; opaque entry bytes are not proof that these checks execute.

Admission enforces intervals from 3,600 through 31,536,000 seconds and grace from zero through 2,592,000 seconds.
The owner producer calculates the absolute deadline once, signs it, and journals it before transmission.
Retries never derive a deadline from arrival time.
The synthetic byte fixtures use shorter times to isolate representation and authorization checks.
The future adapter must test product time limits and the clock cutoff from #106.
Authentication alone does not authorize a late request.

The runtime pins identity-to-key associations outside the incoming message.
It verifies signatures locally even over authenticated HTTPS.
Private prepare bytes and share plaintext never enter public logs, status responses, or recovery exports.
Public exports contain the ciphertext, manifest, enrollment, public receipts, and recovery endpoints.
The owner distributes recipient-encrypted share bodies over HTTPS with pinned endpoint identities.
No agent obtains another agent's decryption identity.
At least two independent agents store independently retrievable ciphertext and the manifest before confirmed arm.
All three prepared receipts attest to their own complete durable copies.

## Client journal and exact retries

Before transmission, persist the complete signed envelope and its decoded state in one journal record.
Also persist the pinned enrollment, confirmed receipts, B0, and the complete issued-intent list.
Reopen that journal without transmission enabled.
Verify every signature and derive each state from those same body bytes.
Compare the derived state with the journal metadata and the requested switch configuration.
Any mismatch quarantines the journal before retry or receipt reconciliation.
The fixture changes each state field independently to check this association.

An increasing sequence identifies one immutable request. Same-sequence different bytes represent a conflict.
An exact retry can return the original durable receipt only while the clock and lifecycle still permit it.
It creates no new acceptance or later deadline. Old sequences never regain authority.
Before confirmation, persist three matching receipt envelopes for the exact issued request and enrollment.
Duplicate identities count once. Receipts for different intents never combine.
B0 advances only after this commit. Bmax includes every durable issued intent, including one not yet sent.
Lost final responses retain the uncertain outcome from B0 through Bmax, with E separate.

## Durable authority outside restored archives

Each agent has its own authoritative append-only journal on an operator-controlled durable volume.
That volume stays outside the backup and restore set for the agent's ordinary data archive.
It needs no external service, shared witness, main server, owner client, or inter-agent connection for release.
The deployment pins its volume identity and enrollment through configuration outside the restored archive.
An empty replacement volume cannot automatically enroll as the previous authority.

This journal stores complete committed state, rather than a counter alone.
Each record contains generation, previous-record SHA-256, exact accepted request, enrollment, ciphertext identity, phase, and irreversible latches.
The latches record possible disclosure and unsafe clock evidence.
The journal also retains the original receipt bytes and any committed release envelope.
Frame length, checksum, and a final commit marker distinguish complete records from interrupted writes.
These checks detect corruption; they do not authenticate a malicious operator or establish freshness after whole-volume rollback.

The trusted head comes from the intact journal, never from a pointer inside the archive being restored.
At startup, scan the journal and validate its complete committed prefix before any output becomes available.
Compare the archive checkpoint with that head, including generation, accepted bytes, enrollment, phase, and both latches.
Missing, corrupt, inconsistent, partial, or older evidence quarantines the process before any receipt or share.
A larger counter without exact state equality does not establish freshness.
The synthetic `restore` predicate tests complete head equality, not a storage implementation or tail-deletion detector.

An ordinary restart with intact committed state follows this path:

1. Read the intact authoritative journal.
2. Validate the latest complete committed state.
3. Reconstruct a disposable local checkpoint from that exact state if needed.
4. Persist and validate that checkpoint.
5. Retain both irreversible latches.
6. Obtain the fresh clock assessment required by #106.
7. Enable operations only after both validations succeed.

An interrupted trailing record keeps the process quarantined during repair.
Repair may discard only a proven uncommitted suffix after the intact committed prefix.
An incomplete record never supplies authority or permits output.
If the committed prefix cannot be established, require operator recovery; do not fall back to an archive.
If a valid head records possible disclosure, retain that fact even when publication has no successful response.
Quarantine does not remove shares that observers already retain.

Routine process restart does not become an excluded fault.
The runtime must complete intact-state replay, checkpoint repair, and a fresh assessment within the publication bound P.
Ordinary host restart also requires #106's fresh elapsed origin or sample-only policy.
The authoritative journal remains accessible after either restart.
Permanent loss, corruption, or rollback of that authority violates the intact-durable-state premise.
Common rollback of an entire VM and its authority can reproduce a valid old history without detection.
This design makes no claim against that fault. A second file on the same restored snapshot cannot solve it.

Each surviving agent trusts its own intact authority and enrolled keys.
An unavailable authority makes that agent unavailable for the conditional bound.
The one-outage claim therefore requires two surviving agents with accessible intact authorities and fresh clock assessments.
An implementation must demonstrate that one planned agent outage does not also remove another agent's authority.
Separate providers, credentials, storage, and restore procedures remain deployment review obligations.
No new shared network authority silently reduces this fault budget.

## Persistence before output

One serialized operation order covers renewal, lifecycle changes, release, and recovery.
The runtime blocks concurrent disclosure while it commits accepted state.
For every transition, use this order:

1. Verify the bounded signed body against pinned enrollment.
2. Validate freshness, lifecycle, sequence, and the current clock assessment.
3. Construct the complete successor state and any output envelope.
4. Append the full successor record to the authoritative journal.
5. Persist its data and commit marker to durable storage.
6. Update and persist the disposable checkpoint.
7. Publish the already committed receipt or share envelope.

The durable release record sets possible disclosure before any share leaves the process.
A failed or lost output response never clears that latch.
If a clock assessment fails, commit the unsafe latch before exposing another protocol result.
Neither restart nor restore clears an irreversible latch.
Recovery of an unsafe clock requires the separately reviewed policy, not an automatic Boolean reset.

| Crash point | Authoritative result before restart output |
| --- | --- |
| Before a complete durable commit | Keep the previous committed state. Quarantine any incomplete suffix during repair. Emit no successor receipt or share. |
| After commit, before checkpoint | Replay the full committed successor, then validate the checkpoint and fresh clock assessment. |
| After checkpoint, before response | Retain the successor. Exact retry returns existing evidence when eligible; a lost response remains uncertain to the client. |
| After release commit, before any response | Retain possible disclosure. Retry publication when eligible. Refuse renewal of the same key. |
| After public disclosure | Retain possible disclosure and all observer copies, independently of endpoint visibility. |

Real implementation tests must kill and restart processes at each point.
They must verify flush errors, filesystem guarantees, directory persistence, locking, and bounded recovery time.
The current fixtures assert logical order through a `durable` input. They establish none of those operating-system guarantees.

## Complete lifecycle contract

Draft preparation creates one random content key and three shares in the owner client.
Each agent decrypts only its own encrypted share and verifies the immutable enrollment.
It stores the ciphertext, manifest, share, and prepared state before its prepared receipt.
Prepared agents never release. Three matching prepared receipts permit an owner-signed arm certificate.
The owner exports and verifies the recovery bundle before it transmits arm.

Each agent validates the complete arm certificate and commits armed state before its armed receipt.
Three matching armed receipts confirm successful arm.
Partial arm or lost arm responses report an uncertain outcome; they never report confirmed arm.
An agent that accepts arm can later release independently, even after permanent owner-client loss.
Incomplete arm does not establish the two-agent availability guarantee.
The client must show that partial arm can disclose content while too few armed agents can prevent recovery.
It must not label partial arm as a harmless unarmed draft.

After confirmed arm, renewal follows B0, Bmax, exact retries, and the cutoff reviewed in #106.
Each agent serializes renewal with release. Any possible disclosure permanently blocks renewal for that key.
Only three matching durable receipts confirm renewal; two public shares reconstruct the key.
Released-share count follows key and agent identity across sequences, restart, and later histories.

Cancellation stops future renewal attempts. A signed cancel can durably freeze renewal at each reached agent.
It does not destroy accepted release authority, ciphertext, or shares.
Three matching stopped-renewal receipts confirm that agents reject further renewals; fewer receipts report uncertain cancellation.
In-flight requests can arrive before cancel in the serialized order.
The client retains their issued boundaries in Bmax.
An unreached agent can still accept such a request under the usual cutoff.
Every armed or stopped-renewal agent still releases at its accepted boundary under the clock policy.
Cancellation cannot promise secrecy, successful recall, or prevention of release.
After possible disclosure, cancel returns irreversible-release status instead of a successful recall claim.

Replacement creates a fresh switch ID, enrollment, key, ciphertext, and recovery bundle.
It completes the same preparation and arm protocol with three agents.
It never substitutes a fresh identity into the old key's threshold slots.
Old agents and exported bundles retain the old independent release authority.
The old content can still become public, even after new content reaches confirmed arm.
If replacement fails partway, retain the old switch and report the new switch's partial-arm uncertainty.
Decommission of old agents cannot claim the one-outage guarantee for the old switch after it removes required survivors.
Replacement of the same key or recall of an old export remains unsupported.

The synthetic lifecycle tests cover local prepare, arm, cancellation, restart, and possible disclosure.
They reject same-key replacement and renewal after possible disclosure.
The receipt tests enumerate all eight subsets of the three enrolled identities.
The #27 observer model supplies the separate cumulative public-share schedules.
These combined records preserve B0 through Bmax and the reviewed E; they do not prove a distributed runtime.

## Producer handoff to #28

The producer of clock readiness must first consume validated authoritative state and irreversible latches from this contract.
Only then may it supply the fresh assessment to #106's clock algorithm.
A stale snapshot must not supply an old accepted deadline or clear a release latch before clock re-anchor.
The clock algorithm cannot establish archive freshness from `Safe` or a UTC sample.
Review #106's candidate profile separately from these storage and byte assumptions.

The reviewer can consume the version-two byte vector, malformed-input fixtures, library matrix, freshness predicate, and lifecycle tables independently.
The unresolved runtime obligations are explicit: certificate semantics, product time admission, journal implementation, authenticated transport, and real restart tests.
The final maintainer decision must distinguish algorithm acceptance from implementation and deployment verification.
No test count establishes universal correctness, a production E, or production enablement.

## Reproduce the research checks

Run from the worktree root with Go 1.27.3:

```sh
mkdir -p var/cache/go-build var/cache/go-mod var/tmp
export GOCACHE="$PWD/var/cache/go-build"
export GOMODCACHE="$PWD/var/cache/go-mod"
export GOPATH="$PWD/var/gopath"
export GOTMPDIR="$PWD/var/tmp"
go -C tests/compatibility/switch-contract mod download
go -C tests/compatibility/switch-contract test -count=1 -race -coverprofile="$PWD/var/switch-contract.cover" ./...
go -C tests/compatibility/switch-contract vet ./...
go -C tests/compatibility/switch-contract build ./...
git diff --check
```

Run the repository's configured `golangci-lint` from the nested module with the same local cache paths.
Use `--config ../../../.golangci.yml ./...` and a worktree-local `GOLANGCI_LINT_CACHE`.
The 2026-11-05 check reports passing race tests, vet, build, and zero lint issues.
First-party compatibility helper coverage is 99.4 percent.
The phase-conflict test also fails before the armed-phase check and passes after it.

The module has no network requirement after its pinned dependencies enter the cache.
The initial behavioral tests fail against the committed permissive stubs in `fca53bf`.
Library compatibility tests establish external API behavior; they do not need a previous project implementation to fail.
Nested-module coverage measures only first-party compatibility helpers.
It does not replace the aggregate first-party coverage gate of at least 95.00 percent.
The epic integration pull request runs the full native product suites and combined coverage gate.
No browser or live infrastructure check applies to this isolated research producer.
