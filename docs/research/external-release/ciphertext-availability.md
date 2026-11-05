# Define independent ciphertext availability

Date: 2026-11-05 UTC. Issue: #48. Scope: AC-research-3 and FR-research-3.
Status: comparative research. This report selects no storage service or dependency.

## Result

Recovery requires complete ciphertext outside the failed infrastructure, authenticated metadata, sufficient release material, and a compatible client.
A content address identifies bytes. It does not establish a retention period or a reachable copy.
Available ciphertext does not establish an available beacon or sufficient shares.
Available release material does not reconstruct missing ciphertext.

The [comparison](models.md) defines C1–C4 and F1–F6.
[ADR 0012](../../adr/0012-switch-uncertainty.md) accepts the bounded uncertainty direction. Production protocol review remains pending.
This report preserves B0, Bmax, and E. It supplies no numeric E or additional release guarantee.
Issue #47 owns execution and deadline enforcement. Issue #49 consumes the storage and recovery requirements below.

## Independent recovery requirements

These requirements describe a future evaluated mechanism. They define no production interface or serialization format.

| ID | Required evidence before an availability claim |
| --- | --- |
| A1: complete copies | A visitor retrieves the complete recovery bundle after every owner-operated host stops. A hash, index, or partial copy does not pass. |
| A2: separate failure domains | The record identifies each operator, provider, account, administrator, region, payment source, and network path. Shared dependencies count as shared failures. |
| A3: retention | Each copy has an explicit retention horizon and deletion policy. The horizon covers the possible release boundary and the chosen recovery period. |
| A4: independent access | Retrieval needs no owner session, surviving main server, owner signing key, or new owner payment. The record identifies any access credential and its lifetime. |
| A5: discovery | The visitor retains the manifest and exact object identifiers outside owner hosts. Independent locators survive loss of the main domain and its index. |
| A6: integrity | The client verifies the expected bytes and authenticated metadata. It rejects corrupt, truncated, substituted, and incompatible content before plaintext output. |
| A7: release inputs | The bundle identifies the release mechanism and its verification inputs. Independent paths supply a valid beacon or sufficient authenticated shares. |
| A8: recovery client | The visitor retains a compatible client and its verified source or executable outside owner hosts. Recovery requires no main-server API. |
| A9: continued evidence | Independent probes retrieve complete copies and verify them. A successful upload or metadata response alone does not establish continued recovery. |

For A3, the owner must choose a recovery period after release. No finite retention period establishes indefinite future recovery.
A renewable switch can exceed an initial retention horizon. Any extension needs independent durability evidence before the system claims continued availability.
An issued renewal can reach Bmax even when the client receives no receipt.
Retention based only on B0 can therefore expire before the actual release boundary.
E remains separate from the interval between B0 and Bmax.

For A2, three copies under one account do not establish three independent operators.
An operator's continued payment and administration remain dependencies even when its hosts differ from the owner's hosts.
The evaluation must name these dependencies. This report establishes no provider failure probability or service-level commitment.

For A5–A8, the recovery bundle needs the following information:

- Complete ciphertext bytes, or verified independent paths to every required part.
- The manifest version, content identity, expected length, and authenticated digest or equivalent authenticated identity.
- Exact immutable object versions or content identifiers, with alternate discovery paths.
- The selected release mechanism and all public verification inputs.
- For C1, the tlock version, quicknet identity, target round, and compatible ciphertext wrapper.
- For C2–C4, the reviewed share format, participant identities, and state verification rules when those contracts exist.
- A compatible client version, dependency records, and instructions that survive main-server loss.

The bundle must exclude unreleased private material that defeats the release mechanism.
A digest from the same untrusted download path does not authenticate the owner's intended content.
The client needs an independently trusted manifest or an established authenticated format.
Verification of a content identifier does not prove that a mutable discovery result represents the latest accepted switch state.

## Storage capability audit

All sources below are primary sources retrieved on 2026-11-05 UTC.
The storage documentation supplies dated snapshots without an implementation version. No storage client or API receives adoption approval.

| ID | Source and evaluated version | Verified capability | Limit for this comparison |
| --- | --- | --- | --- |
| S1 | [IPFS content identifiers](https://docs.ipfs.tech/concepts/content-addressing/), dated documentation snapshot; CID versions 0 and 1. | A CID describes content identity rather than storage location. A file digest can differ from its CID. | The identifier supplies no retention contract. Preserve the exact identifier and format. |
| S2 | [IPFS pins](https://docs.ipfs.tech/how-to/pin-files/), dated documentation snapshot; no Kubo release selected. | Pins protect local data from garbage collection. A recursive pin includes child blocks. Unpinned cache data can disappear. | A pin does not establish operator survival. Direct pins can omit required child blocks. Remote retention needs separate evidence. |
| S3 | [Amazon S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html), dated managed-service documentation snapshot. | Retention protects specified object versions. A new version or delete marker can coexist with a protected version. | Preserve the exact version identifier. Deletion protection alone establishes neither visitor access nor provider independence. |
| S4 | [Amazon S3 presigned URLs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-presigned-url.html), dated documentation snapshot; Signature Version 4. | Download access has an expiry. Credential expiry or revocation can invalidate access earlier. | A retained URL cannot establish retrieval after permanent owner loss. A future design needs an independent access policy. |
| S5 | [tlock source](https://github.com/drand/tlock/blob/v1.2.0/tlock.go), v1.2.0; [drand timelock documentation](https://docs.drand.love/docs/timelock-encryption/), dated snapshot. | C1 uses a round signature for decryption. The documentation identifies network shutdown and threshold compromise as limitations. | Ciphertext retention supplies neither a future signature nor protection from threshold compromise. |

These sources verify narrow capabilities. They establish no duration, price, provider selection, or availability commitment for this project.
The report makes no live storage request. It uses no storage credentials or provider account.
The [comparison](models.md#primary-source-audit) records the quicknet chain identity and its fixture check.

## Candidate requirements

| Candidate | Ciphertext persistence and recovery | Release dependency and observer history |
| --- | --- | --- |
| C1: fixed tlock capsule | Preserve the original complete bundle independently. Supply an independent compatible client with the selected quicknet verification inputs. | Obtain and verify the target beacon [S5]. A later check-in cannot revoke an observer's original fixed-round copy. |
| C2: independent agents | Preserve ciphertext independently from the main server and the owner hosts. Record whether agents also store complete ciphertext. | Three receipts confirm renewal; two shares permit release under the accepted direction. Count every previously disclosed share for the same key. |
| C3: external threshold custodians | Require evidence for complete ciphertext retention and visitor access separately from private custody. No provider contract verifies these requirements here. | Sufficient custodians must preserve and publish qualifying release material. Old public material remains usable. Renewal and rollback rules require review. |
| C4: contract plus separate custody | Preserve complete ciphertext outside owner hosts. A public contract reference does not prove that any store retains the referenced bytes. | Public execution state supplies no private key custody. Sufficient custody and release access remain separate dependencies. Old public keys remain usable. |

For every candidate, deletion from owner hosts does not delete an observer's copy.
After sufficient release material becomes public, later deletion or a changed manifest cannot revoke that observer's recovery.
Key rotation does not erase an old ciphertext and its matching released key.
Private storage access cannot substitute for renewable release enforcement when a visitor already retains the ciphertext.

## Fault review record

The following traces are reproducible review scenarios. They are requirements checks, rather than executed provider or cryptographic experiments.
They use the fault definitions from #46.

| Case | Synthetic event order | Required result and remaining dependency |
| --- | --- | --- |
| V1 / F1 | Retain a complete external bundle. Stop the main server. Use a separate client and independent release endpoint. | Recovery needs no main-server route, session, or metadata lookup. Missing external release material still prevents recovery. |
| V2 / F2 | Remove all owner hosts, their domain, their index, and their credentials. Retain a visitor bundle and independent operators. | Discovery and retrieval use retained identifiers and independent access. Copies behind owner credentials fail A4 even if bytes remain. |
| V3 / F3 | Remove all independent release operators before sufficient material becomes public. Retain every ciphertext copy. | No recovery-time bound follows. If the visitor already retains valid sufficient material, later operator loss need not prevent offline recovery. |
| V4 / F4 | Set B0=100 and issued Bmax=200. Lose every response and the owner client permanently. Retain ciphertext only until 150. | The retention policy fails A3. Actual acceptance can reach 200 without another owner message. E and the recovery period require further retention. |
| V5 / F5 | Retain an old bundle. Delete the local index. Replace the public manifest. Later obtain its matching beacon or sufficient historical shares. | The old bundle remains recoverable. Current state cannot erase the observer's earlier evidence. Review must account for accumulated shares. |
| V6 / F6 | Keep valid ciphertext and release material. Block the only gateway or storage account. | Online retrieval fails. An independently retained complete bundle avoids that storage path; a second URL under the same blocked operator does not. |
| V7 / retention | Retain an identifier. Remove the last complete pin and every external copy after the retention horizon. | Discovery of the identifier cannot reconstruct absent bytes. A pin provider's continued retention needs evidence beyond the identifier [S1, S2]. |
| V8 / access | Preserve a protected object version. Expire its only presigned URL and its signing credentials. | Retention alone does not establish access [S3, S4]. An owner-host outage can therefore prevent retrieval without deleting ciphertext. |
| V9 / integrity | Supply a truncated copy, a substituted copy, and a stale manifest through the surviving locator. | The client rejects invalid bytes. Authenticated old bytes remain old evidence; a valid digest alone establishes no current release authority. |
| V10 / client | Preserve complete ciphertext and release material. Remove the only client executable and its source from all surviving locations. | The availability claim lacks A8 evidence. Storage and key release alone do not demonstrate usable recovery. |

V1 and V2 separate main loss from owner-host loss.
V3 separates ciphertext loss from independent release-operator loss.
V4 preserves the accepted uncertainty interval. It establishes no deadline enforcement mechanism or numeric publication bound.
V5 applies to all C1–C4 histories. V6–V10 apply to all candidate storage paths.
No finite collection of independent copies guarantees recovery after every copy and every client disappears.

## Inputs for #49 and the final research slice

Issue #49 needs separate cost and trust inputs for each proposed deployment:

| Input | Required quantity or evidence |
| --- | --- |
| Stored bytes | Content count, complete bundle size, copy count, version count, and metadata overhead. Include the spec's 10 MiB content limit. |
| Retention | Latest possible release horizon, E, recovery period, extension frequency, and payment continuity after owner loss. No numeric E has approval. |
| Requests and transfer | Complete retrieval size, expected visitors, retries, independent probes, repair transfers, and provider request charges. |
| Independence | Operators, providers, accounts, administrators, network paths, and shared billing or credentials. Price independence separately from copy count. |
| Access | Public access or retained authorization, credential lifetime, censorship response, alternate discovery, and gateway dependence. |
| Repair | Corruption detection interval, repair source, repair operator, and continued payment authority. Detection alone cannot repair the last lost copy. |
| Release and client | Separate beacon or custody costs, release-material retention, client distribution, and dependency preservation. |

Scenario P1 uses a retained visitor bundle plus independent release access. It needs no online ciphertext retrieval for that visitor.
Scenario P2 uses two complete external copies with distinct operators and independent visitor access.
P2 requires evidence for each retention horizon. Two copies represent a comparison input, rather than an accepted resilience threshold.
Scenario P3 uses one provider with two objects and one owner account. It remains one shared account failure case.
Compare P1–P3 under V1–V10. Record unknown prices and unavailable contractual evidence explicitly.

Issue #50 can use V1, V2, V4, V7, V8, and V9 for a synthetic proof or a documented blocker.
A future proof must identify actual implementation versions and verify retrieval after owner loss.
This report supplies neither a selected provider nor evidence that C3 or C4 satisfies its complete contract.
Any new service dependency still requires the separate maintainer decision.

## Verification scope

The source audit checks six current primary URLs. The review record covers A1–A9 through V1–V10.
Local link checks verify repository targets. A whitespace check verifies the diff.
No product behavior, public interface, dependency, or user interface changes.
Behavior TDD, pre-patch behavior tests, and product E2E tests do not apply to this document.
This report makes no new Go coverage claim. The 95.00 percent product gate remains unchanged at the batch integration gate.
The research starts no service, spends no money, and uses no real secret. Switch production use remains disabled.
