# Switch runtime contracts

Issue #31 introduces runtime contracts for the reviewed version-two protocol.
These contracts do not enable production switches.
The CLI provides local prepare, arm, check-in, status, export, and recover commands.
The main API retains authenticated public arm evidence.
The agent runtime supports explicit local experiments over HTTPS.
Actual-host readiness and complete product verification remain incomplete.
The local runtime slice provides content, agent transitions, HTTPS transport, and owner orchestration APIs.

## Canonical wire data

`internal/switchwire` preserves the reviewed version-two frame and certificate domains.
`EncodeFrame` and `DecodeFrame` enforce canonical field order and bounded lengths.
`SignFrame` signs the complete canonical body with Ed25519.
`ReadEnvelope` accepts one length-prefixed body and signature and rejects trailing bytes.
The maximum body size is 16,384 bytes.
The maximum certificate size is 8,192 bytes.

`AdmitCertificate` verifies the owner signature and exact canonical certificate bytes.
It rejects duplicate agent identities, signature keys, and canonical X25519 recipients.
Recovery endpoints require HTTPS without credentials or fragments.
The resulting enrollment pins immutable content, key, grace, and agent identities.
Changing an exported enrollment field invalidates admission.

`VerifyOwnerRequest` verifies the signature against the enrolled owner key.
It binds the request to the admitted content, key, grace, and enrollment digest.
Prepare entries bind each recipient fingerprint to the configured agent.
Arm entries contain three signed prepared receipts for the identical state.
`Confirm` requires a matching durable receipt from each distinct enrolled agent at the requested phase.
It rejects confirmation when a receipt records possible disclosure.
The orchestration must request armed receipts before it reports confirmed arm.

`VerifyEnvelope` supplies low-level signature verification with a caller-supplied key map.
It does not replace enrollment admission or operation-specific authorization.
The transport and agent transition adapters must enforce lifecycle, sequence, freshness, and local clock cutoffs.
They must validate product interval limits through the certificate.
Wire verification alone does not prove durable agent storage.

## Independent authority

`internal/agent.Initialize` creates an empty authority on an explicitly pinned private volume.
`Open` requires the configured volume identity and an intact committed journal.
It obtains an exclusive process lock before replay.
The disposable checkpoint directory must remain outside the authority directory.
Deployment must keep the authority outside ordinary archive restores.
Directory separation alone does not prove independent storage or failure domains.

`Journal.Commit` appends complete state with an increasing generation and a hash chain.
The record contains exact accepted bytes, enrollment, ciphertext, manifest, encrypted share, receipt, and release envelope.
The runtime also retains the original signed prepare request after arm.
This request binds the retained share to the original owner intent during retries.
The journal persists the body before the final commit marker.
It persists the committed marker before it updates and syncs the checkpoint.
The caller receives the committed result after that order completes.
The journal permanently retains possible-disclosure and unsafe-clock latches.
The per-record byte limit rejects an oversized successor before journal mutation.
This refusal preserves the accepted journal, material, and latches across restart.
An unexpected append failure can leave uncertain persistence and quarantine the journal.
The runtime reserves physical publication capacity on Linux ext4 volumes before it accepts new state.
Other filesystems and platforms fail closed for this claim.
`MaxJournalBytes` defaults to 256 MiB. Its supported maximum is 1 GiB.
`MaxJournalRecords` defaults to 4096. Its supported maximum is 65536.
These limits bound the complete replay and authentic-prefix checks.
Admission preserves two maximum-sized records for the release and unsafe-clock latches.
An intent also reserves its receipt record before the accepted deadline changes.
The transition adapter preserves record space for receipts and the release envelope.

The journal uses `FALLOC_FL_KEEP_SIZE` before acceptance.
The runtime verifies allocated blocks and unchanged logical journal length.
Each checkpoint volume retains two private, preallocated inodes.
The runtime uses `RENAME_EXCHANGE` to replace checkpoints without discarding their reserved blocks.
Whitespace padding preserves strict JSON reads without truncation.
Restart validates or reserves these allocations before readiness.
An intact reservation needs no new allocation for release or restart.
A legacy journal keeps its authenticated append format and accepted authority during this validation.
A reservation failure prevents readiness and preserves the journal.
Checkpoint directory creation synchronizes every parent entry before readiness.

`LabOnlySyntheticStorage` explicitly disables physical and atomic-exchange claims for local tests.
Service startup requires the explicit synthetic lab clock with this option.
Public responses identify this mode with `X-Epochalypse-Storage: lab-only-synthetic-unverified`.
The verified clock mode rejects synthetic storage.
Two admitted HTTP requests bound concurrent material copies and output buffers.
Public output releases the transition lock before a potentially slow network write.
Actual-host publication latency and production readiness remain separate gates.

Verified against: [Linux fallocate](https://man7.org/linux/man-pages/man2/fallocate.2.html)
and [Linux rename](https://man7.org/linux/man-pages/man2/rename.2.html), retrieved 2026-11-05.
The implementation uses the pinned `golang.org/x/sys` v0.49.0 syscall signatures.
It rejects attempts to clear either latch.

Replay validates the full committed prefix before the caller can obtain state.
An incomplete suffix quarantines the journal.
A missing checkpoint can reconstruct from the valid head.
An older checkpoint must match an exact committed prefix before reconstruction from the current head.
Reconstruction retains the current authority and all latches.
A corrupt, divergent, or newer checkpoint quarantines the journal.
Common rollback of the entire authority remains outside the intact-state premise.

This storage API retains opaque protocol material.
The transition adapter must authenticate that material and enforce one immutable enrollment before each commit.
It must validate freshness before a new clock assessment and before any receipt or share.
The journal does not provide protocol semantics.
The transition adapter supplies prepare, arm, and signed check-in endpoints.
The release scheduler and public recovery endpoints use the same serialized authority transition as check-in.

`POST /agent/v1/switches/{id}/checkins` uses the existing signed envelope and HTTPS identity pins.
The agent verifies the enrolled owner, immutable content, increasing sequence, and nondecreasing signed deadline.
An exact retry preserves the accepted request and receipt bytes.
One fresh clock assessment checks both the prior release boundary and the proposed release boundary.
Arrival time never extends either signed deadline.
The agent persists accepted intent before it signs and persists a receipt.
Restart validates renewed material against the retained original prepare request and the complete authority journal.
Possible disclosure and unsafe clocks prevent renewal.
The owner-local client journal and CLI support direct renewal.
The main API and owner page reconcile authenticated public renewal evidence.

## Independent public recovery

`Service.RunPublisher` samples fresh clock evidence at a five-second interval.
The agent executable starts this scheduler with its HTTPS server.
Publication requires the conservative UTC bound to reach the accepted deadline plus grace.
The scheduler never projects release authority from elapsed time.
The agent synchronizes the release envelope and irreversible disclosure latch before public output.
Restart authenticates the retained release envelope against the current accepted state and enrolled agent key.
The released content key cannot renew.

`GET /agent/v1/public/switches/{id}` returns a version-one JSON public manifest.
The manifest contains the canonical enrollment certificate, owner signature, accepted signed request, and optional signed release envelope.
`GET /agent/v1/public/switches/{id}/ciphertext` returns the complete authenticated ciphertext object.
These routes require no owner authentication, main service, main DNS, or other agent.
Each agent retains its own complete ciphertext copy.

```sh
epochalypse switch recover --bundle recovery.json \
  --owner-public-key owner-public.key --pins tls-pins.json > recovered-content
```

The recovery command uses public keys and public data only.
It queries the three enrolled HTTPS endpoints concurrently with pinned TLS identities.
Each request has a five-second timeout. The complete attempt has a twenty-second timeout.
The client bounds manifests to 64 KiB and ciphertext to 11 MiB per agent.
It verifies enrollment, accepted owner intent, release signatures, distinct agent identities, and content bindings.
It reconstructs the key from two shares through the adopted content package.
It verifies authenticated ciphertext before stdout receives plaintext.
A failed recovery returns no plaintext.

The HTTPS tests recover through two independent journals while the third endpoint returns an outage.
Negative tests reject invalid manifests, signatures, shares, and ciphertext without plaintext output.
The executable test starts two agents from durable released state and runs the standalone recovery command.
The main state and the third agent remain unavailable. A single reachable agent cannot recover the content.
Controlled clock evidence creates the test release state. This test establishes no actual-host timing bound.
These software tests establish no actual-host timing or production acceptance.

## Owner-local renewal

`switchclient.InitializeRenewal` retains the exact arm request and three matching durable receipts.
The CLI calls it after confirmed arm. Its journal directory is `<bundle>.checkins`.
The directory contains canonical signed envelopes. It contains no second protocol encoding.
Each sequence has one immutable `<sequence>.request` file.
Each received agent receipt has one immutable `<sequence>.<agent-id>.receipt` file.
The owner keeps this directory outside main-server backups.

`switchclient.Renew` locks the journal against concurrent owner processes.
It signs the next sequence only after it authenticates the retained chain.
It derives the deadline from the issue time plus the enrolled interval.
The signed state also binds the grace and release boundary.
The client synchronizes the exact request and its directory before any agent request.
Before a successor, it synchronizes the preceding confirmed request and every receipt again.
This order handles visible bytes after a failed file or directory synchronization.

An incomplete renewal retains the original signed bytes after process restart.
A retry preserves its sequence, deadline, and proposed release boundary.
The client validates each receipt against the enrolled agent and identical signed state.
Only three matching durable receipts confirm renewal.
Missing or invalid responses preserve an uncertain outcome.
A partial-transmission error returns the retained bounds and each agent's evidence.
It does not discard that evidence with the error.

Run a direct check-in with a locally held owner key:

```sh
epochalypse switch checkin --plan private-plan.json --owner-key owner.key \
  --journal recovery.json.checkins --pins agent-pins.json
```

Read authenticated local status without a main-server request:

```sh
epochalypse switch status --plan private-plan.json --owner-key owner.key \
  --journal recovery.json.checkins
```

The JSON reports `confirmed`, `b0`, `bmax`, `deadline`, `grace_seconds`, and `allowance_assumptions`.
The `agents` array reports each agent's durable receipt or uncertain response independently.
B0 remains the earliest possible disclosure boundary after receipt loss.
Bmax includes the retained signed intent even when its receipts remain incomplete.
The allowance assumptions distinguish the candidate E from actual-host acceptance.
The main service holds no owner private key. A scoped API token cannot replace an owner signature.

Verified interface: [x/sys v0.49.0 Flock](https://pkg.go.dev/golang.org/x/sys@v0.49.0/unix#Flock), retrieved 2026-11-05.
The implementation also uses the pinned Go filesystem APIs for bounded files and directory persistence.

## Clock assessment

`internal/agent.AssessClock` validates one externally assessed UTC sample.
It uses the candidate profile from ADR 0013:

- Absolute UTC error: two seconds.
- Maximum sample gap: five seconds.
- Remaining publication and recovery work: 25 seconds.
- Conditional allowance: 34 seconds.

The assessment records the implementation, provenance, reference correctness evidence, source independence evidence, and leap policy.
The caller supplies elapsed readings from the same validated epoch.
The maximum assessment age at capture is 60 seconds.
The maximum capture delay is 250 milliseconds.
Base uncertainty must not exceed 500 milliseconds.
Residual rate uncertainty must not exceed 100 parts per million.
The adapter also accounts for integer quantization within the two-second ceiling.

The resulting lower bound is the sampled UTC second minus two seconds.
Use that result for this decision only.
An elapsed duration never projects release authority into a later decision.

Validate durable authority and irreversible latches before the clock assessment.
After suspend, a clock step, source loss, or an epoch change, obtain new assessment evidence.
Missing or inconsistent evidence blocks readiness.
The function checks evidence structure and candidate limits.
It does not establish the truth of external evidence.
The operator profile states verified reference correctness and source independence premises.
The Linux producer checks short capture and age intervals within a boot and process epoch.

The tests use explicit synthetic evidence.
They do not certify this host or establish actual publication and recovery performance.
Production readiness still requires actual-host evidence and independent failure domains.

Verified interface: [Go 1.27.3 time documentation](https://pkg.go.dev/time@go1.27.3), retrieved 2026-11-05.
The installed `go doc time.Time.Sub` also confirms bounded duration arithmetic.

## Agent runtime commands

`agent initialize` creates authority only on an existing empty private directory.
`agent serve` opens existing authority with the configured volume identity.
It never initializes a missing authority during startup.
The runtime validates retained enrollment, requests, ciphertext, shares, and receipts before the clock assessment.
A corrupt journal or irreversible latch blocks startup.
An intact committed transition can recreate its missing receipt through an exact retry.

Local experiments can select an explicit synthetic clock:

```sh
epochalypse agent initialize --config private-agent.json
epochalypse agent serve --config private-agent.json \
  --lab-only-synthetic-clock --http-addr 127.0.0.1:8443 \
  --tls-cert agent-cert.pem --tls-key agent-key.pem
```

The configuration must be a stable mode-0600 regular file.
Its JSON fields are `authority`, `agent_id`, `owner_key`, `certificate`, `certificate_signature`, `signing_key`, and `age_identity`.
Byte slices use JSON base64 strings.
The `authority` object uses `agent.Config` fields: `Directory`, `CheckpointDirectory`, `ExpectedVolumeID`, and optional `MaxRecordBytes`.
The operator supplies the enrolled owner key, signed certificate, local Ed25519 key, and enrolled age identity.
The TLS private key must also use an owner-only regular file.
The service requires TLS 1.3 and serves the existing prepare and arm handlers.
Stop it with SIGTERM or SIGINT before maintenance.

`NewLabClock` reads a fresh wall-clock sample for each local decision.
This explicit synthetic producer does not establish clock accuracy, source independence, or epoch continuity.
Actual-host readiness requires the separate clock evidence and deployment gates from ADR 0013.

`NewChronyClock` provides a read-only assessment adapter for the pinned chrony 4.9 interface.
It runs bounded `chronyc -v`, `chronyc -n tracking`, and `chronyc -n sources` commands.
The client version query does not identify the running daemon version.
The operator profile must separately attest the deployed daemon, reference correctness, and source independence.
The configured profile supplies explicit operator premises.
Each read obtains a fresh assessment and brackets acquisition with Linux capture evidence.
The adapter rejects an epoch change, stale evidence, excessive acquisition delay, or an unresolved leap state.
It adds the tracking error bound to the independent reference uncertainty before the 500-millisecond check.
It requires the pinned selected source, reachable source samples, and a fresh reference measurement.
`ServiceConfig.VerifiedClock` accepts this typed adapter instead of the explicit lab clock.
The optional `clock_profile` configuration selects this path without the lab flag.
The runtime requires exactly one clock mode.
The [readiness ruling](https://github.com/Crank-Git/epochalypse/issues/31#issuecomment-6667891433) defines the operator premises and later host gates.

Example profile fields use synthetic documentation values:

```json
{
  "executable": "/usr/bin/chronyc",
  "version": "4.9",
  "daemon_version": "4.9",
  "operator_verified": true,
  "provenance": "operator-pinned installation evidence",
  "sources": ["192.0.2.1", "198.51.100.2"],
  "reference_evidence": "operator reference correctness evidence",
  "independence_evidence": "operator source independence evidence",
  "leap_policy": "UTC normal only",
  "base_uncertainty_ns": 100000000,
  "rate_ppm": 100
}
```

Replace each premise with verified installation evidence before use.
The client version query does not verify the configured daemon version or source correctness.
The adapter rejects an absent or unverified profile and unsupported platforms.
`CLOCK_BOOTTIME` includes suspended time.
The producer compares it with `CLOCK_MONOTONIC` to detect an interrupted capture epoch.
The kernel boot identifier and a random process epoch scope each observation.
Capture errors supply no authority and never clear durable latches.
Fresh UTC acquisition supplies each decision; elapsed observations never project release authority.

Verified against [Linux clock_gettime](https://man7.org/linux/man-pages/man2/clock_gettime.2.html), retrieved 2026-11-05.
Verified against the [kernel boot identifier documentation](https://docs.kernel.org/admin-guide/sysctl/kernel.html#random), retrieved 2026-11-05.

Verified against the [chronyc 4.9 manual](https://chrony-project.org/doc/4.9/chronyc.html), retrieved 2026-11-05.
The [versioned client source](https://raw.githubusercontent.com/mlichvar/chrony/4.9/client.c) defines the version output and feature suffix.

## Dependencies

The production enrollment adapter uses `filippo.io/age` v1.3.2 to parse canonical X25519 recipients.
The module already pins this version for capsule dependencies.
The new direct import retains that exact version.
Its BSD-3-Clause notice is in `third_party/age/LICENSE`.

The selected package graph includes age, hpke, and the pinned Go cryptography packages.
It excludes `filippo.io/age/plugin`.
[GO-2024-3344](https://pkg.go.dev/vuln/GO-2024-3344) affects that plugin package before v1.2.1.
The selected version and graph exclude that finding.
This observation does not claim an exhaustive security audit.
The content adapter must complete the selected graph review before production use.

The content adapter imports `github.com/hashicorp/vault/shamir` at Vault v1.21.4.
The selected package uses only standard library dependencies.
The exact `go list -deps github.com/hashicorp/vault/shamir` graph contains 95 packages, including the selected package.
The imported graph excludes the Vault server and its KVv2 implementation.
[GO-2026-5487](https://pkg.go.dev/vuln/GO-2026-5487) reports a Vault server policy bypass.
The selected graph cannot reach that server behavior.
The advisory can still appear in a module-level report.
This package review does not assert module-wide security or an exhaustive security audit.
The package-local MPL-2.0 license applies to the selected Shamir source.
Vault's top-level application license differs.
The retained notice is `third_party/vault-shamir/LICENSE`.
Raw shares still require authenticated enrollment and content authentication.

Verified against the [age v1.3.2 API](https://pkg.go.dev/filippo.io/age@v1.3.2) and shipped license, retrieved 2026-11-05.
The versioned [Vault Shamir source](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/shamir.go) imports only standard library packages.
The [selected package API](https://pkg.go.dev/github.com/hashicorp/vault@v1.21.4/shamir) supplies `Split` and `Combine`.
The [package license](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/LICENSE) supplies the retained MPL-2.0 notice.
These references were retrieved on 2026-11-05.

## Local prepare and arm APIs

The owner stops the main server before the offline CLI opens the shared installation database.
The owner supplies the existing local Ed25519 private key as base64 text in a mode-0600 regular file.
The project does not generate or store a web-use signing key.
The operator separately enrolls that public key on each agent.
The operator supplies three distinct agent identities, signature keys, age recipients, and HTTPS endpoints.
The agent array uses the JSON fields from `switchwire.Agent`.
Each `SigningKey` contains 32 numeric bytes.

```sh
epochalypse switch create --state-dir var/state
epochalypse switch prepare --state-dir var/state --id DRAFT_ID \
  --content content.txt --agents agents.json --owner-key owner.key --plan private-plan.json
epochalypse switch arm --state-dir var/state --plan private-plan.json \
  --owner-key owner.key --pins tls-pins.json --bundle recovery.json
epochalypse switch status --state-dir var/state --id DRAFT_ID
epochalypse switch export --bundle recovery.json \
  --owner-public-key owner-public.key --output retained-recovery.json
```

`prepare` saves a protected private plan before any network request.
It refuses to replace a different existing plan.
The plan contains ciphertext and three encrypted shares. Keep it private.
`tls-pins.json` maps each enrolled HTTPS endpoint to its hexadecimal SHA-256 digest of the TLS public-key information.
The HTTPS client requires normal system trust validation and the configured pin.
It currently has no custom certificate-authority option.
`owner-public.key` contains the separately retained public key as base64 text.

`arm` persists exact local intent, obtains three prepared receipts, and exports the verified public recovery bundle.
It then persists the exact signed arm request beside the bundle with the `.arm` suffix.
Before the first arm send, it records that issued intent in the main database as an uncertain outcome.
After transmission, it records available authenticated armed receipts.
Only three matching durable receipts produce `state = "armed"` and exit code zero.
Repeat the same arm command with the same private plan and bundle path after an uncertain outcome.
The retry preserves the exact request and all previously retained receipts.
A failed update after transmission reports uncertainty.

The main `switches-v1` bucket retains the version-one configuration and optional `arm` evidence.
The evidence contains the public owner key, signed enrollment, exact signed arm request, and available armed receipts.
It contains no private key, plaintext, ciphertext, or encrypted share.
The shared installation quota charges each complete replacement record once.
The strict record decoder bounds each record to 64 KiB.
Older drafts keep their original six fields and bytes.
Backup restore uses the same decoder and signature validation.
Older six-bucket installations can omit the switch bucket.
Restore never installs live-agent authority.

`POST /api/v1/switches/{id}/arm` accepts `ArmEvidence` JSON with the `switch:write` scope or an owner session and CSRF proof.
This route records an owner-local attempt. It does not create signatures or transmit agent requests.
Its JSON fields are `owner_key`, `certificate`, `certificate_signature`, `request`, and `armed`.
The request contains three authenticated prepared receipts for the identical state.
The route verifies the draft identifier, configured durations, enrollment, and every supplied armed receipt.
It freezes the enrollment and signed request on the first observation.
Subsequent observations preserve all prior receipts.
It returns 202 for an uncertain outcome and 200 only for three matching armed receipts.
The request limit is 64 KiB. The route rejects unknown fields and trailing JSON values.
`GET /api/v1/switches/{id}` returns the same retained evidence.
The owner page distinguishes confirmed arm, uncertainty, and an absent observation.
An absent main observation does not prove that remote agents remain unarmed.
The main API cannot independently prove that the owner retains a local bundle file.
The owner-local arm implementation enforces the actual export order.

`switchcontent.Prepare` reads at most 10 MiB of plaintext.
It creates one random AES-256 key and a separate random key identity.
Its versioned ciphertext authenticates the key identity with AES-GCM.
The public frame contains no plaintext hash.
The complete frame adds 65 bytes to the plaintext length.
The content digest covers the complete ciphertext frame.
Vault Shamir splits the key into three raw shares with a threshold of two.
age encrypts each share to a distinct canonical X25519 recipient.
`DecryptShare` accepts only one complete 33-byte share.
`Recover` checks authenticated ciphertext before it returns plaintext.
Wrong or corrupt shares cannot produce accepted plaintext.

`agent.NewTransitions` requires operator-pinned enrollment and local signing and age keys.
Its `Prepare` and `Arm` methods use the independent journal.
Its `Handler` exposes bounded JSON requests under `/agent/v1/switches/{id}/prepare` and `/arm`.
An injected clock assessment remains mandatory.
A missing clock producer blocks acceptance.
The local tests supply explicit synthetic evidence.
These tests establish no actual-host readiness or numeric release guarantee.

`switchclient.NewPlan` consumes the existing version-one draft and encrypted material.
It calculates one deadline from the issue time and interval.
It signs one immutable prepare request with the local owner key.
`PrepareAndArm` persists exact owner intent before transmission.
It requires three matching prepared receipts.
It exports, syncs, reopens, and validates the recovery bundle before arm transmission.
The public bundle omits private prepare bytes and encrypted shares.
Before arm, the public manifest and ciphertext routes return HTTP 404.
After release, startup validates the durable release before it serves recovery material.
A durable release requires no new clock assessment. The agent still refuses renewal of that content key.
It retains enrollment, ciphertext, the exact state, and three prepared receipts.
The owner pins the public key when `ReadBundle` validates the bundle.
An arm retry uses the existing bundle and identical signed arm bytes.
Only three matching armed receipts set `ArmResult.Confirmed`.
An incomplete response reports uncertainty and retains each response.
Partial arm can disclose content while too few armed agents prevent recovery.

`HTTPSReplica` requires operator-pinned endpoint public-key hashes and normal TLS certificate validation.
It enforces TLS 1.3, a five-second timeout, a bounded response, and no redirects.
Agent signatures still provide protocol authentication.
The private owner key never enters the agent request or bundle.
The owner-local CLI records public evidence in the main draft as described above.
Actual-host deployment and the complete product verification gates remain incomplete.
