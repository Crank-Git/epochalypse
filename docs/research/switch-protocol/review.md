# Independent protocol review

Issue #28 reviews integration commit `3c9fbdb9d5f033b31c59841b339c6337aadd5743` on 2026-11-05.
The reviewer did not implement the producers in #27, #106, or #107.
This report records the independent review and the separate maintainer decision. Production use remains disabled.

## Result

The maintainer accepts the reviewed protocol design for subsequent implementation under ADR 0012 after the protocol batch gate.
The design selects the fresh UTC policy in [clock-profile.md](clock-profile.md) and the version-two contracts in [production-contracts.md](production-contracts.md).
The independent review finds no architectural contradiction under their explicit assumptions.
The engineering review passes. The [separate maintainer decision](https://github.com/Crank-Git/epochalypse/issues/28#issuecomment-6664941171) records the acceptance scope on 2026-11-05.
Keep switch production use disabled until implementation and deployment verification also pass.

The accepted product tradeoff includes uncertainty approaching 24 hours with the default interval.
Three distinct agents supply matching durable receipts for confirmation. Two distinct shares permit recovery.
Two colluding agents can disclose content early. A compromised preparation client can disclose the complete key.
One unavailable agent can prevent confirmation while two eligible survivors permit release.
These limits follow ADR 0012 and require no new product decision.

The prior review of `ad9a5eb` accepted conditional research only.
It identified exact elapsed time, unspecified freshness authority, authenticated bytes, and incomplete lifecycle contracts as design gaps.
Issues #106 and #107 now supply concrete algorithms for these gaps.
They supply synthetic evidence, rather than production services or host certification.

## Review results

| Contract | Result | Evidence and limit |
| --- | --- | --- |
| Durable intent precedes transmission | Pass | `ClientJournal.Issue` and `TestJournalIncludesDurableIntentBeforeTransmission`; real journal durability remains an implementation obligation. |
| Exact retry and increasing sequence | Pass | `TestRestartRetriesExactJournaledBytes`, `TestRejectsDelayedOlderIntentAfterLaterAcceptance`, and `TestBindsJournalBytesToStateAndEnrollment`. |
| Lost receipts preserve uncertainty | Pass | `TestLostFinalReceiptsReportUncertaintyAfterCompleteAcceptance` checks all eight receipt subsets. Bmax retains unsent durable intent and in-flight requests. |
| Each outage permits bounded recovery | Pass under explicit premises | `TestSampledEpochsReleaseEveryTwoIntentOutageSchedule` checks 12,288 histories with changing offsets and reset elapsed epochs. |
| Clock safety survives new epochs | Pass | `sampledEligibility` uses only the current `U-epsilon`; `TestFreshEpochPublishesWithoutContinuousElapsed` and `TestFreshSamplesIgnoreElapsedRate`. |
| Missing assessment blocks authority | Pass | `TestSampledRestartRequiresFreshAssessment` blocks receipts and publication and retains the unsafe latch. `Safe` remains an external assertion. |
| Numeric allowance has explicit limits | Pass as a candidate design profile | epsilon=2 seconds, S=5 seconds, W=25 seconds, P=30 seconds, E=34 seconds. Actual readiness remains unverified. |
| Restore validates external authority first | Pass as an algorithm | `production-contracts.md`, “Durable authority outside restored archives”; `TestQuarantinesBeforeReceiptsOrShares` checks exact head equality. |
| Persistence precedes irreversible output | Pass as an algorithm | The persistence table covers each crash point. `TestLifecyclePersistsBeforeOutput` checks logical commit order, not disk flushes. |
| Distinct enrolled keys control confirmation | Pass | `admitEnrollment` validates distinct signature keys and canonical recipients. `confirm` derives keys only from that admitted value. |
| Canonical signed bytes bind state | Pass as a format contract | `TestPreservesVersionTwoByteVector`, `TestVerifiesEveryMessageDomain`, and `TestRejectsAlteredSignedInput`. Nested certificate adapters remain implementation obligations. |
| Maintained libraries support the representation | Pass as compatibility research | Vault v1.21.4 reconstructs a 32-byte key from each pair. age v1.3.2 restricts encrypted shares to their recipients. |
| Complete lifecycle retains old authority | Pass as a design | Prepared agents never release. Partial arm reports uncertainty. Cancellation freezes renewal, and replacement requires a fresh switch and key. |
| Public history remains irreversible | Pass | `TestSampledRestartCountsEveryRetainedShareSubset` covers all eight observer subsets. `TestCancelAndReplacementPreserveOldAuthority` preserves release after cancellation. |

The distinct-key repair matters to the threshold claim.
Three identity labels with one signature key cannot represent three independent receipt authorities.
The admission tests reject repeated signature keys and recipients before confirmation or encrypted distribution.
The certificate adapter must derive the admitted digest and records from the same verified canonical certificate.
Distinct keys alone do not prove separate operators, providers, credentials, storage, or failure domains.

## Clock and publication argument

Let t denote real UTC, U the current integer UTC sample, and epsilon the absolute error limit.
Every decision requires `t-epsilon <= U <= t+epsilon` after assessment age, capture delay, and quantization.
Set `L=U-epsilon` for that decision only. Then `t-2*epsilon <= L <= t`.
Release requires L to reach the accepted release boundary B.
Thus an honest agent cannot disclose a new share before B under these premises.
No prior sample or elapsed tick supplies release authority.

Every accepted B comes from durable issued intent and remains at most Bmax.
The client requires nondecreasing boundaries, so accepted authority remains at least the initial B0.
Three matching durable receipts establish the later confirmed B0 at every agent, subject to intact state.
An uncertain later renewal can move some agents toward Bmax without confirmation.
This distinction preserves the accepted receipt-loss semantics.

At real UTC `Bmax+2*epsilon`, every surviving agent's next trusted sample has L at least its accepted B.
Let S bound the time until that sample. Let W bound subsequent publication and independent recovery work.
Set `P=S+W` and `E=2*epsilon+P`.
Each survivor must complete within those bounds; the slower survivor determines completion.
No owner client, main server, or inter-agent exchange supplies a release dependency.
The observer needs both shares and independently retrievable authenticated ciphertext and manifest.

The proposed profile sets epsilon=2 seconds, S=5 seconds, and W=25 seconds.
W assigns five seconds to schedule and public share work and 20 seconds to authenticated recovery, including decryption.
The resulting candidate is P=30 seconds and E=34 seconds.
Its assessment age ceiling is 60 seconds. Its capture-to-use ceiling is 250 milliseconds.
The error budget includes 500 milliseconds of base uncertainty, 100 parts per million of rate uncertainty, and downward integer quantization.
The illustrative sum is `0.5+0.0001*(60+0.25)+0.25+1=1.756025` seconds.
Its remaining margin is 0.243975 seconds. Any additional uncertainty consumes that margin or prevents readiness.

These numbers define a profile to validate. They approve no actual VPS, reference clock, scheduler, storage device, or endpoint.
The chrony reference bound assumes correct reference time; authentication and synchronization status do not independently establish that premise.
After restart, suspend, source loss, a clock step, or an ambiguous epoch, require the profile's fresh assessment.
Validate restored authority and irreversible latches before that assessment.
Include intact-state replay and assessment within S. An unlimited restart delay violates the availability premise.

An unsafe or quarantined survivor cannot support the conditional availability claim.
Reachability and honesty alone cannot establish the bound without intact state and the reviewed clock and work limits.
This design preserves safety by refusing new authority or disclosure when readiness fails.
It makes no claim that a clock source or finite measurements prove universal accuracy.

## Evidence scope and preserved counterexamples

The 12,288 sampled histories cover two issued intents, 64 fault schedules per intent, and each one-agent outage.
The schedules include permanent client loss, changing offsets, and reset elapsed epochs.
They establish material availability at assigned times, rather than execute reconstruction or measure real publication work.
The candidate-profile test checks 780 combinations of offsets, sample gaps, and assigned recovery work.
The isolated compatibility module separately executes pairwise reconstruction, recipient isolation, signature checks, and authenticated-content rejection.
These finite checks support the argument under its premises. They are not a universal implementation proof.

The earlier 110,592-case enumeration remains evidence for the historical exact-elapsed experiment in `NewUncertainModel`.
The proposed design instead selects `NewSampledModel`. Its UTC-only lower bound removes that elapsed premise.
The original `rules`, `boundaries`, `faults`, and `replacement` reports retain their meaning.
Their old-confirmed-boundary counterexamples remain valid against the former contract.
This review changes no trace, original assertion, or model source.

## Implementation and deployment obligations

The implementation must verify the accepted algorithms through concrete adapters:

- Decode and authenticate the canonical enrollment certificate and all nested prepared receipts against pinned identities.
- Enforce product time limits, local clock cutoff, exact journal identity, operation phase, sequence, and replay rules together.
- Use authenticated HTTPS with pinned endpoint identities, bounded requests, explicit timeouts, and bounded response bodies.
- Implement the separate per-agent authoritative journal and its complete commit and checkpoint order.
- Test process termination before commit, after commit, before response, and after possible disclosure.
- Test flush errors, locking, directory persistence, incomplete records, checkpoint repair, stale archives, and lost share responses.
- Persist possible disclosure and unsafe state before output. Retain both latches through restart and restore.
- Verify exported recovery material before arm. Show partial arm as potentially disclosive and potentially unavailable.
- Verify cancellation receipts without recall claims. Give replacement a fresh switch, enrollment, key, ciphertext, and recovery bundle.
- Complete a current security and license review before production library adoption.

The authority resides on a separate durable volume outside ordinary archive restores.
Startup validates the complete committed head before receipts, shares, or clock assessment.
A missing or inconsistent head quarantines the agent. An intact head can reconstruct the disposable checkpoint after an ordinary restart.
Common rollback of both the whole witness and its state remains outside the intact-state promise.
A second file inside the same restored snapshot cannot detect that fault.
Each survivor requires its own accessible authority; this contract introduces no shared network witness.

The deployment must establish source correctness, source independence, supported leap behavior, bounded assessment age, and capture delay.
It must verify S and each W component through the observer's independent path under the supported fault envelope.
It must verify independent providers, credentials, storage, endpoints, and restore procedures.
No runtime or host evidence exists in these research fixtures.
These checks verify the design during implementation and deployment; they do not require completed services before the design decision.
Live promotion remains a separate owner-approved step.

## Maintainer decision and criteria

The [maintainer decision](https://github.com/Crank-Git/epochalypse/issues/28#issuecomment-6664941171) accepts this design for subsequent implementation under ADR 0012 after the protocol batch gate.
It selects fresh conservative UTC samples with candidate ceilings epsilon=2 seconds, S=5 seconds, W=25 seconds, P=30 seconds, and E=34 seconds.
The recorded assessment and publication contract remains a prerequisite. The general formula remains `E=2*epsilon+S+W`.
The decision requires authenticated canonical enrollment and intent, distinct agent keys and recipients, and irreversible release history.
Durable state and freshness validation outside restored backups precede trusted clock samples.

Runtime persistence, transport, process recovery, actual-host clock and publication evidence, and deployment verification remain required before production use.
The decision approves no real host, service, domain change, or production enablement.
It preserves the old exact-confirmation counterexamples as historical evidence.

AC-switch-protocol-11 is met by the explicit candidate limits, conditional argument, validation contract, and reproducible evidence in this report.
It does not certify a deployment or claim measured production bounds.
AC-switch-protocol-8 is met by the separate recorded maintainer decision linked above.
Design acceptance permits implementation after the batch gate. Production use remains disabled pending the later verification gates.

## Reproduce this review

Use Go 1.27.3 and worktree-local GOCACHE, GOMODCACHE, GOPATH, GOTMPDIR, and XDG_CACHE_HOME paths under ignored `var/`.
Keep `var/go.mod` as the module fence for downloaded source.

```sh
go mod download
go test ./internal/switchprotocol -count=1 -cover
go -C tests/compatibility/switch-contract mod download
go -C tests/compatibility/switch-contract test -count=1 -race -cover ./...
go -C tests/compatibility/switch-contract vet ./...
go -C tests/compatibility/switch-contract build ./...
git diff --check
```

This report changes no behavior, test, public runtime interface, or browser flow.
Test-first and pre-patch behavior checks do not apply to this report.
Package coverage measures research helpers only. It does not replace the aggregate 95.00 percent gate.
The epic integration pull request runs the full required suite.
The producer reports one unexplained countdown focus failure before later successful checks. The PM retains that integration verification limitation.

Primary interfaces independently checked on 2026-11-05:

- [Vault v1.21.4 Shamir source](https://github.com/hashicorp/vault/blob/v1.21.4/shamir/shamir.go): raw-byte shares and pairwise reconstruction.
- [age v1.3.2 API](https://pkg.go.dev/filippo.io/age@v1.3.2): recipient parsing, canonical representation, encryption, and decryption.
- [chrony 4.9 manual](https://chrony-project.org/doc/4.9/chronyc.html): reference-dependent clock error, frequency uncertainty, and synchronization limits.

The compatibility checks exercise pinned APIs. This review adopts no production dependency or clock service.

The earlier review also verifies Kyber v1.3.3 scalar-share signatures in its [versioned source](https://github.com/drand/kyber/blob/v1.3.3/share/poly.go).
`NewPriPoly`, `Shares`, `RecoverSecret`, `Commit`, and `Check` operate on group scalars.
That alternative supplies no selected arbitrary-key representation. The concrete compatibility evidence above uses Vault's raw-byte API.

The independent rerun passes with 99.7 percent model coverage and 99.0 percent compatibility-helper coverage.
The compatibility race tests, vet, and build pass. Test references and local report links resolve.
`git diff --check` reports no whitespace error.
