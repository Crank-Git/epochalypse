# Candidate check-in boundaries

Issue #25 models FR-switch-protocol-3 and FR-switch-protocol-4.
Production switch use remains disabled until the maintainer decision in #28.

## Local transition

`NewModelAgent` accepts an initial state, a clock sample, and a clock tolerance in seconds.
It rejects an unsafe initial clock or an unrepresentable release boundary.
`ModelAgent.Accepted` returns the accepted state by value.
`ModelAgent.CheckIn` accepts a state only before the current local release boundary.
The sequence must strictly increase. A sequence can skip values.
The switch ID, protocol version, and content digest must match the accepted state.
A repeated sequence fails even if every other field matches.
A repeated sequence with a different deadline also fails.
After the maximum sequence, no check-in can increase the sequence.
Every rejected check-in preserves the accepted state.

The current accepted deadline plus its grace defines the local release boundary.
The proposed deadline cannot postpone this validation boundary.
The model rejects check-ins at the boundary and after the boundary.
The release latch survives a later backward UTC step.
An increasing check-in can change the deadline and grace before that latch applies.
This model does not impose a minimum interval or a maximum interval.
Every candidate release boundary must fit signed UTC seconds.
The arithmetic supports negative UTC values and unsigned grace without overflow.

`ModelAgent.ReleaseEligible` checks the clock and updates the release latch.
It reports eligibility only. It publishes no share.
An unsafe clock blocks both check-ins and new release evidence.
The unsafe status persists for the lifetime of the model agent.
External review must define the recovery procedure before production use.
An unsafe clock cannot remove a share that an observer already holds.

The caller authenticates each owner message before this transition.
The caller serializes calls for each agent.
For competing check-ins with the same sequence, the first accepted state wins locally.
A production agent must persist the accepted state before it signs a receipt.
This model has no store, signature, transport, goroutine, or network service.
The receipt and observer predicates from #24 retain their existing meaning.
Issues #26 and #27 own partial renewal, crashes, lost responses, and recovery schedules.

## Proposed clock assumption

`ClockReading.UTC` contains signed UTC seconds from the Unix epoch.
`ClockReading.Elapsed` contains unsigned monotonic seconds within one process lifetime.
An elapsed value can stay equal. It cannot decrease.
`ClockReading.Safe` asserts an external clock assessment.
The model does not measure that assessment.

For each sample, the model projects its previous effective UTC forward by the elapsed difference.
The supplied UTC must differ from this projection by at most the configured tolerance.
The effective UTC becomes the greater of the projection and supplied UTC.
This rule prevents a backward step from extending the current local acceptance interval.
A forward step can close that interval early.
A step beyond the tolerance latches the unsafe status.
An arithmetic overflow also latches the unsafe status.

The tolerance is a model parameter, not an approved production value.
The tests use zero seconds and two seconds to exercise exact and stepped samples.
These values do not establish a measured UTC error bound.
The tolerance compares successive observations with the effective UTC projection.
It does not bound long-term drift against an independent UTC source.
Monotonic elapsed time alone cannot prove correct UTC time.
A process restart needs separate durable clock and rollback evidence.

Issue #28 must establish the supported UTC error bound and the unsafe-clock response.
The review must also resolve sample frequency, drift, leap-second policy, restart behavior, and clock-source independence.
A clock that remains incorrectly marked safe can cause early or late release.
No production timing guarantee follows from these tests.
The initial trust model assumes honest agents. Two colluding agents can disclose content early.

## Reproduce the evidence

Run `go test ./internal/switchprotocol -count=1 -cover`.
`TestAcceptsIncreasingSequenceBeforeBoundary` accepts an increasing sequence before the boundary.
`TestRejectsReplayStaleAndConflict` preserves accepted state after invalid check-ins.
`TestRejectsRenewalAtReleaseBoundary` checks UTC 109, 110, and 111 against boundary 110.
`TestRetainsReleaseBoundaryAfterBackwardStep` retains eligibility after UTC moves backward.
`TestModelsBoundedClockSteps` covers forward and backward steps within the tolerance.
`TestRejectsUnsafeClockAndRetainsState` covers excessive steps, an unsafe assessment, elapsed rollback, and arithmetic overflow.
`TestRejectsBoundaryOverflow` covers the full signed UTC range and unsigned grace.
The initial transition stubs fail the behavior assertions before implementation.
The tests establish local model behavior only. They do not establish maintainer acceptance.
