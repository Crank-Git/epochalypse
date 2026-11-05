# ADR 0012: Accept bounded switch uncertainty

## Date

2026-11-05

## Status

Accepted owner decision for the product tradeoff only. Supersedes ADR 0005's exact-confirmation requirement.
No replacement protocol has production acceptance. Production switch use remains disabled.

## Context

The [replacement report](../research/switch-protocol/replacement.md) demonstrates a contradiction under the former exact-confirmation contract.
Agents cannot distinguish client receipt delivery from receipt loss after their durable state becomes identical.
The original traces remain historical evidence. This decision changes the required predicate rather than those traces.
The [direction report](../research/switch-protocol/directions.md) compares three alternatives with primary source links.
Direction A retains independent agents and changes the receipt-loss promise.
Direction B adds replicated decision authority but retains client uncertainty.
Direction C supplies a fixed external fallback date that later check-ins cannot revoke.

## Decision

The owner states: “I like the recommendation, plus it gets us moving again”.
Accept direction A: three independent agents, three matching durable receipts for confirmed renewal, and two release shares.
After incomplete receipts, show uncertainty from B0 through Bmax, with E separate.
B0 is the last confirmed release boundary and the earliest possible disclosure boundary after an uncertain response.
B0 does not establish every agent's actual authority. A missing response can hide partial or complete acceptance.
Bmax is the latest boundary in any signed renewal already issued for transmission.
After main-server loss plus one agent outage, release occurs by Bmax plus E under explicit review assumptions.
The promise requires two reachable, honest agents with intact durable state and clocks within the reviewed error bound.
Permanent owner-client loss does not require another owner message or inter-agent coordination for release.
The protocol review defines numeric clock limits and bounded publication work. This decision chooses no numeric E.

The client durably journals intent before transmission. Retries preserve signed bytes, sequence, deadline, and boundary.
Arrival time never extends the deadline. An uncertain response never produces a green success state or a definitive failure claim.
An agent that ever releases a share refuses renewal of the same content key.
The model counts observer-retained shares across all later histories.

## Consequences

An uncertain renewal can release before its proposed boundary or after the last confirmed boundary.
With the default interval, uncertainty can approach 24 hours. The owner accepts this cost.
Grace does not remove this uncertainty.
Preserve the baseline and replacement reports as evidence against the former requirement.
Run revised model schedules and independent protocol review before production switch use.
Keep restart, rollback quarantine, release races, signed time limits, and independent ciphertext retrieval in that review.
Completed DNSSEC, capsule, and countdown deployment proceeds separately from live switch operations.
Comparative external research proceeds without production switch acceptance. This decision approves no service, purchase, or new dependency.

## Verification

The issued switch requirements and acceptance criteria define the revised predicates.
The direction report's 108-case enumeration supports only the limited deadline argument.
It does not establish storage, clock, cryptographic, or complete protocol correctness.
