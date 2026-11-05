# Candidate clock profile

Issue #106 proposes this engineering candidate for independent review.
It selects `NewSampledModel` for the proposed protocol contract.
`NewUncertainModel` retains the historical #27 experiment and its exact elapsed assumption.
No numeric profile, real VPS, clock service, or production switch has approval.

## Policy and proof

Let t denote real UTC at a decision and U its fresh integer UTC sample.
The caller establishes `t-epsilon <= U <= t+epsilon` for every decision.
This includes quantization, assessment age, and delay between capture and use.
`ClockReading.Safe` asserts that contract. It does not measure accuracy or sample age.
The model receives trusted samples only after external state and clock validation.
The model cannot distinguish a dishonest `Safe=true` from a valid assertion.

Use `L=U-epsilon` for this decision only.
Then `t-2*epsilon <= L <= t`.
Release requires L to reach the accepted release boundary B.
Thus no new share precedes B in real UTC.
Every accepted B comes from issued durable intent, so `B <= Bmax`.
At any trusted sample with `t >= Bmax+2*epsilon`, L reaches every accepted B.
This proof permits changing offsets, UTC steps within the error bound, arbitrary elapsed rates, and new clock epochs.
It projects no prior sample and assumes no exact process clock.

Let S bound the real time until the next trustworthy decision sample after that threshold.
Let W bound the subsequent schedule, publication, and recovery work.
Set `P=S+W` and `E=2*epsilon+P`.
With two honest reachable agents, intact state, and independent recovery material, completion occurs by `Bmax+E`.
W includes retrieval of both shares, authenticated ciphertext, reconstruction, and decryption if completion means plaintext recovery.
The current synthetic `ContentAvailable` predicate establishes material availability only.
The proof requires an assessed sample within S even after restart. An unlimited restart outage invalidates the bound.
An unready or quarantined survivor also invalidates that survivor's availability premise.
Safety remains fail-closed when the availability premise fails.

## Alternative with bounded elapsed drift

A projected bound could use `L0 + delta/(1+rho)` with downward arithmetic rounding.
Here delta measures elapsed ticks and rho bounds their maximum rate relative to real elapsed time.
This projection also needs a current epoch identifier, suspend rules, and proof of the rate bound throughout the interval.
After restart, a new trusted UTC assessment must replace the old projection.
Without those premises, elapsed ticks can advance too quickly or belong to another epoch.
The simpler UTC-only policy needs no projected elapsed bound and therefore selects fresh samples instead.
The lower bound can decrease between decisions; the durable release latch still prevents renewed authority after disclosure.

## Finite candidate ceilings

All durations below refer to real time. They are limits to validate, rather than measurements of this machine.

| Quantity | Candidate ceiling | Required evidence |
| --- | --- | --- |
| Effective absolute UTC error epsilon | 2 seconds | Account for source uncertainty, drift, capture delay, and integer quantization. |
| External assessment age | 60 seconds | Bound age independently of a stepped wall clock and reject expired evidence. |
| Capture-to-decision delay | 250 milliseconds | Discard the sample after this interval or any clock discontinuity. |
| Residual rate uncertainty | 100 parts per million | Establish a conservative bound, including clock correction behavior. |
| Base assessment uncertainty | 500 milliseconds | Include remaining system offset, root distance, and reference uncertainty. |
| Integer quantization | Less than 1 second | Round the UTC sample downward and account for the lost fraction. |
| S: next valid decision sample | 5 seconds | Include restart, assessment, scheduler delay, and the polling interval. |
| Schedule and public share work | 5 seconds | Include durable latch persistence before share output. |
| Recovery work | 20 seconds | Retrieve both shares and ciphertext, authenticate, reconstruct, and decrypt. |
| P | 30 seconds | Sum the sample, schedule, publication, and recovery budgets. |
| E | 34 seconds | Compute `2*2+30` with checked arithmetic. |

A sufficient illustrative error budget is `0.5 + 0.0001*(60+0.25) + 0.25 + 1 = 1.756025` seconds.
The two-second ceiling leaves 0.243975 seconds for additional bounded uncertainty.
Every omitted source of uncertainty consumes that margin or makes the candidate unready.
These chosen ceilings are engineering proposals. Reference algorithms do not guarantee them for a VPS.
The rate limit supports assessment-age accounting only. Release never projects elapsed ticks.

## Validation contract

Before a restored agent uses a clock sample, validate accepted state and irreversible release and unsafe flags.
Issue #107 owns the separate freshness and lifecycle contract.
A stale snapshot, missing witness, or uncertain latch enters quarantine before any trusted clock assessment.
Clock accuracy cannot establish durable-state freshness.
An intact known snapshot preserves accepted state and flags when a stale backup appears.
Whole-witness rollback remains outside this model's detection capability.

Obtain a fresh assessment after every process restart, host restart, suspend, and clock epoch change.
Invalidate cached assessments after an observed step, daemon restart, lost synchronization, or expired age.
Require an explicit leap policy and a common UTC interpretation before readiness.
During an unresolved leap event or incompatible smear, supply no trusted sample.
Measure age and capture delay with a validated epoch and bounded rate.
A reset, suspension, or ambiguous duration invalidates that age evidence.
An implementation must validate this mechanism; this synthetic model supplies no age detector.

Retain assessment provenance, reference time, uncertainty components, source identities, and the deployed clock implementation version.
Require independently assessed reference correctness and source independence.
Authentication alone does not prove correct reference time or symmetric network delay.
An operator must validate source faults and shared infrastructure against the trust assumptions.
Collect load, restart, suspend, step, source-loss, and recovery traces before proposing host support.
Measure S and every W component through the observer's independent recovery path.
Repeat under the supported failure envelope, including one agent outage and main-server loss.
Reject readiness if any ceiling lacks evidence. Finite measurements alone do not prove a universal bound.

## Primary references

Verified on 2026-11-05:

- [Go 1.27.3 time documentation](https://pkg.go.dev/time@go1.27.3), “Monotonic Clocks”: serialized time omits process-local monotonic data.
  The documentation also warns that some systems stop monotonic clocks during sleep.
- [RFC 5905](https://www.rfc-editor.org/rfc/rfc5905.html), sections 8 and 9: dispersion grows with a declared frequency tolerance.
  Its typical 15-parts-per-million parameter is an algorithm assumption, rather than a host certification.
- [chrony 4.8 chronyc manual](https://chrony-project.org/doc/4.8/chronyc.html), `tracking`: the reference-dependent bound combines system offset, root dispersion, and half root delay.
  `Skew` estimates frequency uncertainty. `Leap status` reports synchronization and leap state.
  A successful `waitsync` checks selected thresholds; it supplies no independent guarantee of reference correctness.

These references justify the evidence fields and age accounting, rather than the chosen numeric ceilings.
This change adopts no daemon, external library, provider API, or live clock configuration.

## Reproducible synthetic evidence

Run `go test ./internal/switchprotocol -run 'TestFresh|TestSampled|TestCandidate' -count=1` with the repository's Go 1.27.3 environment.
`TestCandidateProfileBoundsSampleGapAndRecoveryWork` checks 780 combinations of offsets, sample gaps, and recovery work.
Offsets range from minus two through two. Sample gaps range from zero through five seconds.
Recovery work ranges from zero through 25 seconds after the sample.
Each case rejects disclosure at real UTC 109 and completes by UTC 144 for B=110.
The test models assigned work durations; it measures no real scheduler or network.
The two-intent outage enumeration adds 12,288 changing-offset and reset-epoch histories.
Other tests preserve accepted state, released flags, unsafe flags, and all eight observer-share subsets.
They reject missing assessments and arithmetic underflow.
The baseline factory at `e296f26` produces actual behavioral failures before the UTC-only change.
These finite checks support the proof under its premises. They establish no production readiness or protocol acceptance.
