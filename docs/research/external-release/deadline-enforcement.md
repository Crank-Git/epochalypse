# Evaluate external deadline enforcement

Date: 2026-11-05 UTC. Issue: #47. Scope: AC-research-2 and FR-research-2.
Status: comparative research. No provider, dependency, contract, or production protocol receives approval.

## Result

Ethereum can order public renewal transactions. An external executor can request a release transition after a time condition.
Neither capability alone supplies private key custody. Solidity state remains public, including variables marked `private` [D1].
A complete renewable mechanism needs private custody that enforces authoritative state before irreversible disclosure.
This report verifies component capabilities. It establishes no complete external replacement or exact UTC availability bound.

The [producer report](models.md) defines C1–C4, F1–F6, B0, Bmax, and E.
[ADR 0012](../../adr/0012-switch-uncertainty.md) preserves bounded uncertainty after receipt loss.
Three matching durable receipts confirm C2 renewal. Two shares permit release.
Bmax includes every owner-signed renewal already issued for transmission. E has no accepted numeric value.
Issues #27 and #28 retain the production protocol gates. Issue #50 owns the eventual proof or blocker and dependency decision.

## Separate the authorities

| Capability | Verified component or accepted direction | Remaining obligation |
| --- | --- | --- |
| Public commitment | Ethereum transactions change public contract state [D2]. | Specify owner authorization, sequence, chain identity, contract identity, and signed absolute boundaries. |
| Finality | Ethereum distinguishes inclusion from finalized checkpoints [D3]. | Define which state custody accepts. Bound or explicitly exclude finality failure. |
| Deadline automation | Chainlink Runtime Environment (CRE) documents scheduled triggers [D5]. | Keep execution available after owner-host loss. Recheck current state at execution. |
| Executor availability | CRE separates workflow management from execution [D7]. | Verify access, funds, quotas, runtime continuity, and recovery paths before selection. |
| Private custody | C2 assigns a private share to each agent. C3 requires external threshold custody. | Verify independent operators, durable state, threshold availability, and irreversible disclosure rules. |
| Renewal cutoff | No reviewed external contract exists in this project. | Define cutoff, transaction order, finality, and custody behavior together. |
| Clock and network | Solidity v0.8.30 exposes the current block timestamp [D1]. | Establish a reviewed relation to UTC and a failure policy. Block time supplies no publication bound. |

These obligations describe a proposed comparison contract. They are not new API definitions or approved protocol rules.
The v0.8.30 documents bound the Solidity claims. This report neither adopts that compiler nor calls it the current release.
Ethereum documentation has no pinned client release here. A future implementation must select its client, network configuration, and contract version.

## Candidate comparison

| Candidate | Execution and custody | Renewal conclusion |
| --- | --- | --- |
| C1: fixed tlock v1.2.0 on quicknet | The selected round's beacon permits decryption. The producer report verifies the chain identity and library interface. | Fixed release only. Later check-ins cannot revoke an observer's original bundle. Beacon continuity remains necessary before that round. |
| C2: three independent agents | Each agent enforces durable accepted state and publishes its share. This is the accepted research direction. | Renewable within the reviewed assumptions. Confirmation needs three receipts. Recovery targets Bmax + E with two reachable, honest, durable agents. Production review remains pending. |
| C3: external threshold custodians | TACo supplies a concrete historical capability to compare. Current service and protocol guarantees remain unverified below. | Potentially renewable through authoritative external state. No verified provider satisfies the complete project contract. |
| C4: Ethereum contract plus separate private custody | Public state can order renewals. CRE is a current automation candidate. Custody must enforce the resulting state privately. | Potentially renewable. No reviewed integration establishes cutoff, finality, disclosure, or recovery guarantees. A public contract alone is incomplete. |

Every candidate requires independent ciphertext and authenticated metadata. Every recovery path also needs a compatible client outside the failed infrastructure.
Issue #48 owns persistence, discovery, integrity, and retrieval requirements. Execution evidence does not prove those requirements.

## C4: Ethereum and external execution

Ethereum requires a transaction to change contract state. A sender signs the transaction and pays execution fees [D2].
A timestamp condition does not submit that transaction. A surviving visitor, executor, or custodian must initiate the next step.
A proposed mechanism can permit any funded sender to request release while the contract checks eligibility.
This alternative reduces exclusive executor authority. It still requires a reachable sender and sufficient transaction inclusion.

Submission does not prove inclusion. Inclusion does not prove finality [D2, D3].
Finality depends on validator votes, and sufficient withheld votes can stop finality [D3].
An unavailable remote endpoint also prevents a client from learning state, even when the underlying chain continues.
Custody must authenticate chain state rather than trust an arbitrary endpoint response.
No reviewed verification method or endpoint redundancy policy exists for this candidate.

The official Automation documentation states that v2.1 sunsets on 2026-07-31. It directs users to CRE [D4].
CRE documents cron expressions with a minimum 30-second interval [D5].
That interval describes scheduling granularity. It supplies no maximum delay for transaction inclusion, finality, private release, or retrieval.
The current release notes identify CLI v1.39.0, dated 2026-10-21 [D8].
This report installs no CLI or SDK. No exact SDK or deployed runtime version has verification here.

CRE chain reads default to `LATEST`; reads also support `FINALIZED` [D6].
Its `WriteReportReply` with `TX_STATUS_SUCCESS` denotes inclusion rather than finality [D6].
A proposed release path must verify authoritative state after a write. A successful workflow response cannot replace that check.
Public workflow management requires a linked wallet and Ethereum Mainnet transaction gas [D7].
Private workflow management uses the CRE login session [D7]. Neither registry choice alone makes execution confidential [D7].

A future integration must verify deployment access, supported networks, workflow limits, and execution payment before adoption.
It must also verify consumer authorization for executor reports. This report approves no deployment or service account.
CRE availability, chain inclusion, custody availability, and ciphertext retrieval remain separate dependencies.
The release notes warn that chain upgrades can interrupt writes [D8]. No uninterrupted deadline guarantee follows from the documented trigger.

## C3: concrete custody evidence and limits

TACo is a plausible threshold candidate because nodes check access conditions before they return decryption material [D9].
The current documentation includes a missed-check-in example [D9]. This makes it relevant to renewable release rather than merely fixed time.
The audited client package identifies version 0.6.0 at commit `deacead89e121d79947abdfa0704bcfba719bcfd` [D10].
Its `TimeCondition` and `ContractCondition` classes provide concrete condition interfaces [D11].
A fixed time condition alone cannot move an observer's retained ciphertext to a later boundary.
A contract condition could instead consult current external state. That possibility is an inference, not a verified switch implementation.

Current NuCypher repositories mark the client and node runtime inactive [D10, D12].
The documentation plans a stable relaunch in Q3 2026 under new stewardship [D9].
That planned date has passed by this report's 2026-11-05 audit date.
This audit verifies neither a deployed successor nor service continuity.
This audit verifies no successor version, operator cohort, threshold, availability commitment, or renewal protocol.
The historical package version therefore supplies interface evidence, not an approved current service.

Selection requires evidence for these unresolved capabilities:

- A surviving visitor can request decryption without the lost owner's account or client.
- Enough independent operators retain usable private material for the full content lifetime.
- Custodians verify authoritative state and reject rollback across restart and network failure.
- Custodians enforce a common cutoff and count disclosures across every observer's history.
- No custodian accepts renewal after its irreversible disclosure for the same content key.
- The client can recover sufficient material without another owner message after F4.

Lit supplies a separate current custody comparison. Its documentation identifies v3, also called Chipotle [D13].
It describes enclave-held keys and chain-controlled authority. It does not establish C3's independent threshold contract.
Its authority documentation explicitly retains runtime and key-service availability dependencies [D14].
It also permits runtime operators to pause signing [D14].
This report verifies no versioned renewable decryption interface or bounded release path for Lit v3.
Older descriptions of distributed custody cannot substitute for the current architecture.

Neither historical TACo interfaces nor Lit v3 documentation closes C3's complete requirements.
This is an evidence gap, not a claim that external renewable custody is impossible.
The report adds no cryptographic construction to compensate for that gap.

## Renewal order and uncertainty

The following review cases preserve the producer report's accepted semantics.
They are explicit traces for a proposed mechanism, not executed contracts or a proof of correctness.

| Case | Event order | Required conclusion |
| --- | --- | --- |
| D-R1: lost reply | Client journals signed Bmax=200, submits renewal, and loses every reply. B0=100. Client disappears permanently. | Acceptance remains uncertain. Transmission failure does not prove rejection. Recovery must need no later owner message. |
| D-R2: delayed request | Owner signs an absolute boundary of 200. A relay delivers it much later. | Arrival time cannot extend the signed boundary. The contract needs a defined cutoff for late acceptance. |
| D-R3: stale executor | Executor observes boundary 100. A renewal to 200 precedes its release transaction in authoritative order. | Execution must check current authority. A cached eligibility result cannot authorize disclosure. |
| D-R4: chain rollback | Custodian releases from a recent block. The chain replaces that block with a renewal history. | The observer retains disclosed material. Later finality cannot reverse disclosure. Define a finality policy before release. |
| D-R5: release precedes renewal | Sufficient custodians disclose for a key. A later transaction records boundary 200. | The observer can still decrypt the old ciphertext. The later transaction cannot recall material. |
| D-R6: finality stops | Boundary passes while finality or custody access fails. | Block time alone cannot establish availability by Bmax + E. State the additional liveness assumptions or record no bound. |

C2's B0 and Bmax remain the comparison inputs. C4 needs its own reviewed authoritative-state and confirmation rules.
An Ethereum transaction receipt is not one of C2's signed durable agent receipts.
No automatic translation of three receipts into a finalized transaction has acceptance.
The proposed contract must define the race at the cutoff, including equal timestamps and transactions within one block.
It must preserve owner-signed absolute boundaries. It must prevent a delayed request from extending authority through its arrival time.

## Fault conclusions for the next reports

| Fault | C3 and C4 consequence | Input for #49 |
| --- | --- | --- |
| F1: main loss | External authority and private custody can continue only through independent surviving paths. | Account for executor, custody, publication, and client operation separately. |
| F2: all owner-host loss | Externally operated components must retain the required state and access. Owner-host relays cannot be mandatory. | Verify operator independence, payment continuity, credentials, and public recovery permissions. |
| F3: all independent-operator loss | Public contract state cannot replace private material. No bound exists before sufficient material survives outside the operators. | Identify threshold collusion and threshold unavailability separately. |
| F4: receipt and owner-client loss | An unknown response can hide acceptance. A surviving visitor needs authority discovery without owner participation. | Include journal export, chain-state verification, and independent client access costs. |
| F5: stale state and observer copies | Rollback or a stale query can cause irreversible disclosure. Old ciphertext and sufficient old material remain usable. | Include verification, durable monotonic state, and retained-history review. |
| F6: network and clock failure | Executor outage, censorship, finality failure, and custody outage can delay release. No exact UTC bound has verification. | Price independent access and state explicit failure assumptions. |

C1 and C2 retain the producer report's distinct fault conclusions. C1 cannot renew its public fixed-time bundle.
C2 requires two qualifying agents for its bounded recovery target. C3 and C4 have no verified equivalent bound.
Issue #49 must leave unresolved services and custody costs unpriced rather than assume zero cost.

## Primary source audit

Retrieve date for all sources: 2026-11-05 UTC. Dated snapshots do not pin deployed software.
The source identifiers record the capability review. They do not approve an implementation.

| ID | Primary source and exact version or snapshot | Evidence checked |
| --- | --- | --- |
| D1 | [Solidity v0.8.30 security](https://docs.soliditylang.org/en/v0.8.30/security-considerations.html#private-information-and-randomness), [global variables](https://docs.soliditylang.org/en/v0.8.30/units-and-global-variables.html#block-and-transaction-properties) | Public visibility and `block.timestamp`. |
| D2 | [Ethereum transactions, dated snapshot](https://ethereum.org/en/developers/docs/transactions/) | Signatures, fees, inclusion, and transaction lifecycle. |
| D3 | [Ethereum proof of stake, dated snapshot](https://ethereum.org/en/developers/docs/consensus-mechanisms/pos/#finality) | Checkpoints, finality, and withheld votes. |
| D4 | [Automation supported networks, dated snapshot](https://docs.chain.link/chainlink-automation/overview/supported-networks) | Sunset dates and CRE successor. |
| D5 | [CRE cron trigger, TypeScript documentation snapshot](https://docs.chain.link/cre/guides/workflow/using-triggers/cron-trigger-ts) | `CronCapability.trigger()`, schedule, and minimum interval. |
| D6 | [CRE finality, TypeScript documentation snapshot](https://docs.chain.link/cre/concepts/finality-ts) | Read defaults and write-success limits. |
| D7 | [CRE workflow deployment, dated snapshot](https://docs.chain.link/cre/guides/operations/deploying-workflows) | Registry authorization, wallet, gas, and confidentiality distinction. |
| D8 | [CRE release notes, CLI v1.39.0](https://docs.chain.link/cre/release-notes) | Version date and upgrade interruption warning. |
| D9 | [TACo documentation at d489debc58d93c0c5c2d28b8a62b215697ae9e55](https://github.com/nucypher/taco-docs/tree/d489debc58d93c0c5c2d28b8a62b215697ae9e55) | Conditional threshold release, missed-check-in example, and relaunch announcement. |
| D10 | [TACo client at deacead89e121d79947abdfa0704bcfba719bcfd](https://github.com/nucypher/taco-web/tree/deacead89e121d79947abdfa0704bcfba719bcfd), [package](https://github.com/nucypher/taco-web/blob/deacead89e121d79947abdfa0704bcfba719bcfd/packages/taco/package.json) | Inactive notice and package version 0.6.0. |
| D11 | [TimeCondition](https://github.com/nucypher/taco-web/blob/deacead89e121d79947abdfa0704bcfba719bcfd/packages/taco/src/conditions/base/time.ts), [ContractCondition](https://github.com/nucypher/taco-web/blob/deacead89e121d79947abdfa0704bcfba719bcfd/packages/taco/src/conditions/base/contract.ts) | Exact historical constructors and condition schemas. |
| D12 | [TACo node runtime, dated snapshot](https://github.com/nucypher/nucypher) | Inactive notice; no verified successor deployment. |
| D13 | [Lit documentation, v3 Chipotle snapshot](https://developer.litprotocol.com/) | Current version and enclave architecture. |
| D14 | [Lit Chain Secured, v3 documentation snapshot](https://developer.litprotocol.com/architecture/chain-secured) | Public authority, runtime dependency, and operator pause capability. |

## Reproducible review and verification

Read each source at the recorded version. Compare its documented capability with the corresponding candidate row.
Verify mutable-source status again before a service decision. A reachable documentation link establishes neither service availability nor protocol correctness.
Resolve the two TACo repository commits with read-only GitHub API calls:

```sh
gh api repos/nucypher/taco-web/commits/main --jq .sha
gh api repos/nucypher/taco-docs/commits/main --jq .sha
```

The audit returns the two commits recorded in D9 and D10.
Review D-R1–D-R6 against the authority table and F1–F6. Each trace records the missing rule or required consequence.
The link check verifies every report link with bounded reads. The prose review checks scope, source claims, and STE.
No source, test, dependency, public interface, or user interface changes.
TDD, pre-patch behavior tests, and product E2E tests do not apply to this research document.
This report claims no Go coverage result. The existing 95.00 percent gate remains at the integration gate.
No service starts, provider mutation occurs, real secret enters the report, or production switch receives enablement.
