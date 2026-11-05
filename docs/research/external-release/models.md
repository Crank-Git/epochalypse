# Compare fixed capsules and renewable switches

Date: 2026-11-05 UTC. Issue: #46. Scope: AC-research-1 and AC-research-7.
Status: comparative research. This report selects no service or dependency.

## Result and authority

A fixed tlock capsule can survive owner-host loss if a visitor retains its complete bundle and obtains the selected beacon.
A later check-in cannot revoke an observer's copy of that bundle.
A renewable switch instead requires private custody that enforces the latest accepted state before irreversible release.
An external execution network can supply public decision state. It does not alone supply private custody or durable ciphertext retrieval.

[ADR 0012](../../adr/0012-switch-uncertainty.md) accepts three independent agents, three matching durable receipts for confirmation, and two release shares.
It accepts bounded uncertainty after an incomplete response. It does not accept a production protocol or a numeric E.
[The direction report](../switch-protocol/directions.md) supplies the limited argument for this tradeoff.
[The replacement report](../switch-protocol/replacement.md) preserves the contradiction under the superseded exact-confirmation requirement.
Its historical contradiction does not block this comparison or establish that the revised direction fails.
Issues #27 and #28 retain the production model and independent review gates.

## Terms and evaluation inputs

The [spec Terms table](../../specs/spec.md#terms) controls domain words.
The identifiers below label research cases. They define no production interface or wire format.

| Input | Meaning and required record |
| --- | --- |
| B0 | Last release boundary confirmed by three matching durable receipts; earliest possible disclosure after uncertain renewal. |
| Bmax | Latest release boundary in any owner-signed renewal already issued for transmission, including requests without receipts. |
| E | Separate reviewed allowance for clock error and publication work; no accepted numeric value. |
| Accepted state | Agent state that survives ordinary restart before the agent signs its receipt. |
| Confirmation | Client receipt of three matching durable receipts; neither transmission nor one receipt establishes confirmation. |
| Observer evidence | All ciphertext, manifests, keys, shares, and old responses that an observer retains across the complete history. |
| Recovery inputs | Complete ciphertext, authenticated metadata, required key material, and a compatible client outside the failed infrastructure. |

The client journal records signed intent before transmission. Retries preserve its bytes, sequence, deadline, and boundary.
Arrival time does not extend a deadline. Receipt loss can hide partial or complete acceptance.
B0 does not prove that every agent still uses B0. Permanent client loss does not require another owner message for release.

## Candidate matrix

Each candidate needs a separate assessment of confidentiality, release authority, persistence, and retrieval.
The external-custodian rows describe requirements, rather than supported product capabilities.

| ID and candidate | Verified capability or accepted direction | Renewal and retained copies | Remaining assumption or blocker |
| --- | --- | --- | --- |
| C1: fixed tlock capsule | tlock v1.2.0 encrypts for a selected round; strict mode rejects another chain [S1]. | An observer retains the original ciphertext and wrapper. Later check-ins cannot move its decryption round. | Required beacon, complete bundle, and compatible recovery client remain available. Threshold compromise can disclose early [S2]. |
| C2: independent agents | ADR 0012 accepts three receipts for confirmation and two shares for release. | Each agent enforces accepted state. Public shares remain available to an observer permanently. | Two reachable, honest agents retain durable state and bounded clocks. Production model and review remain pending. |
| C3: external threshold custodians | Candidate requirement: independent operators retain private material and enforce authenticated renewable state. No provider has verified this contract here. | Every custodian must reject stale state and refuse renewal after release of its share for that content key. | Provider selection, exact protocol version, custody, rollback policy, and release evidence remain unresolved. Drand's fixed-round capability does not verify renewable custody. |
| C4: externally executed contract plus custody | Ethereum contracts execute transaction-defined rules [S4]. Solidity v0.8.30 exposes a block timestamp; contract data remains public [S5, S6]. | Public ordered state could record renewals. Observers retain old ciphertext and all public contract data. | A reviewed private custodian must obey authoritative state. Execution, transaction inclusion, custody, publication, and ciphertext retrieval must survive owner-host loss. |

C3 is a research candidate without a selected implementation. This report claims no external service supports its complete contract.
C4 identifies an execution candidate, not a complete release mechanism. No contract, compiler, network account, or custody integration receives approval.
No new cryptographic primitive closes either gap in this report.

## Fault matrix

“Conditional” means the listed surviving dependencies are necessary. It does not mean that a production implementation has passed review.
“No bound” means this comparison establishes no recovery-time guarantee under that fault.
Independent operators include the relevant agents, custodians, or beacon operators, rather than the owner alone.

| Case | C1: fixed tlock | C2: independent agents | C3: external custodians | C4: contract plus custody |
| --- | --- | --- | --- | --- |
| F1: main server disappears | Conditional on independent bundle and beacon access. | Accepted target: release by Bmax + E with two qualifying agents. | Conditional on independent state, custody, publication, and retrieval. | Conditional on independent execution access, custody, publication, and retrieval. |
| F2: all owner-operated hosts disappear | Conditional on copies and beacon access outside those hosts. | Conditional on two qualifying agents outside those hosts. Co-location defeats this condition. | Conditional on sufficient external custodians and independent ciphertext. | Conditional on external execution, sufficient external custody, and independent ciphertext. |
| F3: all relevant independent release operators disappear | No bound before the required beacon exists. A retained valid beacon permits later offline recovery. | No bound before sufficient shares become public. Retained sufficient shares can permit recovery. | No bound without sufficient retained release material. | Public execution state alone cannot recover a privately held key after all custodians disappear. |
| F4: receipts disappear; owner client disappears permanently | Fixed release ignores check-ins. Visitor needs an independent client and complete recovery inputs. | Uncertain outcome spans B0 through Bmax, with E separate. Two qualifying agents need no further owner message. | Requires defined acceptance and uncertainty rules. Client silence cannot establish rejection. | A transaction can succeed without client confirmation. Requires explicit authoritative-state and uncertainty rules. |
| F5: stale state and observer copies persist | Original bundle remains decryptable at its original round. Deletion and key rotation do not recall it. | Review must prevent rollback and renewal after share release. Count old shares across all histories. | Requires durable monotonic state and an explicit rollback policy. Count every historical disclosure. | Custody must reject stale or non-authoritative execution state. Public old ciphertext and previously disclosed keys remain usable. |
| F6: network or clock assumptions fail | Missing beacon or unreachable copies delay recovery. Wall-clock arrival alone supplies no signature. | No Bmax + E guarantee without two reachable, honest agents and reviewed clock/publication bounds. | No bound without specified custody reachability and clock assumptions. | Execution outage, censorship, or unavailable custody prevents the proposed path. Block time alone establishes no UTC publication bound. |

F3 distinguishes loss before release from loss after sufficient material becomes public.
An observer's complete recovery inputs can outlive every operator. A hash or a public release decision alone is insufficient.
All candidates require a client that survives main-server loss. Permanent owner-client loss differs from loss of every visitor and recovery client.

## Reproducible cases and limits

The following cases use synthetic values. They establish consequences of the stated assumptions, rather than complete protocol correctness.

| Case | Inputs and event order | Required conclusion |
| --- | --- | --- |
| R1: retained fixed bundle | Observer copies C1 at round r. Owner later requests r+100. The signature for r becomes available. | The original copy still targets r. A later request cannot revoke it. |
| R2: lost receipts | B0=100; issued Bmax=200. Agents durably accept any subset. Responses disappear. One agent may remain unavailable. | The second surviving release boundary lies in [100,200]. Confirmation requires all three matching receipts. |
| R3: complete receipt loss | All agents accept 200; client receives no receipt and disappears. | Actual agent authority can be 200 despite B0=100. Exact release at B0 is not the accepted contract. |
| R4: accumulated shares | Observer retains agent A's released share. A later response hides that share; observer later obtains B's share for the same key. | Two shares remain sufficient. Current responses cannot erase earlier disclosure. Renewal after release requires rejection. |
| R5: missing ciphertext | Visitor obtains sufficient valid key material but no complete ciphertext copy survives. | Key release does not establish recovery. |
| R6: public contract secret | A proposed C4 mechanism puts its content key in contract state marked private. | Public visibility defeats confidentiality [S5]. Separate private custody remains necessary. |

Reproduce R2 with this command from the repository root:

```sh
python3 - <<'PY'
from itertools import product
cases = confirmed = 0
for accepted in product((False, True), repeat=3):
    for received in product((False, True), repeat=3):
        if any(r and not a for a, r in zip(accepted, received)):
            continue
        for unavailable in (None, 0, 1, 2):
            boundaries = [200 if a else 100 for i, a in enumerate(accepted)
                          if i != unavailable]
            release = sorted(boundaries)[1]
            assert 100 <= release <= 200
            if all(received):
                assert release == 200
                confirmed += 1
            cases += 1
assert (cases, confirmed) == (108, 4)
print(f'{cases} cases; {confirmed} confirmed cases')
PY
```

This enumeration assumes exact clocks and immediate publication, so E=0 only inside this arithmetic example.
It excludes restart, rollback, delayed delivery, multiple renewals, cryptography, and accumulated shares. R4 records an excluded review obligation.
It neither replaces #27's model nor establishes production protocol acceptance.
R1, R3, R4, R5, and R6 are explicit review traces, not executed cryptographic experiments.

## Primary source audit

Retrieve date for every source below: 2026-11-05 UTC.
Versioned source URLs identify the evaluated version. Unversioned pages identify documentation snapshots on that date, rather than adopted implementations.

| ID | Primary source and version | Capability checked |
| --- | --- | --- |
| S1 | [tlock.go, tlock v1.2.0](https://github.com/drand/tlock/blob/v1.2.0/tlock.go) | `Network`, `Strict`, `Encrypt`, `Decrypt`, and signature verification in `TimeUnlock`. |
| S2 | [drand timelock documentation, dated snapshot](https://docs.drand.love/docs/timelock-encryption/) | Fixed-round signature release; threshold compromise and network-stop limitations. |
| S3 | [quicknet announcement, 2023-10-16](https://docs.drand.love/blog/2023/10/16/quicknet-is-live/) and [chain info, dated response](https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/info) | Chain identity, public key, scheme, genesis, period, and group hash. |
| S4 | [Ethereum contract introduction, dated snapshot](https://ethereum.org/en/developers/docs/smart-contracts/) | Transaction-driven contract execution, public contracts, gas, and external-data limitations. |
| S5 | [Solidity v0.8.30 security considerations](https://docs.soliditylang.org/en/v0.8.30/security-considerations.html#private-information-and-randomness) | Public visibility includes state marked `private`. |
| S6 | [Solidity v0.8.30 global variables](https://docs.soliditylang.org/en/v0.8.30/units-and-global-variables.html#block-and-transaction-properties) | `block.timestamp` denotes the current block timestamp in Unix seconds. |

The existing [compatibility report](../tlock-compatibility.md) records the full quicknet public key and historical library pins.
The [adapter report](../tlock-production.md) records the root module's later dependency selection. Those reports describe different module graphs.
This comparison adds no dependency and makes no new claim about adapter readiness.

The current S3 response matches all six identity fields in `tests/compatibility/tlock/testdata/quicknet-info.json`.
The chain hash is `52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971`.
The scheme is `bls-unchained-g1-rfc9380`; genesis is `1692803367`; period is three seconds.
Reproduce the identity check with a bounded read:

```sh
curl -fsSL --max-time 20 --max-filesize 65536 \
  https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/info \
  | python3 -c 'import json,sys; a=json.load(sys.stdin); b=json.load(open("tests/compatibility/tlock/testdata/quicknet-info.json")); fields=("public_key","period","genesis_time","hash","groupHash","schemeID"); assert all(a[k]==b[k] for k in fields); print("quicknet identity: six fields match")'
```

An identity match does not establish future availability, current operator independence, or timely publication of every future round.

## Contracts for the next reports

| Consumer | Required inputs from this report | Required distinct output |
| --- | --- | --- |
| #47: deadline enforcement | C1–C4; F1–F6; B0, Bmax, E; R1, R3, R4, R6. | Identify execution authority, renewal order, custody, release trigger, clock policy, and unresolved implementation version. Distinguish verified capability from a proposed mechanism. |
| #48: ciphertext availability | Recovery inputs; F2, F3, F5, F6; R1 and R5. | Identify independent copies, retention, discovery, integrity, access, and a compatible client. Distinguish available ciphertext from available key material. |
| #49: cost and trust | Candidate IDs and fault IDs; #47's execution/custody dependencies; #48's persistence/retrieval dependencies. | Account separately for operator trust, censorship, execution cost, storage cost, renewal cost, and recovery access. State unsupported bounds and unpriced dependencies. |

Each report keeps its own evidence and scope. Candidate IDs do not imply selection or approval.
Issue #50 owns the eventual synthetic proof or documented blocker and the maintainer decision before dependency adoption.
This report supplies comparison inputs without waiting for production switch acceptance.

## Verification scope

The R2 enumeration passes 108 cases, including four confirmed cases. The current quicknet identity check passes six fields.
The review traces above cover retained copies, irreversible disclosure, receipt loss, permanent client loss, and absent ciphertext.
Source review checks each named capability against the primary documents above.
No product behavior, public API, dependency, or user interface changes. TDD, pre-patch behavior tests, and product E2E tests do not apply.
This report supplies no Go coverage result. The existing 95.00 percent product gate remains unchanged at the batch integration gate.
The report starts no service, purchases no resource, uses no real secret, and enables no production switch.
