# Switch runtime contracts

Issue #31 introduces runtime contracts for the reviewed version-two protocol.
These contracts do not enable production switches.
The prepare, export, and arm commands remain incomplete until the orchestration slice passes its checks.

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
The journal persists the body before the final commit marker.
It persists the committed marker before it updates and syncs the checkpoint.
The caller receives the committed result after that order completes.
The journal permanently retains possible-disclosure and unsafe-clock latches.
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
The journal does not provide a prepare endpoint, receipt signer, release scheduler, or public recovery endpoint.

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
An evidence producer must assess reference correctness and elapsed epoch continuity independently.

The tests use explicit synthetic evidence.
They do not certify this host or establish actual publication and recovery performance.
Production readiness still requires actual-host evidence and independent failure domains.

Verified interface: [Go 1.27.3 time documentation](https://pkg.go.dev/time@go1.27.3), retrieved 2026-11-05.
The installed `go doc time.Time.Sub` also confirms bounded duration arithmetic.

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

The PM selects Vault v1.21.4 for the later raw-byte Shamir adapter.
This checkpoint imports no Vault code.
[GO-2026-5487](https://pkg.go.dev/vuln/GO-2026-5487) reports a Vault server policy bypass.
The later adapter must verify applicability against its actual import graph.
Raw shares still require authenticated enrollment and content authentication.

Verified against the [age v1.3.2 API](https://pkg.go.dev/filippo.io/age@v1.3.2) and shipped license, retrieved 2026-11-05.
The versioned [Vault Shamir source](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/shamir.go) imports only standard library packages.
The actual production import and license review remain part of the content slice.
