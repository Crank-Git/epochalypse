# Recovery after main-server loss

Issue #27 tests FR-switch-protocol-7 with synthetic recovery schedules under ADR 0012.
Production switch use remains disabled until #28 records an accepted review.
The revised model supplies recovery material by Bmax plus E under explicit synthetic assumptions.
The old experiment still fails recovery at the owner's confirmed release boundary after some partial renewals.
Its original assertion was: "AC-switch-protocol-7 therefore remains unmet as a supported outage guarantee."
That assertion concerns the former B0-only requirement. ADR 0012 supersedes that requirement.
The old traces and tests remain unchanged. The revised predicates have separate tests below.

## Former experiment contract

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

## Former baseline evidence

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

## Former partial-renewal counterexample

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

## Former assumptions and limits

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

## Revised uncertainty model

`ClientJournal` stores complete synthetic durable values before transmission.
Each `SignedIntent` contains the exact signed bytes and their decoded state.
The caller asserts authentication and the association between those values.
The model supplies no signature, canonical encoding, transport, or disk store.
An unsuccessful persistence step leaves no intent available for transmission.
An issued intent contributes to Bmax even when no agent receives it.
`Restart` copies the complete journal and receipt evidence into a new client instance.
`Retry` returns an independent copy of the original bytes, sequence, deadline, and boundary.
The model rejects empty bytes, a different content identity, nonincreasing sequences, and decreasing boundaries.
The model rejects boundary arithmetic overflow.

`UncertainModel` applies the journal with a separate conservative clock policy.
The historical local constructors and fault explorers retain their original clock policy.
Its executable schedules reside in the same package as the model state.
Each delivery separates volatile acceptance, durable state, and receipt delivery.
A receipt requires persistence first. A lost receipt changes no confirmed client evidence.
Three distinct matching durable receipts advance B0.
Later issued intents keep the outcome uncertain until their matching receipts arrive.
An exact retry can obtain a lost receipt without extending its signed deadline.
A delayed older sequence cannot overwrite a later accepted sequence.
A changed byte or deadline cannot use a journal entry as transmission authority.
`Transmit` retains an exact synthetic network copy before permanent client loss.
An in-flight request can still reach an agent after that loss. Its receipt cannot recreate the client.
The model rejects a new transmission after client loss.

The complete-acceptance test covers all eight receipt-delivery subsets.
Seven subsets report B0=110 through Bmax=210 with `Confirmed=false`.
All three delivered receipts report B0=Bmax=210 with `Confirmed=true`.
The agent states remain identical across those receipt outcomes.
An unsent durable intent also reports 110 through 210.
With no delivery, two survivors can publish at B0.
These outcomes explain why an incomplete response proves neither success nor definitive failure.

## Revised outage evidence

`TestReleasesEveryTwoIntentScheduleByBmaxPlusAllowance` enumerates 110,592 schedules.
Each schedule combines two independent 64-state renewal experiments and one of three permanent agent outages.
Each combination tests nine clock cases: epsilon zero, one, or two, with every integer offset from minus epsilon through epsilon.
The schedules check zero public shares at real UTC 109, before the initial B0=110.
The client issues deadlines 200 and 300 after the initial deadline 100, with grace 10.
Each request can disappear, lose volatile acceptance, lose its durable receipt, or deliver its durable receipt at each agent.
Every stage includes an agent crash and a new live instance from the complete durable value.
The client then disappears permanently. The main server supplies no operation or recovery material.
Each surviving agent has an independent ciphertext copy.
The observer obtains two shares by Bmax plus the assumed allowance in every schedule.

### Conservative clock policy

This section records the conditional #27 experiment in `NewUncertainModel`.
The proposed restart contract instead selects `NewSampledModel` from issue #106.
See [the candidate clock profile](clock-profile.md) for its proof, numeric ceilings, and readiness contract.
The old constructor and its counterexamples retain their original semantics.

The earlier 12,288-case experiment at `87cfc47` uses nonnegative clock lag only.
It does not establish a symmetric clock-error guarantee.
With B0=110 and tolerance one, UTC 110 at real UTC 109 permits early publication under the historical rule.
`TestConservativePolicyRejectsFastClockEarlyPublication` reproduces this failure before the revised implementation.
The historical rules and their B0-only counterexamples remain unchanged.

Let epsilon bound absolute UTC error in both directions at initialization and every later sample.
For real time t and sample U, assume t-epsilon <= U <= t+epsilon.
`Safe` asserts external readiness. It neither measures nor proves that inequality.
The revised policy uses L=U-epsilon as a conservative UTC lower bound.
At each later sample, it takes the greater of this lower bound and the prior lower bound plus exact elapsed time.
Complete synthetic snapshots retain this lower bound and the continuous elapsed timeline across restart.
A sample difference above twice epsilon latches an unsafe clock.
Backward elapsed time and unrepresentable arithmetic also latch an unsafe clock.
The constructor rejects an unrepresentable initial lower bound or twice-epsilon allowance.

Under these assumptions, each sampled lower bound satisfies t-2*epsilon <= L <= t.
Exact elapsed projection preserves both inequalities between samples.
The maximum of two such lower bounds preserves both inequalities.
Release requires this lower bound to reach the accepted release boundary.
Therefore, release cannot precede that boundary in real time.
At real time Bmax+2*epsilon, every intact surviving agent has a lower bound at least Bmax.
Every accepted boundary is at most Bmax because delivery uses durably issued intent.
Thus each surviving agent can publish by that time, subject to the publication allowance below.
This argument assumes every accepted boundary remains at least B0. The journal permits only nondecreasing boundaries.

Let P bound all time from mathematical eligibility through public share availability.
P includes the next clock sample, scheduling, publication work, endpoint access, and observer retrieval.
The schedules sample once at the assumed completion time; they do not measure these operations.
With two surviving honest agents and independent ciphertext access, E=2*epsilon+P bounds completion at Bmax+E.
`Allowance.UTCError` represents epsilon. `Allowance.ReleaseBy` checks both additions and the multiplication by two.
No numeric production E has review acceptance.

The conditional argument requires these assumptions:

- Each reachable honest agent retains complete accepted state and irreversible release history.
- Each sample satisfies the absolute UTC error bound, including samples after restart.
- Elapsed time measures real elapsed seconds exactly on one continuous synthetic timeline.
- Two endpoints complete publication and retrieval within P after mathematical eligibility.
- At least one endpoint supplies an authenticated ciphertext copy without owner or main-server access.

The finite experiment uses fixed offsets and publication values from zero through two seconds.
Separate tests use forward and backward UTC steps within the asserted bound across each agent outage.
They check the release boundary before, at, and after real UTC 110, and completion by UTC 114 for epsilon two.
Other tests check unsafe samples, elapsed rollback, lower-bound underflow, allowance overflow, and stale-state quarantine.
These cases support the conditional argument. They do not enumerate every clock, network, or implementation history.
An external readiness assertion cannot establish accuracy, exact elapsed time, polling limits, or real persistence.
Issue #28 owns independent review of these assumptions under AC-switch-protocol-11.

### Fresh UTC samples after restart

`TestFreshEpochPublishesWithoutContinuousElapsed` uses the existing crash, restart, and publication operations.
The test first runs through a factory that delegates to `NewUncertainModel` at commit `e296f26`.
At UTC 114, its reset elapsed sample fails with `clock is unsafe`.
`TestFreshSamplesIgnoreElapsedRate` also fails there with `clock exceeds modeled tolerance`.
These failures exercise the old behavior with compatible inputs, rather than a missing symbol.
The revised factory selects fresh UTC samples and both tests pass.

`TestSampledEpochsReleaseEveryTwoIntentOutageSchedule` checks 12,288 schedules with two issued intents and each permanent agent outage.
Offsets change from fast to slow between samples. Elapsed values reset and change rate without time authority.
Each schedule checks no new public share before real UTC 110.
After permanent client loss, each schedule supplies recovery material within the stated sample gap.
The model still assumes independent ciphertext copies. It performs no cryptography or disk operation.

Every decision requires a new externally assessed sample.
Absent or expired assessments use `Safe=false` and persist the unsafe latch before output.
Restart preserves accepted state and both irreversible latches.
Known stale state causes quarantine without replacing known authority with the initial snapshot.
An observer retains every prior share across restart and quarantine.
The model assumes external freshness validation before the first trusted sample after restore.
Issue #107 specifies that validation boundary. This clock model detects no whole-witness rollback.
Neither a later sample nor ordinary restart clears quarantine.

## Irreversible history and quarantine

Publication persists the release latch before the observer receives a share.
An agent that ever publishes refuses every later renewal for that content key.
The same rule applies after ordinary restart and at a renewal/release race.
Cancellation stops local renewal attempts. It does not recall shares or prevent eventual disclosure from remaining copies.
The observer retains shares after release, restart, cancellation, quarantine, and later renewal attempts.
One retained share can combine with a later share even when no current endpoint displays the first share.

`RestartAgent` quarantines a snapshot that the caller identifies as stale.
Quarantine blocks new renewal and publication. It preserves observer evidence.
The model assumes an external rollback detector; it implements no detector or trusted hardware.
The original undetected-rollback counterexample remains in [faults.md](faults.md).
Unsafe clocks and stale-state quarantine can defeat availability outside the intact-state and reviewed-clock assumptions.
Synthetic snapshots establish neither disk durability nor real process restart behavior.
The model cannot verify ciphertext authentication, key reconstruction, failure independence, or transport availability.

## Maintainer decision

ADR 0012 accepts bounded uncertainty and supersedes the former exact-confirmation requirement.
It does not approve a production protocol.
Issue #28 owns independent review, AC-switch-protocol-8, AC-switch-protocol-11, and the explicit maintainer decision.
That review must retain the old failed property, the revised evidence, and the production restriction.

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

`TestJournalIncludesDurableIntentBeforeTransmission` fails against the initial stub with Bmax=110 instead of 210.
`TestRestartRetriesExactJournaledBytes` checks exact retry values and independent byte copies.
`TestRetriesPreserveSignedDeadlineAcrossArrivalAndRestart` checks delayed arrival, duplicate receipt retrieval, and UTC 209/210/211.
`TestLostFinalReceiptsReportUncertaintyAfterCompleteAcceptance` checks all final-receipt subsets.
`TestIncludesUnsentIntentAndPermitsReleaseAtB0` checks durable unsent intent and early disclosure within the uncertainty interval.
`TestRetainsReleasedSharesAfterRestartCancelAndRenewal` checks public history and the release latch.
`TestQuarantinesStaleRestartWithoutRecallingObserverShares` checks external quarantine and retained evidence.
`TestRejectsDelayedOlderIntentAfterLaterAcceptance` checks out-of-order sequence delivery.
`TestRenewalAndReleaseRaceKeepsIrreversibleBoundary` checks both serialized orders at the release boundary.
The rejection tests cover conflicting bytes, missing persistence, unavailable agents, absent intents, unsafe initial clocks, and overflow.
