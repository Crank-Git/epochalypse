# Switch runtime contracts

Issue #31 introduces runtime contracts for the reviewed version-two protocol.
These contracts do not enable production switches.
The prepare, export, and arm commands remain incomplete until the orchestration slice passes its checks.

## Clock assessment

`internal/agent.AssessClock` validates one externally assessed UTC sample.
It uses the candidate profile from ADR 0013:

- Absolute UTC error: two seconds.
- Maximum sample gap: five seconds.
- Remaining publication and recovery work: 25 seconds.
- Conditional allowance: 34 seconds.

The assessment records the implementation, provenance, reference correctness evidence, source independence evidence, and leap policy.
The caller supplies elapsed readings from the same validated epoch.
The maximum assessment age at capture is 60 seconds.
The maximum capture delay is 250 milliseconds.
Base uncertainty must not exceed 500 milliseconds.
Residual rate uncertainty must not exceed 100 parts per million.
The adapter also accounts for integer quantization within the two-second ceiling.

The resulting lower bound is the sampled UTC second minus two seconds.
Use that result for this decision only.
An elapsed duration never projects release authority into a later decision.

Validate durable authority and irreversible latches before the clock assessment.
After suspend, a clock step, source loss, or an epoch change, obtain new assessment evidence.
Missing or inconsistent evidence blocks readiness.
The function checks evidence structure and candidate limits.
It does not establish the truth of external evidence.
An evidence producer must assess reference correctness and elapsed epoch continuity independently.

The tests use explicit synthetic evidence.
They do not certify this host or establish actual publication and recovery performance.
Production readiness still requires actual-host evidence and independent failure domains.

Verified interface: [Go 1.27.3 time documentation](https://pkg.go.dev/time@go1.27.3), retrieved 2026-11-05.
The installed `go doc time.Time.Sub` also confirms bounded duration arithmetic.
