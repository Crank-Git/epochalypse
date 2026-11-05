# Compare external release cost and trust

Date: 2026-11-05 UTC. Issue: #49. Scope: AC-research-4 and FR-research-4.
Status: comparative research. This report approves no service, purchase, account, or dependency.

## Result

C1 provides fixed release under beacon assumptions. It cannot provide renewable release for an observer's retained bundle.
C2 remains the accepted direction, subject to production model and protocol review.
C3 lacks a verified current provider contract. C4 adds public execution but still requires separate private custody.
No defensible total price exists for C3 or C4 until those contracts exist.

The [model report](models.md) defines C1–C4 and F1–F6.
The [deadline report](deadline-enforcement.md) defines execution and custody limits through D1–D14.
The [availability report](ciphertext-availability.md) defines A1–A9 and V1–V10.
These identifiers retain their original meanings here.
[ADR 0012](../../adr/0012-switch-uncertainty.md) accepts three independent agents, three matching durable receipts for confirmation, and two release shares.
After incomplete receipts, uncertainty spans B0 through Bmax. E remains separate and has no accepted numeric value.
Research does not enable production switches or replace the gates in #27 and #28.

## Cost boundaries

The following expression separates costs that a public endpoint or a storage rate can hide:

```text
total = ciphertext storage + requests + transfer + release-material retention
      + release operators + state verification + executor + transactions
      + independent discovery + recovery client distribution + maintenance
```

Maintenance includes durable state, rollback review, clock checks, repair, security updates, and response to outages.
Operator labor and independent administration have no verified rate here. A low storage subtotal cannot price them.
Public beacon access does not make the complete C1 recovery path free.
The audit finds no per-capsule beacon tariff in the reviewed sources. It verifies no permanent free-service commitment.
An owner-host outage must not require a new owner payment for recovery.
Each independent operator needs a documented payment authority and retention obligation that survives that outage.

| Candidate | Preparation and renewal cost | Release and recovery cost | Unpriced obligation |
| --- | --- | --- | --- |
| C1: tlock v1.2.0, quicknet | Local preparation and complete bundle distribution. No renewal can revoke the original round. | Beacon access, verification, ciphertext copies, and compatible recovery client. | Long-term beacon continuity, storage access, independent discovery, and client maintenance. |
| C2: independent agents | Three durable services. Each confirmed renewal needs three matching receipts. Retried requests add work. | Two qualifying agents publish shares. Each agent also needs its durable ciphertext copy under the spec architecture. | Host resources, independent administration, durable state, clock bounds, share publication, and protocol review. No current host quote exists here. |
| C3: external custodians | Provider-specific preparation, state checks, authorization, and renewal fees remain unknown. | Sufficient surviving custodians, public release access, storage, and a compatible client remain necessary. | Complete current contract, exact deployed version, threshold, quotas, retention, retrieval rights, and fees. |
| C4: Ethereum plus custody | Contract deployment, renewal transactions, state verification, workflow operation, and private custody. | Release transaction, finality verification, sufficient custody, ciphertext retrieval, and recovery client. | Contract gas measurements, executor tariff, custody tariff, endpoint cost, funds after owner loss, and complete protocol. |

C2's three copies do not follow from the two-copy P2 storage example below.
P2 isolates storage economics. It does not change the project's three-agent architecture or establish an accepted resilience threshold.

## Transparent storage scenarios

All quantities below are synthetic accounting inputs. They are not measured provider bills or approved deployment settings.
One MiB means 1,048,576 bytes. One GiB means 1,073,741,824 bytes.
The calculations use decimal GB=1,000,000,000 bytes and TB=1,000,000,000,000 bytes for the quoted rates.
Confirm the provider's metering and invoice before any purchase.

| Input | Explicit assumption |
| --- | --- |
| Content | 50 objects, each with 10 MiB of plaintext: 10,485,760 bytes. |
| Wire format | Compact JWE for C1 content. The accounting allowance includes its encoded content and public metadata. No C2–C4 wire format receives approval. |
| Complete object allowance L | `EncodedObjectLimit(10 MiB) + 8192 + 256` = 13,990,528 bytes. |
| Format limits | `EncodedObjectLimit` includes encoded content, the protected header, the initialization vector, the authentication tag, and separators. The bundle adds 8192 metadata bytes and 256 envelope bytes. |
| Scope of L | Use the existing bounds in `internal/capsule/object.go` and `docs/capsules.md`. Count the complete stored or transmitted allowance, including the key wrapper and manifest reserve. |
| Actual format evidence | L is a budget, not an observed serialization length. A proof must measure its actual bundle and separately add any excess, container overhead, or client archive. |
| One complete copy S | `50 * L` = 699,526,400 bytes, or 0.6995264 decimal GB. One installation remains below 100 objects and 1 GiB. |
| Retention | Constant storage for twelve 30-day periods. The synthetic latest possible boundary is day 330. Recovery lasts 30 days. |
| Allowance E | Unpriced and unresolved. The twelve-period budget suffices only if `Bmax + E + recovery_period <= day 360`. Extend the budget otherwise. |
| Visitors | 100 complete downloads per object per month, plus one retry per initial download: 10,000 object reads. |
| Probes | One complete download per object per day for 30 days: 1,500 additional reads per copy. |
| Transfer Q | 11,500 complete reads per copy per month: 160,891,072,000 bytes, or 160.891072 decimal GB. |
| Writes | 50 initial object uploads per copy in month one. No replacements, repair, or extra versions in this baseline. Add their full bytes and requests when they occur. |
| Client and discovery | A visitor retains a compatible client, authenticated manifest, immutable locators, and verification inputs outside owner hosts. Distribution and upkeep have no quote here. |
| Custody and transactions | P1–P3 price ciphertext only. They include no private custody, executor, renewal transactions, or release transactions. Add the candidate-specific costs separately. |

The encoded allowance prevents the mistaken assumption that 100 objects of 10 MiB fit the default 1 GiB quota.
A hundred such allowances require 1,399,052,800 bytes before additional installation state.
The scenario uses no compression or provider-side deduplication discount.
The twelve-period scenario does not promise indefinite retention or establish a numeric E.
An issued Bmax can exceed B0 without a receipt. Price retention through Bmax, E, and the recovery period, as V4 requires.

| Scenario | Copies, operators, and access | Ciphertext expense and limitation |
| --- | --- | --- |
| P1: visitor-retained bundle | One complete visitor-held copy. The visitor retains the client and manifest. | Online ciphertext egress can be zero for that visitor. Local media, repair, client upkeep, and release access remain unpriced. |
| P2: distinct external operators | Two complete copies. For rate illustration only, one uses R2 Standard and one uses B2. Each needs independent administration and visitor access. | Stored bytes total `2*S`; baseline transfer totals `2*Q` monthly. Distinct provider names alone do not prove A2 independence or A4 access. |
| P3: shared account | Two complete object sets in one R2 account under one operator. | Stored bytes total `2*S`; transfer totals `2*Q`. Account suspension or shared payment failure affects both sets. This is not two independent operators. |

P2 assumes every copy serves the full workload during a failover or independent probe campaign.
It therefore avoids an unsupported discount from a perfect traffic split.
All scenarios require separate key release. A visitor's ciphertext copy contains no unreleased plaintext key.

## Dated rate examples

These primary sources describe managed services, not pinned software releases. The audit date is 2026-11-05 UTC.
The examples quote United States dollars. Taxes, discounts, account charges, currency changes, and other metered services remain outside these subtotals.

R2 Standard lists $0.015 per GB-month, $4.50 per million Class A operations, and $0.36 per million Class B operations [K1].
Direct R2 egress has no transfer charge. Other connected services can charge separately [K1].
R2 rounds usage up to billing units [K1].
Its monthly Standard allowance includes 10 GB-month, one million Class A operations, and ten million Class B operations [K1].
If those allowances remain available, P2's R2 ciphertext subtotal is $0 for the stated workload.
If other account usage consumes them, a standalone rounded-unit illustration costs `$0.015 + $4.50 + $0.36 = $4.875` in month one.
Later months cost `$0.015 + $0.36 = $0.375` with no further writes.
That illustration totals $9.00 across twelve periods. Actual incremental charges depend on account aggregation and rounding.
P3 shares allowances and rounding across both sets. With exhausted allowances, its illustration totals `$0.03*12 + $4.50 + $0.36*12 = $9.18`.
Neither subtotal includes an independent operator or a retention commitment.

B2 lists storage from $6.95 per TB per 30 days, based on byte-hours [K2].
Its published example uses U.S. West rates. It lists free transactions and direct egress up to three times average monthly storage [K2].
Additional egress costs $0.01 per GB [K2]. No partner-transfer discount enters this scenario.
B2 includes the first 10 GB of storage free [K2].
If that allowance remains available, P2's B2 storage subtotal is $0.
Its egress subtotal is `$0.01*max(0,Q-3*S)/10^9 = $1.587924928` per period.
Twelve periods total $19.055099136 for storage and egress before invoice rounding or other charges.
If other account usage consumes the storage allowance, the gross storage subtotal is `$6.95*S/10^12 = $0.004861708480` per period.
That gross illustration totals $19.113439637760 across twelve periods, including the same egress subtotal.
These are arithmetic subtotals, not an invoice prediction or a binding provider offer.
P2 combines that subtotal with the applicable R2 subtotal. It still excludes custody, client distribution, labor, and independent access guarantees.

R2's example shows why a fractional request rate is not always a fractional invoice.
B2's example shows why a small retained object set can still incur substantial transfer relative to storage.
An increased visitor count or repair campaign changes Q. It does not change the release mechanism's trust properties.

## Execution and custody expense

For C4, assume one contract deployment, one arm transaction per object, 30 renewals per object, and one release transaction per object.
For 50 objects, this gives `1 + 50 + 1500 + 50 = 1601` transactions.
This is an unbatched comparison assumption. No contract implementation verifies these counts or its gas use.
The complete ciphertext stays off-chain. Transactions carry authority state, not plaintext keys or the full content object.

Ethereum's fee formula uses gas consumed multiplied by the base fee plus the priority fee [K3].
For changing fees, the total is `sum(gas_i * (base_i + priority_i))` in native units.
Deployment and contract calls need measurements from actual bytecode. A simple transfer's gas count does not price this contract.
Failed executions can also consume gas [K3]. Add retries and public workflow lifecycle transactions when applicable.
No fixed dollar conversion or guaranteed inclusion price follows from this formula.
Fee spikes, exhausted balances, endpoint failure, and withheld transactions can prevent timely release.

CRE deployment requires access approval [K4]. This audit requests no access.
Its private registry needs no registry-management gas; its public registry uses Ethereum Mainnet transactions [K4].
Neither choice establishes free workflow execution or private custody.
CLI v1.39.0 supplies a dated component reference [K5]. The reviewed documentation supplies no verified complete tariff for this workload.
Obtain execution, quota, custody, and payment-continuity terms before a total C4 estimate.
The documentation dates Automation v2.1's sunset to 2026-07-31 [K6].
That date precedes this audit. Its historical tariffs cannot establish a current CRE estimate.
The documented CRE schedule interval does not bound inclusion, finality, private release, or retrieval; see D5–D8.

C3 has no defensible current custody quote. The historical TACo client remains at the audited 0.6.0 snapshot [K7].
Its inactive notice and prospective relaunch do not establish a supported successor service or a renewable release contract.
The TACo documentation plans a Q3 2026 relaunch. That quarter ends before this audit; no successor deployment has verification here [K12].
Lit v3 retains runtime and key-service dependencies. Its operator can pause access [K8].
Its documented authority does not verify C3's threshold independence or this project's release path.
Current capability evidence must precede a price comparison between either provider and C2.

## Trust, censorship, and operational failures

The following rows are deductions from the producer reports and the dated sources. They are not measured failure probabilities.

| Candidate | Confidentiality and trust | Censorship and network failure | Operational effort and shared failures |
| --- | --- | --- | --- |
| C1 | Trust the selected beacon's threshold assumptions and local preparation. Threshold compromise can disclose early [K9]. Retained bundles keep their original round. | A blocked relay can delay signature access. Independent paths help only if the required beacon exists. Complete retained inputs permit later offline recovery. | Preserve exact quicknet identity, wrapper, complete bytes, and client. Network shutdown before the required beacon removes the release path. Storage and relay separation do not create a new beacon. |
| C2 | Two colluding agents can disclose early. Three receipts confirm renewal; two honest, reachable agents support the accepted recovery target. | One unavailable agent fits the accepted direction. Two inaccessible agents prevent sufficient new shares. Shared provider, administrator, or network censorship can affect several agents. | Maintain three durable states and ciphertext copies. Review clocks, rollback quarantine, share history, and payment continuity. Three hosts under one owner account do not establish independent operation. |
| C3 | Verify threshold, operator independence, state authority, and every historical disclosure. Current evidence establishes no complete qualifying provider contract. | Enough custodians can withhold material. A provider account, request policy, or shared state endpoint can block recovery despite retained ciphertext. | Verify operator continuity, custody migration, rollback rules, fees, authorization after owner loss, and compatible clients. An enclave architecture requires its own trust analysis. |
| C4 | Public contract state cannot keep a content key private. Trust private custody and its authenticated view of authoritative chain state. | Sender, executor, endpoint, validator, or custodian failure can stop the path. Inclusion does not establish finality. Censorship resistance supplies no exact UTC publication bound. | Maintain funds, executor access, contract verification, endpoint diversity, custody, and external storage. Separate operators can still share one chain, cloud provider, account, or administrator. |

For every candidate, an observer retains old ciphertext and every previously public key, beacon, or share.
A later manifest, local deletion, or key rotation cannot revoke sufficient retained inputs.
For C2–C4, custody must reject stale authority before disclosure. An irreversible disclosure cannot be repaired by later finality or state correction.
For C1, a later check-in cannot alter the observer's original fixed release.

| Fault | Required cost and trust conclusion |
| --- | --- |
| F1: main loss | Independent release, ciphertext access, and client operation need separate evidence. A main-server proxy defeats that path. |
| F2: all owner-host loss | External operators need independent state, discovery, access, administration, and payment. An owner credential or owner-host relay cannot remain mandatory. |
| F3: independent-operator loss | Retained sufficient release material can permit recovery. Without it, ciphertext or public contract state alone supplies no recovery-time bound. |
| F4: receipt and owner-client loss | Missing responses do not prove rejection. Retain ciphertext through issued Bmax, E, and recovery. No further owner message can be mandatory. |
| F5: stale state and observer copies | Count disclosures across all histories. Budget durable authority and verification. Current endpoint responses cannot erase an observer's prior inputs. |
| F6: network and clock faults | Price independent paths and state their shared dependencies. Without the reviewed reachability and clock assumptions, no Bmax + E bound follows. |

IPFS content identifiers do not purchase retention. Recursive pins require surviving operators and complete blocks [K10].
S3 Object Lock protects object versions against deletion. It does not alone establish surviving visitor access [K11; V8].
These capabilities select no storage service. P2's rate sources establish neither A2 independence nor A4 access.
An access policy, paid account, or gateway can censor recovery even when the storage provider retains the bytes.

## Inputs for #50 and a maintainer decision

Issue #50 can reproduce the arithmetic below without an account or a new dependency.
It can use V1, V2, V4, V7, V8, V9, and V10 to evaluate a synthetic recovery path.
A proof must record actual wire bytes and implementation versions. This report's allowance cannot substitute for that measurement.
For C3 and C4, record the missing complete custody contract as a blocker unless primary evidence closes it.
Do not introduce new cryptography to close the gap.

Before any new dependency, the maintainer needs these concrete inputs:

- Choose fixed C1 release or a renewable mechanism. A fixed fallback remains irrevocable for an observer's retained bundle.
- For C2, retain the accepted bounded uncertainty direction and complete the production protocol gates.
- For C3 or C4, require a versioned custody contract and an independently recoverable client before selection.
- Identify every operator, provider, administrator, account, payment source, and network path for A2.
- Set the recovery period and retention policy through Bmax plus E. Keep E subject to protocol review.
- Require surviving access without a new owner message, credential renewal, or owner payment after F2 and F4.
- Obtain actual host, custody, executor, transaction, transfer, and client distribution costs for the selected workload.
- State tolerated censorship and failure assumptions. A provider rate or schedule interval cannot establish a release guarantee.

These are inputs for the maintainer's later decision. They grant no service selection, purchase, deployment, or new external dependency.
The accepted C2 direction does not imply C3 or C4 adoption. No numeric E or production acceptance follows from this report.

## Primary source audit

All external sources below have a retrieval date of 2026-11-05 UTC.
Unversioned managed-service pages represent dated documentation snapshots, not adopted implementation versions.
The source audit checks capability and price separately. Reachable documentation does not establish future service continuity.

| ID | Primary source and exact version or snapshot | Evidence |
| --- | --- | --- |
| K1 | [R2 Standard rates](https://developers.cloudflare.com/r2/pricing/), page updated 2026-10-30. | Rates, allowances, billing-unit rounding, and direct egress distinction. |
| K2 | [B2 rates](https://www.backblaze.com/cloud-storage/pricing), dated snapshot. | $6.95/TB/30-days, first 10 GB free, byte-hours, U.S. West example, transactions, and excess egress. |
| K3 | [Ethereum gas](https://ethereum.org/developers/docs/gas/), page updated 2026-10-23. | Variable fees, gas use, and failed execution cost. No client or contract version selected. |
| K4 | [CRE deployment](https://docs.chain.link/cre/guides/operations/deploying-workflows), dated snapshot. | Deployment access and private versus public registry cost boundaries. |
| K5 | [CRE release notes](https://docs.chain.link/cre/release-notes), CLI v1.39.0, 2026-10-21. | Component version only; no runtime adoption or full-workload tariff. |
| K6 | [Automation networks](https://docs.chain.link/chainlink-automation/overview/supported-networks), v2.1 sunset 2026-07-31. | Historical Automation is not the current executor baseline. |
| K7 | [TACo client notice](https://raw.githubusercontent.com/nucypher/taco-web/deacead89e121d79947abdfa0704bcfba719bcfd/README.md), commit matching D10's client 0.6.0. | Inactive development and prospective successor. No verified current contract or quote. |
| K8 | [Lit Chain Secured](https://developer.litprotocol.com/architecture/chain-secured), v3 Chipotle documentation snapshot; see D13–D14. | Runtime and key-service dependency, operator pause authority, and chain authority distinction. |
| K9 | [drand timelock](https://docs.drand.love/docs/timelock-encryption/), dated snapshot; [tlock source](https://github.com/drand/tlock/blob/v1.2.0/tlock.go), v1.2.0. | Fixed-round behavior, threshold compromise, and network shutdown limits. |
| K10 | [IPFS pins](https://docs.ipfs.tech/how-to/pin-files/), dated snapshot; no Kubo version selected. | Complete pin and operator dependency. No retention tariff. |
| K11 | [S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html), dated snapshot. | Protection applies to object versions. No retrieval or provider-independence guarantee. |
| K12 | [TACo documentation](https://raw.githubusercontent.com/nucypher/taco-docs/d489debc58d93c0c5c2d28b8a62b215697ae9e55/README.md), commit matching D9. | Planned Q3 2026 relaunch; the elapsed date supplies no deployment evidence. |

## Reproducible review and verification

Run this arithmetic check from the repository root:

```sh
python3 - <<'PY'
from decimal import Decimal as D
from pathlib import Path
import re
plaintext = 10 * 2**20
source = Path('internal/capsule/object.go').read_text()
assert 'const protectedHeaderLimit = 1024' in source
assert 'const overhead = protectedHeaderLimit + 16 + 22 + 4' in source
assert 'return encodedLength(limit) + overhead, nil' in source
assert 'return size/3*4 + (size%3*8+5)/6' in source
guide = Path('docs/capsules.md').read_text()
reserves = re.search(r'`EncodedObjectLimit\(limit\)`, (\d+) metadata bytes, and (\d+) envelope bytes', guide)
assert reserves is not None
metadata, envelope = map(int, reserves.groups())
assert (metadata, envelope) == (8192, 256)
encoded_object_limit = (plaintext * 8 + 5)//6 + 1024 + 16 + 22 + 4
length = encoded_object_limit + metadata + envelope
stored = 50 * length
transfer = (10000 + 1500) * length
assert (length, stored, transfer) == (13990528, 699526400, 160891072000)
assert stored < 2**30 < 100 * length
egress = D('.01') * (transfer - 3*stored) / 10**9
gross_storage = D('6.95') * stored / 10**12
net_storage = D('6.95') * max(0, stored - 10*10**9) / 10**12
assert net_storage == 0
assert (net_storage + egress) * 12 == D('19.055099136')
assert (gross_storage + egress) * 12 == D('19.113439637760')
assert D('.015') * 12 + D('4.50') + D('.36') * 12 == D('9.000')
assert D('.03') * 12 + D('4.50') + D('.36') * 12 == D('9.18')
assert 1 + 50 + 30*50 + 50 == 1601
print('wire allowance, quota, transfer, rate, and transaction checks pass')
PY
```

Review P1–P3 against A1–A9 and V1–V10. Review each candidate against F1–F6 and D-R1–D-R6.
Recheck K1–K12 before a maintainer decision. Verify local links and whitespace in this document.
The record contains explicit assumptions, source versions, arithmetic, fault consequences, and unresolved contracts.
No product behavior, test, public interface, dependency, or user interface changes.
Behavior TDD, pre-patch behavior tests, and product E2E tests do not apply to this research-only change.
This report claims no new Go coverage result. The 95.00 percent product gate remains unchanged at the batch integration gate.
The audit starts no service, requests no account, purchases nothing, and uses no real secret.
