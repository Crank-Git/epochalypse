# Partial renewal and retained public shares

Issue #26 supplies executable evidence for FR-switch-protocol-5 and FR-switch-protocol-6.
Production switch use remains disabled until #28 records an accepted review.
The current candidate fails a release guarantee at the owner's confirmed release boundary.
The tests detect counterexamples. Their success does not prove protocol safety.

## Experiment contract

`NewFaultModel` creates three synthetic agents and one persistent public observer.
`Confirmed` returns the owner's last state with three matching durable receipts.
`Accepted` returns all three local states, including unavailable agents' last volatile states.
`CheckIn` separates local acceptance, presumed persistence, and receipt delivery.
It rejects receipt delivery without persistence.
Lost receipts do not change the owner's confirmed state.
Receipt evidence for different states cannot combine into a confirmation.
`Crash` makes one agent unavailable.
`Restart` resumes its presumed durable snapshot.
`Publish` checks local eligibility before it calls `Observer.Observe`.
An ineligible agent publishes nothing. An unsafe clock publishes nothing.
`ReleaseAvailable` reports the cumulative public threshold.

The model presumes an atomic durable copy of the complete `ModelAgent` value.
That copy includes the accepted state, effective UTC, elapsed sample, tolerance, unsafe latch, and release latch.
`CheckIn` copies these facts when its persistence parameter is true, including after a rejected transition.
`Publish` copies these facts before any public disclosure, including an unsafe-clock result.
A crash before that copy loses the volatile update.
A crash after that copy preserves the update even when its receipt disappears.
Snapshots use values. They share no mutable reference with the live agent.

These copies assert durability for the experiment. They do not implement a durable store.
Elapsed samples continue on one synthetic timeline across model restarts.
A real process resets its monotonic clock origin and needs a separately reviewed resume mechanism.
The model does not reconstruct an agent with `NewModelAgent` after a crash.
It therefore preserves presumed durable latches during an ordinary model restart.
The rollback parameter deliberately restores the initial snapshot without detection.
This adversarial operation exposes the consequence of a missing rollback control.

`ClockReading.Safe` remains an external assertion.
The tolerance parameter measures no independent UTC accuracy.
The experiment assumes authenticated messages and valid synthetic shares.
It supplies no signatures, storage, transport, threshold cryptography, or production agent.
Two colluding agents can disclose content early.
A common provider, credential, or disk violates the independence assumption.

## Exhaustive bounded schedules

`ExploreRenewalFaults` enumerates 64 schedules in numeric order, from 0 through 63.
Each agent has four stages:

| Digit | Request | Presumed durable update | Receipt at owner |
| --- | --- | --- | --- |
| 0 | Partition loses request | No | No |
| 1 | Agent accepts request | Crash loses volatile update | No |
| 2 | Agent accepts request | Yes | Response disappears |
| 3 | Agent accepts request | Yes | Yes |

Agent zero uses the lowest two bits. Each subsequent agent uses the next two bits.
For each agent, the experiment applies its stage, crashes it, resumes it, and attempts publication.
The clock samples describe each agent's local history independently.
No agent receives another agent's state.
Every agent resumes before its publication attempt.
This experiment enumerates one renewal and one publication attempt per agent.
It does not exhaust all possible distributed histories.

The test starts with sequence 2, deadline 100, and grace 10.
At UTC 101, sequence 3 proposes deadline 200 with the same grace.
At UTC 110, each agent attempts publication after restart.
Exactly one schedule confirms sequence 3 at the owner.
In 31 schedules, the owner retains deadline 100 but the public threshold remains unavailable at UTC 110.
Each result includes all three accepted states for comparison with the confirmed state.

Schedule 10 has digits 2, 2, 0 for agents zero through two.
Agents zero and one persist deadline 200. Both receipts disappear.
Agent two misses the request and retains deadline 100.
All three agents crash and resume their presumed durable snapshots.
At UTC 110, only agent two publishes a share.
The owner retains deadline 100, but the observer holds only one distinct share.
The other two agents become eligible at UTC 210 under exact clocks.
Increasing the proposed deadline increases this delay without a configured maximum.
The candidate therefore establishes no finite delay bound relative to the old confirmed boundary.
Issue #27 owns the separate main-server loss and one-agent outage evidence.

## Stale snapshot counterexample

`TestDetectsEarlyReleaseAfterStaleSnapshotRollback` executes this trace:

1. All three agents accept deadline 200 at UTC 101.
2. All three agents persist their accepted state.
3. All three receipts reach the owner.
4. The owner confirms deadline 200.
5. Agents zero and one crash.
6. Both agents resume the initial snapshot with deadline 100.
7. Both agents publish at UTC 110.
8. The observer reaches the public threshold before the confirmed release boundary of 210.

The baseline model supplies no external rollback detector.
This trace violates an early-release property under stale backup restoration.
Ordinary snapshot resume preserves both latches in `TestSnapshotPreservesReleaseAndUnsafeLatches`.
Those assertions establish the pure model assumption, not real persistence across process restarts.

## Retained public share trace

`TestRetainsPublishedShareAfterRenewalAndRestart` gives agent zero a shorter deadline at UTC 101.
Its accepted deadline and zero grace make it eligible immediately.
The model publishes that share through the eligibility check.
The other agents then accept deadline 200.
Agent zero crashes and resumes its initial snapshot through the explicit rollback fault.
At UTC 210, agent one publishes a second share.
The observer combines that share with agent zero's earlier public share.
Neither renewal nor rollback removes the earlier public copy.
The accepted states differ, so the owner never confirms this renewal.

The current release latch prevents an ordinary local renewal after publication.
A later local acceptance elsewhere cannot recall a public share.
Cancellation and agent replacement must preserve this same public history constraint.

## Revision for maintainer review

The current candidate cannot satisfy the displayed-boundary guarantee after partial renewal.
Review a protocol that separates tentative updates from a globally verifiable renewal decision.
Specify when an agent changes its release authority after that decision.
Specify the outcome when a decision or its proof reaches only some agents.
Require an explicit maximum uncertainty interval in the decision.
Require a rollback control outside a restored snapshot before an agent resumes release authority.
Keep public shares irreversible across every revision.
These proposals need a new executable model and independent review.
They do not approve a new protocol or relax the required property.

## Reproduce the evidence

Run `go test ./internal/switchprotocol -count=1 -cover`.
`TestEnumeratesPartialRenewalAndLostReceipts` checks all 64 results and the 31 delayed schedules.
`TestPreservesCompetingRenewalEvidence` checks conflicting concurrent requests and replay rejection.
`TestRejectsInvalidFaultScenarios` checks unsafe samples and rejected sequences.
`TestRejectsUnavailableAgentsAndInvalidFaultSteps` checks partitions, invalid identities, and impossible receipt delivery.
The initial stubs fail both schedule-count and public-retention assertions.
This source-only research change introduces no user interface or E2E flow.
The existing CLI E2E suite remains part of `make check`.
