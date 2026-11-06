# Replacement release protocol: first feasibility report

Issue #70 preserves the original guarantee and compares two replacement approaches.
This report supplies a concrete blocker, rather than an accepted replacement protocol.
ADR 0005 remains in force. Issue #28 still requires maintainer acceptance before production use.
The baseline model, its counterexamples, and parked PR #67 remain comparison evidence.

## Original guarantee

The original owner statement in [the spec](../../specs/spec.md#overview) is:

> outage shouldnt stop this as it undermines the entire deadman switch behavior.

The spec states:

> The initial guarantee covers main-server loss plus one unavailable agent.

The owner decision in ADR 0005 is:

> Keep the original guarantee; research a replacement protocol (recommended)

The direction on epic #23 states:

> Preserve the owner-confirmed release boundary after main-server loss plus one unavailable agent.

This report preserves that boundary. A later agent deadline does not replace it.
The [switch feature](../../specs/features/05-switches.md) also requires these behaviors:

> FR-switches-4: The interface reports renewal success only after three durable receipts.

> If renewal lacks a receipt, use the old deadline as the conservative boundary.

> FR-switches-6: The public recovery client reconstructs a key from two valid release shares.

The research contract therefore has two obligations:

- Without client-observed renewal confirmation, sufficient recovery material becomes public at the previous confirmed release boundary.
- After client-observed renewal confirmation, sufficient recovery material stays private until the new release boundary.

These obligations concern the deadline plus grace, not the deadline alone.
Exact clocks below remove clock uncertainty from the counterexample.
A separate clock allowance cannot resolve a receipt uncertainty that spans almost an entire check-in interval.

## State and fault assumptions

| State | Meaning in this report |
| --- | --- |
| Proposed | The owner signs a check-in with an increasing sequence and an issue time. |
| Prepared | An agent durably records tentative renewal evidence without final release authority. |
| Chosen or committed | The distributed protocol fixes its renewal decision under its own rules. |
| Durable | A state survives an ordinary process restart under the candidate's storage assumption. |
| Applied | An agent uses a decision to control its release authority. |
| Client-observed confirmation | The owner client receives and validates all three required durable receipts for the same renewal. |

A chosen decision need not reach every agent immediately.
A durable decision need not yet be applied.
Neither state proves that the owner client receives a response.
The report uses confirmation as a client event. It does not mean that a human reads the screen.

Both candidates assume honest agents, authenticated messages, and correct synthetic shares.
Each agent already holds its ciphertext copy and distinct share.
Two colluding agents remain outside the trust guarantee.
The release threshold remains two of three. Renewal confirmation still needs three durable receipts.
Both candidates require increasing sequences and a reviewed cutoff before the existing release boundary.
Neither candidate supplies a production encoding, cryptography library, or transport adapter.

The challenge permits these independent faults:

| Fault | Scope |
| --- | --- |
| Agent outage | Any one agent can remain unavailable through release. |
| Message loss | A request or response can disappear. No successful response delivery to the owner is guaranteed. |
| Client loss | The owner client can stop permanently after a request or receipt. No later status query is required for recovery. |
| Main loss | The main server can stop permanently. It supplies no release authority or recovery material. |
| Clock uncertainty | The concrete traces use exact UTC clocks. Production clock bounds remain unresolved. |
| Ordinary restart | Durable state survives. This does not permit restoration of an older backup as current state. |
| Stale backup | A separate adversarial fault can erase newer decisions. Neither source proves safety under this fault. |

The strongest blocker below permits reliable communication between the two surviving agents.
It needs neither an agent partition nor a clock fault.
Public recovery clients can reach those agents and already hold the recovery bundle.
An unavailable owner client does not mean an unavailable public recovery client.

## Primary sources

The following capability statements come from primary sources retrieved on 2026-11-04.
The release conclusions in subsequent sections are this report's analysis, not claims from those sources.

- [Lamport, Paxos Made Simple](https://lamport.azurewebsites.net/pubs/paxos-simple.pdf), 1 November 2001, sections 2.1–2.5 and 3.
  Majority acceptance chooses a value. A learner can miss that choice because messages disappear.
  Restart safety requires retained acceptor state. Progress needs successful communication with a majority and a suitable proposer.
  These properties do not establish a real-time release deadline or client receipt.
- [Gray and Lamport, Consensus on Transaction Commit](https://lamport.azurewebsites.net/video/consensus-on-transaction-commit.pdf), sections 2–4 and 6.4.
  This author-hosted version identifies the March 2006 publication and a 5 July 2017 correction.
  Two-Phase Commit can block after coordinator failure.
  Paxos Commit uses `2F + 1` coordinators and permits progress with `F + 1` working coordinators under timeliness assumptions.
  Its failure model retains state on stable storage. Its safety specification does not supply real-time progress bounds.
- [Ongaro, thesis chapter 6 source](https://github.com/ongardie/dissertation/blob/0c10ea1b53d73bf6d163bdde8e4141a1cbd8f36a/clients/clients.tex), section 6.3.
  The [thesis PDF](https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf) times out during this review.
  The author's chapter source supplies the verified evidence instead.
  A leader can commit a request and fail before its response reaches the client.
  Client identifiers, sequence numbers, and cached responses prevent duplicate execution on retries.
  These mechanisms require a later client interaction to resolve the missing response.

## Candidate A: replicated renewal decisions

Three agent services also hold a replicated decision log.
The main server does not hold a necessary consensus role.
The proposed renewal decision includes the complete signed state and evidence from all three agents.
Majority replication orders the decision; it does not lower the three-receipt confirmation requirement.
An agent applies only a verified decision from that log.
Tentative local requests alone cannot extend its release authority.

This sketch assumes durable consensus state and eventual communication between a surviving majority.
A deadline guarantee would additionally require measured bounds for elections, decision recovery, and publication.
The sources do not supply those bounds for this project.
Confirmation remains receipt of all three renewal receipts at the owner client, after the required decision and durable state.

Apply published schedule 10 before any complete decision evidence exists:

1. Agents zero and one persist tentative deadline 200. Their responses disappear.
2. Agent two receives no request and retains deadline 100.
3. The main server stops. Agent two becomes unavailable.
4. At UTC 110, the surviving agents resolve or abort the tentative request before public release.

This separates tentative state from release authority and can avoid the baseline's immediate local extension.
It is not a completed protocol: a decision cutoff and timely recovery still need proof.
If the surviving agents instead find a committed extension, they must preserve that decision.
Complete all three durable updates, then lose the final owner response: the owner still retains deadline 100.
At UTC 110, honoring committed deadline 200 delays recovery beyond the confirmed boundary.
Returning cached results helps a live client. It does not help a client that disappears permanently.

## Candidate B: explicit prepare and commit evidence

Each agent first persists a tentative proposal and signs prepare evidence.
A coordinator collects all three pieces and distributes a commit certificate for that exact state.
Only the commit certificate can change release authority.
The owner client confirms only after it receives all three required durable receipts for that committed state.
The owner client or an agent can coordinate. The main server cannot be the only coordinator.

This sketch assumes durable prepare evidence and authenticated certificates.
It needs an atomic decision rule for commit, abort, and the release cutoff.
Prepared evidence alone must not authorize a later deadline.
Ordinary Two-Phase Commit supplies no progress guarantee after its only coordinator fails.
Replicated coordinators through Paxos Commit can address that failure under the source's stated timeliness assumptions.
They do not prove the owner receives the outcome.

Apply the same published schedule 10:

1. Agents zero and one persist prepare evidence. Their responses disappear.
2. Agent two receives no request and retains deadline 100.
3. The main server stops. Agent two becomes unavailable.
4. At UTC 110, a safe abort can preserve release at the old boundary if no commit can exist.

If a prepared agent blocks instead, the surviving pair cannot meet the old boundary.
If agents use an unchecked abort timeout, a delayed valid commit can conflict with public release.
A reviewed decision rule must exclude that commit before it authorizes abort.

Even an ideal atomic commit rule retains the response problem.
Let all three agents prepare and durably apply the commit certificate.
Lose the final response before owner confirmation, then stop the owner client and main server.
Make any one agent unavailable. The surviving pair honors the new boundary and misses the old confirmed boundary.
If the client instead confirms before it distributes commit evidence, client loss can leave agents using the old boundary.
That order can disclose content before the client's newly confirmed boundary.
An extra client acknowledgment moves this uncertainty to another message; it does not establish client receipt at every agent.

## Product-valid times

The baseline's UTC seconds 100, 101, 110, 200, and 210 are arbitrary model times.
Its deadline proposal has no product interval validation.
It does not prove an unbounded product delay because the product limits intervals and grace.
The following example uses the default interval and grace within the allowed limits:

| Event | Exact UTC |
| --- | --- |
| Previous check-in issue time | 2026-11-04 00:00:00 |
| Previous confirmed deadline `D0` | 2026-11-05 00:00:00 |
| Grace `G` | 1 hour |
| Previous confirmed release boundary `B0 = D0 + G` | 2026-11-05 01:00:00 |
| Renewal issue time `T` | 2026-11-04 23:59:00 |
| Proposed deadline `D1 = T + 24 hours` | 2026-11-05 23:59:00 |
| New release boundary `B1 = D1 + G` | 2026-11-06 00:59:00 |

The interval is 24 hours, within 1 hour to 365 days.
The grace is 1 hour, within zero to 30 days.
The renewal occurs before both the previous deadline and release boundary.
Exact clocks accept its issue time without a future-time allowance.
The difference `B1 - B0` is 23 hours and 59 minutes.
Both candidates can have complete agent evidence before `B0` while the owner response disappears.
The contradiction therefore requires no interval beyond the product limits.

## Scoped contradiction: the final response

This argument applies to both sketches, including ideal atomic decisions and ideal stable storage.
Assume a useful renewal can complete before `B0` and extend the boundary to `B1 > B0`.
Assume owner receipt loss and permanent client loss remain allowed.
Assume the release obligations in this report both apply.

Construct a finite successful execution up to its client confirmation event.
Call the message whose delivery completes confirmation the final response.
For three receipts, it is the last required valid receipt.
Define two histories that differ only at that delivery:

| Event | History C: client confirms | History L: response disappears |
| --- | --- | --- |
| Agent protocol before final response | Same complete durable history | Same complete durable history |
| Final response at owner client | Client receives it and confirms `B1` | Network loses it; client retains `B0` |
| Subsequent client messages at agents | None arrive | None arrive |
| Owner client and main server | Both stop | Both stop |
| Agent two | Unavailable | Unavailable |
| Agents zero and one through `B0` | Same messages, durable state, clocks, and decisions | Same messages, durable state, clocks, and decisions |

Suppress any later client messages in both histories before they reach an agent.
The client can stop immediately; the network can also lose an acknowledgment already sent.
The surviving agents cannot observe the differing client receipt event.
For randomized protocols, fix the same random choices in both histories.
Public observers begin with fewer than two shares and receive identical agent outputs.

At `B0`, the surviving agents therefore expose the same recovery material in both histories.
History L requires two shares because `B0` remains the owner's confirmed boundary.
History C prohibits two shares because its confirmed boundary is `B1`.
The same public output cannot satisfy both requirements.
With product-valid times above, the conflict lasts until `B1`, almost 24 hours later.
Waiting for a client status query cannot meet the requirement after permanent client loss.

This is a contradiction within the listed assumptions, not a universal impossibility claim about release protocols.
It does not rely on the general impossibility of asynchronous consensus.
Reliable agent communication and perfect clocks do not distinguish these histories.
An additional assumption that excludes one history would change the research contract and needs explicit maintainer review.

## Other fault limits

| Challenge | Replicated decisions | Prepare and commit evidence |
| --- | --- | --- |
| Lost request or prepare response | Tentative state need not extend authority; safe abort and cutoff remain unproven. | Tentative state need not extend authority; prepared-state resolution remains unproven. |
| Lost final response plus client loss | Committed extension can remain unknown to the client. The scoped contradiction applies. | Complete commit evidence can remain unknown to the client. The same contradiction applies. |
| Main loss alone | Agent-held log avoids a necessary main role. No deadline bound follows automatically. | Independent coordination avoids a necessary main role. A sole failed coordinator can still block. |
| One agent outage | Two agents retain a majority, assuming timely communication and durable state. Renewal still needs three receipts. | A sole coordinator can block. Replicated coordinators need separate placement and progress assumptions. |
| Agent partition at release | Consensus safety does not provide a fixed publication time without a communication bound. | Neither unresolved prepare nor replicated commit supplies a fixed publication time without a communication bound. |
| Stale backup | Lost promises or log entries violate the stable-state premise. | Lost prepare, commit, or abort evidence violates the stable-state premise. |
| Clock fault | Ordered decisions do not prove UTC accuracy or restart clock safety. | Signed certificates do not prove UTC accuracy or restart clock safety. |
| Previously public shares | A later decision cannot recall them. | A later certificate cannot recall them. |

The [baseline rollback trace](faults.md#stale-snapshot-counterexample) remains relevant to both replacements.
Two restored agents can forget a confirmed extension and disclose two old shares before its release boundary.
Production needs a rollback control outside the restored snapshot or a safe procedure that removes release authority.
Removing authority can reduce availability. One outage plus another agent excluded for rollback exceeds the simple one-outage premise.
Neither candidate establishes safe cancellation, agent replacement, durable restart, or a production clock bound.

## Result and required decision

Neither candidate satisfies the precise contract under these fault and confirmation assumptions.
Candidate A improves decision consistency. Candidate B separates tentative evidence from final authority.
Both remain blocked by the final-response contradiction, before storage or cryptography implementation begins.
This first pass creates no service, production dependency, paid resource, or release route.

The maintainer must decide which premise, if any, can change while the owner preserves the guarantee:

1. Can an explicit transport or client assumption exclude the two final-response histories? State how it survives permanent owner client loss.
2. If no such assumption has acceptance, does this scoped contradiction require a new owner decision before further protocol work?

This report recommends no silent change to confirmation or the release boundary.
Treating a committed extension as confirmed would change the owner's requested contract.
Allowing release uncertainty after a missing receipt would also change that contract.
Both choices require an explicit owner decision rather than implementation discretion.
Keep #27 blocked and retain #28 as the acceptance gate.
Request maintainer review of this report before a larger implementation or any production use.

## Verification and review record

This is a documentation-only feasibility report. It adds no model code or product behavior.
TDD and a pre-patch behavior test do not apply because no executable behavior changes.
A new browser E2E spec does not apply because this report adds no user interface.
`make check` passes on 2026-11-04 with Go 1.27.3 and golangci-lint 2.15.0.
Unit, integration, CLI E2E, race, fuzz, format, vet, lint, and both Linux builds pass.
Aggregate first-party Go coverage reaches 401/410 statements, or 97.8049 percent.
The resolver suite remains outside the current `make check` contract, as [the check record](../../checks.md) explains.
These results verify the unchanged baseline, not a replacement implementation.

Reproduce the analysis by following histories C and L with the product-valid times above.
Compare each surviving agent's received messages and stored state before checking the conflicting obligations at `B0`.
The proof depends on the listed premises. A counterproposal must identify the premise it excludes and justify that exclusion.
Preserve the executable baseline with `TestEnumeratesPartialRenewalAndLostReceipts` and `TestDetectsEarlyReleaseAfterStaleSnapshotRollback`.
Those tests demonstrate the original candidate's failures; they do not validate either replacement sketch.
