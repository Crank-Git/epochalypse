# Record the external release blocker

Date: 2026-11-05 UTC. Issue: #50. Scope: FR-research-5 and FR-research-6.
Result: documented blocker. No new service dependency receives adoption approval.

## Result and evidence

No evaluated candidate establishes a complete renewable release path after all owner-operated hosts disappear.
This is an evidence gap, not a proof that such a path is impossible.
The report takes the documented-blocker option in AC-research-5.
It adds no cryptography, production interface, fixture, or dependency.

The inputs are [models](models.md), [deadline enforcement](deadline-enforcement.md), [ciphertext availability](ciphertext-availability.md), and [cost and trust](cost-trust.md).
Their shared baseline is commit `e986b2d`. Their source audits have the date 2026-11-05 UTC.
Candidate IDs C1–C4, faults F1–F6, requirements A1–A9, and traces D-R1–D-R6 retain their meanings from those reports.
The commands below reproduce their arithmetic and inventory the existing synthetic artifacts.
They do not simulate a new external service or claim its continued availability.

| Candidate | Existing evidence | Precise blocker for renewable total-loss release |
| --- | --- | --- |
| C1: fixed tlock | The isolated experiment pins tlock v1.2.0 and quicknet. It retains ciphertext, public verification inputs, a saved beacon, and an offline client test [P1, P2]. | An observer retains the original round and ciphertext. Later check-ins cannot revoke that copy. C1 demonstrates a capsule, not a renewable switch. |
| C2: independent agents | ADR 0012 accepts bounded uncertainty, three receipts for confirmation, and two shares for release. The model report enumerates 108 synthetic receipt/outage cases. | F2 needs two qualifying agents outside all failed owner hosts. Deployment independence, retention, access, and reviewed production behavior require separate evidence. The accepted direction alone supplies no total-loss demonstration. |
| C3: external custodians | Historical TACo client 0.6.0 exposes condition interfaces. Its audited repository marks maintenance inactive [P3]. Lit v3 documents chain authority and runtime dependencies [P4]. | No verified current provider contract covers renewable custody, stale-state rejection, historical disclosure, surviving visitor authorization, retention, and independent client recovery together. |
| C4: external contract plus custody | Solidity v0.8.30 documents public contract data [P5]. CRE documents scheduled triggers [P6]. Automation v2.1 is deprecated [P7]. | Public execution supplies no private content key. No reviewed custody integration joins authoritative finality, renewal cutoff, irreversible disclosure, and independent recovery. A cron schedule supplies no release-time bound. |

The C2 protocol design review does not select an external service or demonstrate an independently operated deployment.
The production protocol gates remain separate. This report gives no numeric E or production acceptance.
C3 and C4 remain candidates. Their component documentation cannot establish a complete service contract by composition alone.

## Complete path after host loss

F1 removes the main server. F2 also removes every owner-operated host, domain index, session, and credential service.
F3 removes all relevant independent release operators. F2 and F3 are different fault scopes.
For F3 before sufficient release material exists, none of these candidates establishes a recovery-time bound.
After sufficient material survives, a visitor can potentially recover offline from complete retained inputs.

| Required part | Surviving evidence required for F2 | Current limit |
| --- | --- | --- |
| Ciphertext | Complete authenticated bytes outside owner hosts, immutable identifiers, measured lengths, independent copies, and retention through Bmax + E plus recovery. | A1–A3 define the obligations. The existing tlock fixture inventory below measures artifacts, not a complete renewable recovery bundle. |
| Key release | Independent operators enforce accepted authority and publish sufficient verifiable material without another owner message. | C1 supplies a fixed-round mechanism. C2 needs qualifying surviving agents. C3 and C4 lack the complete verified custody contract. |
| Retrieval | A visitor uses retained locators and access that survives owner loss, credential expiry, censorship, and payment failure. | A4–A7 and V1–V9 define the checks. A protected object or reachable identifier alone does not pass them. No storage service receives selection. |
| Client | A visitor retains compatible source or an executable, verification inputs, and instructions outside the main service. | A8 and V10 require this evidence. The existing isolated C1 test client has no HTTP client, but it implements no renewable custody client. |
| Continued operation | Independent probes verify complete copies and recovery inputs over the chosen recovery period. | A9 requires observations and repair. This local review supplies no external uptime, retention commitment, or operator-independence measurement. |

The cost report's 13,990,528-byte allowance is a budget bound, not an observed complete bundle length.
Its P2 storage example supplies two copies, not C2's three agents or a custody price.
A future proof must measure its complete serialized bundle and client archive separately.
It must identify operators, administrators, providers, accounts, regions, payment sources, and shared network paths before claiming A2 independence.

## Retained evidence and stale authority

| Trace | Reproducible consequence | Obligation before a future proof |
| --- | --- | --- |
| R1 / V5: retained fixed copy | The observer keeps round 1000's ciphertext. Publication of a later round leaves that original copy usable with round 1000's valid signature. | Do not offer fixed fallback as renewable revocation. Reuse the existing offline fixture tests for the narrow fixed-round claim. |
| R2 / D-R1 / V4: receipt loss | B0=100 and issued Bmax=200 permit hidden acceptance at 200. Retention ending at 150 can lose ciphertext before release. | Retain through issued Bmax, E, and recovery. Require no further owner message after permanent client loss. |
| R4 / D-R5: historical shares | The observer retains A's share and later obtains B's share for the same key. Current responses cannot erase either disclosure. | Count all historical disclosures. Reject renewal after an agent's irreversible share release for that content key. |
| D-R3 / D-R4: stale state | Cached eligibility or a replaced block can cause irreversible disclosure before authoritative renewal permits it. | Verify authoritative state, restart behavior, rollback policy, cutoff order, and finality before disclosure. |
| V3 / V6 / V7 / V10: missing inputs | Missing sufficient material, complete bytes, independent access, or a compatible client prevents a complete recovery claim. | Exercise each failure independently. A local success cannot establish survival of every external dependency. |

These traces apply to every candidate through the model and availability matrices.
C1 has immutable round authority. C2–C4 need private state enforcement before disclosure.
Neither a new manifest nor local deletion recalls an observer's sufficient old inputs.

## Reproduce the local evidence

Run this command from the repository root. It uses Python's standard library and starts no service.
It executes the reviewed Python examples in the two input reports.
It also measures the existing synthetic artifacts and prints their SHA-256 digests.

```sh
python3 - <<'PY'
from pathlib import Path
import hashlib
import re

root = Path('docs/research/external-release')
for name in ('models.md', 'cost-trust.md'):
    text = (root / name).read_text()
    blocks = re.findall(r"python3 - <<'PY'\n(.*?)\nPY", text, re.S)
    assert len(blocks) == 1, name
    exec(compile(blocks[0], str(root / name), 'exec'), {})
fixtures = Path('tests/compatibility/tlock/testdata')
for name in ('synthetic.age', 'quicknet-info.json', 'quicknet-1000.json'):
    data = (fixtures / name).read_bytes()
    assert data, name
    print(name, len(data), hashlib.sha256(data).hexdigest())
print('existing arithmetic and artifact inventory pass')
PY
```

The arithmetic reports `108 cases; 4 confirmed cases` and verifies the cost report's quota, transfer, rates, and transaction counts.
The receipt enumeration assumes exact clocks and immediate publication. It supplies no measured E or cryptographic protocol proof.
The inventory reports raw artifact lengths. It does not authenticate a beacon or decrypt content.

| Existing artifact | Measured bytes | SHA-256 |
| --- | --- | --- |
| `synthetic.age` | 393 | `482e25ef7ab1d63535f6426f37a8ac1673036d34d616cc2c9d0a0a1557b92515` |
| `quicknet-info.json` | 472 | `3e690bc527c8a4e78232bc06b5a3cff057c51c68f51b208a1a21b2abd6d6b194` |
| `quicknet-1000.json` | 205 | `7fc6e6956940f0f02920a5ff8bdad77db0406d75555caddc6d0c96ae7aef1a18` |

These three files total 1,070 bytes. They exclude the client, dependencies, and a complete capsule manifest.

For existing cryptographic reproduction, use [the compatibility procedure](../tlock-compatibility.md#offline-fixtures-and-failure-evidence) and its pinned nested module.
`TestVerifiesBeaconLocally` rejects a changed round and signature.
`TestDecryptsVersionedCiphertextFixture` recovers the saved synthetic plaintext with strict chain selection.
`TestRejectsInvalidRecoveryInputs` covers invalid recovery inputs.
The test network supplies the saved public beacon without a main-server route or HTTP client.
These tests verify fixed release. They do not exercise later renewal, external retention, live operator independence, or continued beacon publication.
The historical nested pins reproduce that experiment; [the adapter review](../tlock-production.md#dependency-review) records the separate production dependency review.

## Primary source recheck

The audit retrieves these sources again on 2026-11-05 UTC.
Managed documentation represents a dated snapshot. No new implementation version receives selection.

| ID | Primary source | Rechecked claim |
| --- | --- | --- |
| P1 | [tlock v1.2.0 source](https://github.com/drand/tlock/blob/v1.2.0/tlock.go) | Encryption targets a round. Decryption uses its signature. |
| P2 | [drand timelock](https://docs.drand.love/docs/timelock-encryption/) | Round signatures permit recovery. Threshold compromise and network shutdown remain separate risks. |
| P3 | [TACo client README at the audited commit](https://github.com/nucypher/taco-web/blob/deacead89e121d79947abdfa0704bcfba719bcfd/README.md) | Inactive maintenance notice and prospective relaunch. The audit reads the exact file through GitHub's contents API. A successor deployment has no verification here. |
| P4 | [Lit Chain Secured](https://developer.litprotocol.com/architecture/chain-secured) | Chain authority still depends on runtime access and key services. See D13–D14 for the v3 context. |
| P5 | [Solidity v0.8.30 security](https://docs.soliditylang.org/en/v0.8.30/security-considerations.html#private-information-and-randomness) | Contract variables marked private remain publicly visible. |
| P6 | [CRE cron trigger](https://docs.chain.link/cre/guides/workflow/using-triggers/cron-trigger-ts) | Scheduled workflow triggers. No complete renewable custody guarantee follows. |
| P7 | [Automation networks](https://docs.chain.link/chainlink-automation/overview/supported-networks) | Automation v2.1 deprecation date is 2026-07-31. The page directs users to CRE. |

The source audit verifies component claims. It cannot verify an absent deployment or prove that no future provider can meet the requirements.
The input reports retain the detailed source versions and capability boundaries for storage, finality, costs, and historical condition interfaces.

## Request the maintainer decision

Recommended decision: adopt no new external service dependency from this research.
Retain ADR 0012's independent-agent direction and its separate production gates.
Record the complete renewable F2 demonstration as blocked on the evidence above.
This decision accepts the research result. It grants no production deployment, purchase, account, or protocol approval.

The maintainer must choose one of these actions before any new external dependency:

1. Accept the recommendation and close this research with no new service adoption.
2. Sponsor a separate C3 or C4 investigation with a named provider and exact version or deployed contract identity.
3. Request a separate fixed C1 demonstration while explicitly retaining its irrevocable fixed-release behavior.

For option 2, require these inputs before implementation or adoption:

- A current supported custody interface and operator cohort, with authenticated renewable authority and irreversible disclosure rules.
- A reviewed cutoff, finality policy, rollback policy, receipt-loss policy, and explicit failure assumptions.
- Independent complete ciphertext, visitor access, discovery, retention, and a compatible surviving client under A1–A9.
- Retention through issued Bmax plus E and the chosen recovery period, including uncertain outcomes.
- A synthetic end-to-end recovery and negative traces for F1–F6, D-R1–D-R6, and V1–V10.
- Measured bytes and complete prices for custody, execution, transactions, storage, transfer, client distribution, and operation after owner loss.

A future external experiment needs separate authority for any resource mutation, credential, or paid service.
The present decision request requires none of them. Research can finish without selecting a provider.

## Verification scope

This change contains one research document. Local checks reproduce the existing arithmetic, inventory fixtures, and verify links and whitespace.
It changes no product behavior, public API, user interface, source code, or test.
Behavior TDD, a pre-patch behavior failure, and product E2E tests do not apply to this document.
No new Go coverage result follows. The unchanged 95.00 percent product coverage gate belongs to the final batch suite.
The batch integration gate runs the full required native checks separately.
