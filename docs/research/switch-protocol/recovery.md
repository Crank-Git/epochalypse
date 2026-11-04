# Recovery after main-server loss

Issue #27 tests FR-switch-protocol-7 with synthetic recovery schedules.
Production switch use remains disabled until #28 records an accepted review.
The candidate supports baseline recovery under the assumptions below.
It fails recovery at the owner's confirmed release boundary after some partial renewals.
AC-switch-protocol-7 therefore remains unmet as a supported outage guarantee.
Successful tests detect that failure. They do not approve the protocol.

## Experiment contract

`ExploreRecoveryFaults` consumes the `FaultModel` transitions from #26.
It repeats all 64 renewal schedules with each of the three agents unavailable.
The result contains 192 schedules in schedule order, then outage order.
Digits retain the meaning in [faults.md](faults.md):

| Digit | Request | Presumed durable update | Receipt at owner |
| --- | --- | --- | --- |
| 0 | Lost | No | No |
| 1 | Accepted | Crash loses update | No |
| 2 | Accepted | Yes | Lost |
| 3 | Accepted | Yes | Delivered |

Each agent resumes its presumed durable snapshot after the renewal stage.
The selected agent then remains unavailable for both observations.
The main server remains absent throughout recovery.
The model has no main-server operation, endpoint, ciphertext copy, or share source.
Its owner evidence records receipts delivered before main-server loss.
That evidence remains available to the experiment for comparison.
Recovery does not call the owner or main server.

`RecoveryScenario.Ciphertext` states which agents hold independently accessible ciphertext copies.
The initial experiment assigns a copy to each agent.
An unavailable agent supplies neither a new share nor its ciphertext copy.
`RecoveryEvidence.ContentAvailable` requires two distinct public shares and at least one surviving ciphertext copy.
This predicate represents available recovery material. It does not decrypt content.
The experiment tests all eight ciphertext assignments separately.
If only the unavailable agent holds ciphertext, two public shares do not permit recovery.

`Retained` supplies authenticated public shares from earlier history for the same content.
The observer rejects mismatched identities or content metadata.
The caller must establish that this earlier history exists.
The experiment does not infer publication from a receipt or a changed deadline.
The observer retains earlier public shares across renewals, model restarts, and agent outages.
An unavailable agent's earlier public share can still contribute to the threshold.

## Baseline evidence

The initial state has sequence 2, deadline 100, and grace 10.
Clock samples use exact UTC with zero tolerance.
Schedule 0 delivers no renewal request.
At UTC 109, neither surviving agent publishes.
At UTC 110, both surviving agents publish.
Each one-agent outage leaves two public shares and two ciphertext copies.
Recovery material is therefore available without the main server at the confirmed release boundary.

The complete-renewal schedule delivers all three durable receipts for deadline 200.
It confirms the new release boundary of 210.
Each one-agent outage permits recovery at UTC 210, but not at UTC 209.
Tests also check one second after both boundaries.

## Partial-renewal counterexample

The renewal proposes sequence 3, deadline 200, and grace 10 at UTC 101.
At UTC 110, 141 schedules retain owner deadline 100 without sufficient recovery material.
The count excludes the three schedules whose owner confirms deadline 200.
Every result exposes the confirmed state and each agent's accepted state separately.

Schedule 10, outage 2, executes this trace:

1. Agent zero accepts deadline 200 and persists it.
2. Its receipt disappears.
3. Agent one accepts deadline 200 and persists it.
4. Its receipt disappears.
5. Agent two misses the request and retains deadline 100.
6. All agents resume their presumed durable snapshots.
7. The main server disappears.
8. Agent two becomes unavailable.
9. At UTC 110, agents zero and one remain ineligible.
10. The observer holds zero shares and two accessible ciphertext copies.
11. The owner evidence still shows deadline 100 and release boundary 110.
12. At UTC 210, both surviving agents publish.

The same renewal with outage zero or one supplies only one share at UTC 110.
All three outages therefore defeat recovery at the old confirmed boundary in this renewal state.
With exact clocks, every enumerated schedule eventually supplies recovery material at UTC 210.
This later observation proves no bound relative to the old confirmed boundary.
A test changes the proposed deadline to 1000000 and preserves the same failure at UTC 110.
The candidate imposes no maximum proposed deadline.

## Assumptions and limits

The experiment assumes honest agents and authenticated inputs.
Two colluding agents can disclose content early.
Each agent needs an independent failure domain, public endpoint, state, and ciphertext copy.
A common provider, credential, or disk invalidates this independence assumption.
The surviving endpoints must permit public access without the main server or owner authentication.
The model asserts ciphertext availability. It does not test network retrieval or content authentication.

The exact synthetic clocks satisfy the proposed local transition rule.
They establish no supported production UTC error bound.
An unsafe clock blocks new publication and can defeat recovery availability.
The clock review in [boundaries.md](boundaries.md) remains necessary.
Snapshots preserve complete values on one synthetic elapsed timeline.
They do not prove real storage durability or process restart behavior.
The stale-backup counterexample in [faults.md](faults.md) remains unresolved.
This experiment excludes Byzantine tolerance and loss of every agent.

## Maintainer decision

The maintainer must decide how to revise the protocol to meet the confirmed-boundary guarantee.
The tentative-update and global-decision proposal in [faults.md](faults.md) remains a research direction.
Its delivery, maximum uncertainty, and rollback rules require a new model and independent review.
This report neither changes the promise nor implements that proposal.
Issue #28 must retain the failed property and the production restriction in its decision.

## Reproduce the evidence

Run `go test ./internal/switchprotocol -count=1 -cover`.
`TestRecoversBaselineAfterMainLossAndEachAgentOutage` checks the three baseline outages.
`TestDetectsRecoveryDelayAcrossEveryRenewalState` checks all 192 schedules and the 141 delayed results.
`TestRequiresSurvivingCiphertextForRecovery` checks all eight assignments of ciphertext copies.
`TestRetainsPublicSharesFromTheUnavailableAgent` checks irreversible public history.
`TestChecksConfirmedAndLocalRecoveryBoundaries` checks both boundaries with exact synthetic clocks.
`TestPreservesRecoveryDelayForALaterProposedDeadline` preserves the delay with a larger proposed deadline.
`TestRejectsInvalidRecoveryScenarios` checks rejected clocks, sequences, and public evidence.
The initial stub fails the schedule-count and invalid-evidence assertions before implementation.
This pure model adds no user interface or browser flow.
