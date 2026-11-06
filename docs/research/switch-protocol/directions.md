# Directions for a usable switch

Date: 2026-11-05.
Status: research recommendation. No replacement protocol has production acceptance.

## Finding

The earlier report does not establish that useful dead man's switches are impossible.
It establishes a contradiction between two obligations under the listed faults.
Those obligations make release depend on receipt delivery to an owner client that can disappear permanently.

The owner now requests research into a direction that permits progress.
This report proposes explicit changes for review. ADR 0005 and the production gate remain unchanged.

## A: retain independent agents and report an interval

This is the recommended next research direction. It changes the receipt-loss promise rather than the release mechanism.
Keep three independently operated agents, three durable renewal receipts, and two release shares.

Define these values:

| Value | Meaning |
| --- | --- |
| B0 | The last release boundary confirmed by three receipts. |
| Bmax | The latest release boundary in any owner-signed renewal already issued for transmission. |
| E | The measured allowance for clock error and publication work. |

Before transmission, the client records the exact signed request and its proposed boundary durably.
Retries transmit the same sequence, bytes, and boundary. Arrival time never extends the deadline.
An agent accepts only increasing, authenticated states within the supported time range.

After three matching receipts, the client reports the new confirmed boundary.
After a lost or incomplete response, the client reports uncertainty between B0 and Bmax.
The client must not report that the attempted renewal failed or that the old boundary still controls every agent.
The owner must continue to treat B0 as the earliest possible disclosure boundary.

The proposed release promise is:

> After main-server loss and one agent outage, the surviving agents can release by Bmax plus E.
> This requires two reachable agents with intact durable state and clocks within the accepted error bound.

For an uncertain renewal, release can occur before the proposed new boundary.
It can also occur after the last confirmed boundary. The interface must state both possibilities.
This is the exact change that removes the earlier final-response contradiction.

### Example

The last confirmed release boundary is Monday at 12:00 UTC.
The owner sends a renewal through Tuesday at 12:00 UTC, but the response disappears.
The interface reports Monday through Tuesday as the possible release interval, plus the stated clock/publication allowance.
After permanent owner-client loss, the agents do not need another owner message to release.

A 24-hour renewal can therefore introduce almost 24 hours of uncertainty.
That cost needs explicit acceptance. A short grace period does not remove this uncertainty.
Smaller extension steps could reduce it, but would require more confirmed steps and a separate review.

### Why this direction is plausible

Assume each surviving agent holds either B0 or the requested B1, with B1 greater than B0.
Each independently releases its share at its own accepted boundary.
Any two surviving agents release by B1, regardless of which receipts reached the owner.
After three confirmed durable updates, every agent uses B1.

This is an elementary deadline argument, not a proof of a complete protocol.
It assumes no prior irreversible release invalidates the renewal.
An agent that has released a share must refuse renewal of that content key.
The review must include observers who retain old shares, not only currently visible agent responses.

### Reproducible limited check

The following enumeration passes 108 acceptance, receipt, and outage combinations.
Four combinations have complete owner confirmation. The other cases include partial updates and lost receipts.

```python
from itertools import product

for accepted in product((False, True), repeat=3):
    for received in product((False, True), repeat=3):
        if any(r and not a for a, r in zip(accepted, received)):
            continue
        for unavailable in (None, 0, 1, 2):
            boundaries = [200 if a else 100
                          for i, a in enumerate(accepted) if i != unavailable]
            release = sorted(boundaries)[1]
            assert 100 <= release <= 200
            if all(received):
                assert release == 200
```

This check excludes storage rollback, clock faults, message ordering, concurrent renewals, cryptography, and old public shares.
It supplies evidence for the changed deadline predicate only. It does not replace the protocol gate.

## B: use a replicated decision as the release authority

This alternative gives all agents one committed boundary through an established consensus implementation.
The client reports an unknown outcome after a timeout and queries the decision later.
An unavailable client does not change the committed result.

[Raft section 8](https://raft.github.io/raft.pdf) describes committed requests whose responses do not reach clients.
Request identities and cached results support safe retries; they do not guarantee response delivery.
[etcd's API guarantees](https://etcd.io/docs/v3.6/learning/api_guarantees/) explicitly distinguish commitment from client knowledge.

This changes the original receipt-based promise too.
A committed renewal can delay release even when the owner never receives its confirmation.
A release decision also needs a working, communicating majority and a reviewed clock policy.
This adds operational complexity without preserving the original exact-confirmation guarantee.

Ordinary etcd leases are not a direct substitute for the release timer.
[Its failure documentation](https://etcd.io/docs/v3.6/op-guide/failures/) says leader changes and recovery extend lease timeouts.
Our inference: such leases do not provide the required independent UTC release bound without additional design.
If this option proceeds, model explicit absolute deadlines and irreversible release decisions separately from lease expiry.

## C: add an optional fixed external release boundary

An independently retained tlock bundle can supply a fixed fallback date.
It could permit recovery after every owner-operated host disappears, if ciphertext and the required beacon remain available.

[drand documents](https://docs.drand.love/docs/timelock-encryption/) fixed-round decryption and its threshold and network-availability assumptions.
The existing capsule implementation already uses the verified fixed-time mechanism.

Once a decryptable future bundle is public, later check-ins cannot recall an observer's copy.
Therefore the fallback date cannot move indefinitely for the same published content.
Rotating keys does not revoke old ciphertext that contains the same secret.
This option is an explicit maximum release date, not a renewable switch replacement.

## Comparison

| Direction | Useful property | Accepted cost | Recommendation |
| --- | --- | --- | --- |
| A: independent agents | Main loss does not become a necessary release dependency. | A lost response produces a visible, bounded release interval. | Model this next. |
| B: committed decision | One ordered authoritative state. | Client uncertainty remains; release needs a working majority. | Keep as an alternative. |
| C: fixed tlock fallback | Potential recovery after all owner-operated hosts disappear. | The published fallback date is not revocable. | Optional separate feature. |

These are design assessments, not properties established by a shipped switch implementation.

## Work that can proceed independently

The current operations issue #40 groups finished features with the blocked switch deployment.
That dependency comes from the chosen issue scope, not a DNSSEC or capsule requirement.

Split a deployment slice for the main service, DNSSEC experiments, capsules, and countdowns.
Keep agent shares, live-agent restoration, switch recovery drills, and switch-specific readiness behind the protocol gate.
Use the existing no-purchase and no-domain-change limits until the owner supplies deployment values.

The comparison research in #46 can also proceed without an accepted production switch protocol.
It should compare the assumptions and failure cases rather than inherit the implementation gate.
This report recommends those dependency changes; it does not silently relabel existing issues.

## Next protocol gate

The owner accepts direction A's explicit receipt-loss tradeoff. ADR 0012 records the decision.
The issued spec defines separate confirmed, earliest-possible, and latest-issued boundary assertions.
Run the revised model and independent protocol review before production switch use.

The model must cover:

- Lost requests, partial acceptance, lost receipts, and permanent owner-client loss.
- Main loss plus each possible unavailable agent, without inter-agent coordination as a release dependency.
- Delayed delivery, duplicate retries, multiple issued requests, sequence conflicts, and signed time limits.
- Clock error, restart, stale backups, and quarantine after rollback.
- Already released shares, accumulated observers, renewal cutoffs, and irreversible release races.
- Independent ciphertext retrieval and bounded publication work.

Define E from explicit clock and service assumptions. Do not invent an exact UTC availability guarantee.
Run independent protocol review before enabling production switches.
Implementation still requires all mandatory tests to pass and at least 95 percent first-party Go statement coverage.

## Verification scope

Primary sources were retrieved on 2026-11-05.
The report reuses the existing contradiction and verifies the limited 108-case deadline enumeration above.
No product code, dependency, deployment, approved requirement, or production gate changes in this research step.
No new project-wide test run is necessary for this report; it makes no new product test-pass claim.

## Owner acceptance and current status

On 2026-11-05, the owner states: “I like the recommendation, plus it gets us moving again”.
The owner accepts direction A and its possible almost-24-hour uncertainty under the default interval.
[ADR 0012](../../adr/0012-switch-uncertainty.md) records this product tradeoff and supersedes ADR 0005.
The recommendation above describes the research assessment before owner acceptance.
The Bmax definition and next gate reflect the accepted direction. The historical counterexamples remain unchanged.
B0 is the earliest possible disclosure boundary after incomplete receipts, not proof of actual authority.
Bmax includes every signed renewal already issued for transmission, recorded durably before transmission.
The issued spec now requires the revised model, independent review, and explicit clock and publication assumptions.
No numeric E or production protocol has acceptance. Production switch use remains disabled.
The historical counterexamples still refute the former exact-confirmation contract; they do not automatically refute the revised bound.
Completed-feature deployment and comparative research can proceed independently. Live switch operations remain gated.
