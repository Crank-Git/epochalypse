# Independent protocol review

Issue #28 reviews commit `ad9a5eb` on 2026-11-05.
The reviewer did not implement the model in #27.
This report supplies a proposed decision for the maintainer. It records no production acceptance.

## Result

The revised model supports the conditional deadline argument below.
It supplies useful evidence for implementation research under ADR 0012.
It does not establish a complete production protocol.
Keep switch production use disabled.

ADR 0012 accepts three independent agents, three matching durable receipts for confirmation, and two release shares.
The accepted product tradeoff includes uncertainty approaching 24 hours with the default interval.
Two colluding agents can disclose content early.
One unavailable agent can prevent confirmation while two surviving agents permit release.
These accepted limits require no new product decision.

## Review results

| Contract | Result | Evidence and limit |
| --- | --- | --- |
| Durable intent precedes transmission | Pass in the model | `ClientJournal.Issue`, `UncertainModel.Transmit`, and `TestJournalIncludesDurableIntentBeforeTransmission`. Persistence copies values only. |
| Exact retry and increasing sequence | Pass in the model | `TestRestartRetriesExactJournaledBytes`, `TestRejectsDelayedOlderIntentAfterLaterAcceptance`. Authentication and byte decoding remain assumptions. |
| Lost receipts preserve uncertainty | Pass in the model | `TestLostFinalReceiptsReportUncertaintyAfterCompleteAcceptance` checks all eight receipt subsets. Three matching receipts advance B0. |
| Each outage permits bounded recovery | Conditional pass | `TestReleasesEveryTwoIntentScheduleByBmaxPlusAllowance` checks 110,592 cases. The bound depends on the assumptions below. |
| Fast clocks prevent early disclosure | Conditional pass | `TestConservativePolicyRejectsFastClockEarlyPublication` checks real UTC 109 against B0=110. Absolute clock accuracy remains external. |
| Public history remains irreversible | Pass in the model | `TestRetainsReleasedSharesAfterRestartCancelAndRenewal` retains observer shares and rejects later renewal. |
| Release and renewal serialize | Pass in the model | `TestRenewalAndReleaseRaceKeepsIrreversibleBoundary` checks both serialized orders at the boundary. No concurrent store or server exists. |
| Ordinary restart preserves clock safety | Not established | `FaultModel.Restart` resumes a complete snapshot on the same synthetic elapsed timeline. No real process or host restart runs. |
| Stale backups cannot restore authority | Not established | `RestartAgent` receives the stale flag from its caller. `TestQuarantinesStaleRestartWithoutRecallingObserverShares` tests the response, not detection. |
| Exact authenticated wire protocol | Not established | `rules.md` specifies byte order and domains. No encoder, decoder, signature verifier, or wire fixture executes them. |
| Complete switch lifecycle | Not established | `prepare` and `arm` have message proposals only. Cancellation and replacement lack complete distributed contracts. |

The recovery test name above refers to the two-intent enumeration in `uncertain_test.go`.
The old `rules`, `boundaries`, `faults`, and `replacement` reports remain historical evidence.
Their B0-only counterexamples remain valid against the former contract.
The revised predicate does not repair that former contract.

## Clock and publication argument

Let t denote real UTC, U a clock sample, and epsilon the asserted absolute UTC error limit.
Require `t-epsilon <= U <= t+epsilon` at initialization and every sample.
Set the sampled lower bound to `U-epsilon`.
Then `t-2*epsilon <= L <= t`.
Exact elapsed projection preserves this interval between samples.
The maximum of that projection and the next sampled lower bound preserves the interval.

An agent releases only when L reaches its accepted release boundary.
Therefore, an honest agent cannot release before that boundary under these assumptions.
Every accepted boundary comes from durable issued intent and remains at most Bmax.
At real UTC `Bmax+2*epsilon`, each intact surviving agent has L at least Bmax.
The journal requires nondecreasing boundaries, so these accepted boundaries also remain at least the initial B0.
After later confirmation, three matching receipts establish the new common lower boundary, subject to intact durable state.

Let P bound the entire interval from mathematical eligibility to public recovery material at the observer.
P includes the next sample, scheduling, publication, endpoint access, ciphertext retrieval, and observer retrieval.
The proposed conditional allowance is `E=2*epsilon+P`.
At least one surviving endpoint must supply authenticated ciphertext independently of the owner and main server.
If the deadline includes actual plaintext recovery, P must also cover authenticated reconstruction and decryption.
The current `ContentAvailable` predicate checks material availability, not decryption.

The enumeration covers two issued intents, 64 delivery/persistence/receipt states per intent, and each one-agent outage.
It uses every integer offset within epsilon zero, one, and two.
Each case uses one shared fixed offset across agents; separate tests exercise selected clock steps.
Publication values range from zero through two and derive from the schedule number.
The tests do not independently enumerate every publication value for every schedule.
They sample completion at the assumed bound. They do not measure publication work.
This finite evidence supports the conditional argument, rather than a universal implementation proof.

The review accepts no numeric production epsilon, P, or E.
`ClockReading.Safe` asserts readiness. It does not establish absolute accuracy.
The twice-epsilon comparison detects inconsistent samples under the model assumptions. It cannot detect every incorrect clock.
An unsafe clock or quarantined agent removes a potential publisher.
Reachability and honesty alone therefore cannot establish the bound without the reviewed clock and intact-state conditions.

## Concrete implementation obligations

### Clock recovery

The production design must define sample frequency, clock-source independence, leap-second treatment, suspend behavior, and an accuracy assessment with bounded age.
It must account for elapsed-clock drift and time quantization in its lower bound.
Exact synthetic elapsed seconds cannot serve as the implementation's accuracy evidence.

Ordinary process restart needs a fresh clock assessment and an explicit lower-bound recovery rule.
Host restart also needs a new elapsed origin.
One candidate re-anchors from a freshly assessed `U-epsilon` while retaining durable release and unsafe-state evidence.
That candidate requires a separate proof and executable schedules before acceptance.
It must neither project across incompatible elapsed origins nor clear irreversible release history.
Readiness must block mutation and new disclosure until the recovery rule establishes safety.
Any recovery delay must fit P or remove that agent from the conditional availability claim.

A second research candidate uses only each freshly assessed `U-epsilon`, without elapsed projection.
Under the same absolute error assumption, each sample remains between `t-2*epsilon` and t.
This candidate removes the exact continuous elapsed premise from the deadline argument.
It still needs bounded sampling within P, fresh assessment after restart, and durable irreversible latches.
Review backward samples, cutoff behavior, restart, and the publication race before choosing this candidate.
No candidate has acceptance through this report.

Go 1.27.3 omits monotonic readings from serialized `time.Time` values.
Its documentation also warns that some monotonic clocks stop during sleep.
Consequently, ordinary serialization cannot implement the model's continuous elapsed timeline.
See the [versioned Go source](https://github.com/golang/go/blob/go1.27.3/src/time/time.go#L54-L68).

### Durable state and rollback

Persist accepted state, durable sequence, release history, and clock safety evidence before a receipt or public share escapes.
Use one serialized transaction order for renewal, release, and restart recovery.
Test crashes before commit, after commit, before response, and after disclosure with real process restarts.
A failed disclosure response must still preserve the durable fact that disclosure might have occurred.

A stale backup can contain a consistent old sequence and an old private-share flag.
The backup cannot prove its own freshness.
Specify a freshness authority outside the restored snapshot and its failure behavior.
Quarantine must cover startup before either receipts or shares become available.
Tests must restore stale backups after a confirmed renewal and after a lost share response.
They must check durable refusal to renew the same content key after any possible disclosure.
Public observers retain shares even when local history or an endpoint loses them.

### Authenticated bytes and journal identity

The candidate format specifies fixed integer widths, lengths, ordered arrays, message domains, and rejection rules in `rules.md`.
Implement versioned positive and negative fixtures for every proposed operation.
Bind the verified signed bytes to their decoded state, signer, enrolled agent keys, content key, ciphertext digest, and protocol version.
The model's `SignedIntent` accepts synthetic bytes and decoded state as an asserted association.
It cannot detect a mismatch that a real verifier must reject.

On client restart, validate the journal's identity and configuration against the requested switch before transmission or receipt reconciliation.
Match each returned receipt to an exact issued request and the enrolled agent identity.
Persist receipt evidence before the client reports confirmation.
Test another switch's journal, substituted bytes, conflicting receipts, stale configurations, corrupt journals, and receipts reordered across multiple intents.
Keep the same signed bytes and times for retries. Arrival time must never extend authority.
Specify timeout, body, nesting, and total message limits before allocation or signature work.

### Arm, renewal, cancellation, and replacement

Define durable prepared and armed states, their receipts, and the precise meaning of successful arm.
Distribute and verify three encrypted shares and independent ciphertext copies before activation confirmation.
Define partial arm, lost arm responses, permanent client loss, and restart at each transition.
An arm certificate must bind the same content, enrollment, protocol version, and accepted state at every agent.

Define the renewal cutoff using the reviewed clock policy.
`CheckIn` rejects at or after its effective local release boundary, which can trail real UTC under the conservative policy.
Confirm that the future implementation and owner status use this exact contract.
Retain the configured interval and grace limits at the authenticated boundary.
Test renewal just before, at, and after the cutoff, with release in each serialized order.

`Cancel` currently stops local transmission only. Already in-flight requests can still arrive.
Specify which distributed states cancellation changes and which receipts report its uncertain result.
Cancellation cannot recall public shares or promise destruction of observer copies.

Replacement cannot treat a new agent identity as a fresh threshold slot for the same content key without analysis.
Previously disclosed shares remain available to observers.
Specify enrollment changes, retired shares, old exports, and transition failure schedules before replacement exists.
If replacement requires a fresh key and ciphertext, define the relationship to old independently releasable content.
Do not claim that an old key becomes secret again.

### Maintained threshold library

The module already pins `github.com/drand/kyber v1.3.3` for other dependencies.
The repository is not archived and reports activity on 2026-11-05.
This is maintenance evidence, not a security audit or adoption decision.

The [v1.3.3 source](https://github.com/drand/kyber/blob/v1.3.3/share/poly.go) exposes these exact APIs:

```go
NewPriPoly(group kyber.Group, t int, s kyber.Scalar, rand cipher.Stream) *PriPoly
(*PriPoly).Shares(n int) []*PriShare
RecoverSecret(g kyber.Group, shares []*PriShare, t, n int) (kyber.Scalar, error)
(*PriPoly).Commit(b kyber.Point) *PubPoly
(*PubPoly).Check(s *PriShare) bool
```

These APIs share and recover a group scalar. They do not establish an arbitrary content-key encoding.
Before selection, review the group, scalar representation, key derivation, randomness, authenticated share envelopes, and malformed-share handling.
Verify every two-agent combination reconstructs the intended key and one share supplies no recovery path.
Review public commitments for the chosen secret representation before disclosure.
No implementation or library adoption occurs in this review.

Primary evidence, retrieved 2026-11-05:

- [Go 1.27.3 time source](https://github.com/golang/go/blob/go1.27.3/src/time/time.go), lines 54–68: sleep and serialization limits.
- [Kyber v1.3.3 source](https://github.com/drand/kyber/blob/v1.3.3/share/poly.go), lines 56, 95, 141, 180, and 404: the signatures above.
- [Kyber v1.3.3 release](https://github.com/drand/kyber/releases/tag/v1.3.3) and [repository metadata](https://api.github.com/repos/drand/kyber): version and maintenance evidence.

## Proposed maintainer decision

Accept the revised synthetic model as conditional research evidence under ADR 0012.
Retain the symbolic bound `E=2*epsilon+P` with its explicit assumptions.
Keep production use disabled and record no numeric allowance.
Require the concrete obligations above before a production protocol review can pass.

The next engineering step compares the fresh UTC-only lower bound with a restart-aware elapsed projection.
If the design retains elapsed projection, model bounded elapsed error and an explicit re-anchor operation.
Then specify and test durable rollback detection, authenticated wire fixtures, and complete lifecycle contracts.
These are engineering requirements, rather than a request to reconsider the accepted receipt-loss tradeoff.
If a proposed implementation requires a weaker availability promise, request an explicit owner decision before changing the spec.

Separate protocol algorithms from their future implementation and deployment profile.
Clock recovery, lifecycle transitions, and authenticated byte contracts need complete algorithm definitions before protocol acceptance.
Real stores, transport, and process tests then verify those definitions during implementation.
The deployment profile supplies supported numeric epsilon and P for the selected hosts and endpoints.
This review chooses no hosts and adds no availability promise outside the honest-agent, intact-state, bounded-clock assumptions.

AC-switch-protocol-8 needs the maintainer's separate recorded decision after this review.
AC-switch-protocol-11 has an explicit conditional formula here, but lacks supported production clock limits and measured publication bounds.
The maintainer must preserve that distinction when recording the result.

## Reproduce this review

Run `go test ./internal/switchprotocol -count=1 -cover` with the repository's local cache paths.
Inspect `transition.go`, `faults.go`, `journal.go`, `uncertain.go`, and their tests at commit `ad9a5eb`.
No source behavior changes in this issue. A pre-patch behavior test does not apply.
Package coverage measures the research model only. It does not replace the project's aggregate 95.00 percent gate.
The epic integration pull request runs the full required suite.
The review run passes with 99.7 percent package coverage on 2026-11-05.
`git diff --check` reports no whitespace error.
