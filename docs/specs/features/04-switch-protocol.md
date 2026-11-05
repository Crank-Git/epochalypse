---
id: switch-protocol
feature: "Switch protocol review"
epic: "Epic 4: Switch protocol review"
status: built
issues: [23, 24, 25, 26, 27, 28, 70, 106, 107]
mockups: []
---

# Switch protocol review

## Purpose

Validate the release protocol before implementation makes an outage guarantee.

## User stories

As an owner, I want this feature so that I can verify the release behavior before I trust the deployment.

## Functional requirements

- FR-switch-protocol-1: The protocol defines a two-of-three release threshold.
- FR-switch-protocol-2: The protocol requires three durable acknowledgments for a successful renewal.
- FR-switch-protocol-3: The protocol binds each check-in to an increasing sequence number.
- FR-switch-protocol-4: The protocol rejects a renewal at or after the local release boundary.
- FR-switch-protocol-5: The review models partial renewal and lost acknowledgments.
- FR-switch-protocol-6: The review models old shares retained by public observers.
- FR-switch-protocol-7: The model releases content by Bmax plus E after main-server loss plus one agent outage.
- FR-switch-protocol-8: The review records a maintainer decision before production switch use.

## User flows

```mermaid
flowchart TD
 Draft[Write protocol] --> Model[Model faults and transitions]
 Model --> Safe{Properties satisfied?}
 Safe -->|No| Revise[Revise protocol or state limitation]
 Revise --> Model
 Safe -->|Yes| Review[Independent protocol review]
 Review -->|Accepted| Gate[Permit switch implementation]
 Review -->|Rejected| Revise
```

1. Define the fault model.
2. Specify every transition and message.
3. Explore the failure schedules.
4. Record counterexamples or supporting evidence.
5. Obtain a maintainer decision before production use.

## Screens & states

This feature has no dedicated screen. The CLI reports progress, success, and structured errors.

## Behaviour rules

The initial fault model permits one unavailable agent. It assumes honest agents with bounded clock error.
Two colluding agents can disclose a secret early. State this trust limit.
A renewal must reach all agents. One unavailable agent can prevent renewal even when release remains available.
ADR 0012 accepts bounded uncertainty after incomplete receipts. It does not accept a production protocol.
B0 is the last release boundary confirmed by three matching durable receipts.
Bmax is the latest boundary in any owner-signed renewal already issued for transmission.
E is the reviewed allowance for clock error and publication work. Its numeric value remains a review output.
With two reachable, honest agents with intact durable state, release occurs by Bmax plus E under the reviewed clock assumptions.
This promise includes permanent owner-client loss. Release does not require the main server or inter-agent coordination.
After incomplete receipts, B0 is the earliest possible disclosure boundary, not proof of every agent's actual authority.
A missing response can hide partial or complete acceptance. The client reports uncertainty from B0 through Bmax, with E separate.
The default interval can produce almost 24 hours of uncertainty. The owner accepts this cost; grace does not remove it.
Before transmission, the client durably records the signed request, sequence, deadline, and proposed boundary.
Retries use the identical signed bytes, sequence, and deadline. Arrival time never extends a deadline.
The model covers multiple issued requests, delayed delivery, duplicate retries, sequence conflicts, and signed time limits.
The review establishes clock limits, bounded publication work, restart behavior, and rollback quarantine.
The baseline and replacement counterexamples remain evidence against the former exact-confirmation contract.
Preserve their traces and original assertions. Evaluate the revised predicates separately.
Do not use a new threshold cryptography primitive without a maintained library and review.
Specify secure share distribution, authenticated transport, replay defense, and durable state before acknowledgment.
Specify backup rollback detection, released-share history, cancellation, and agent replacement.
Treat a partial release as irreversible. Any agent that ever releases its share refuses renewal of the same content key.
The model counts all shares retained by observers, including shares no endpoint currently displays.
Keep switch production use disabled until the review passes.

## Data touched

Protocol version, durable client journal, B0, Bmax, E assumptions, state model, counterexample schedules, decision record. This epic uses synthetic secrets only.

## Interfaces

Specify `prepare`, `arm`, `checkin`, `status`, and `release` message schemas.
Each signed message includes switch ID, protocol version, sequence, deadline, grace, and content digest.
Each receipt includes agent identity, accepted state, durable sequence, and signature.
Status distinguishes confirmed state, per-agent evidence, issued intent, and uncertain outcome.
An unresolved request survives client restart through the durable journal.
Use canonical byte encoding with domain separation. The spike specifies the exact encoding.
Library selection and exact threshold APIs remain research outputs, not assumed external capabilities.

The API uses the shared error format in the project spec. Mutations require owner authorization unless this file states otherwise.

## Edge cases & failures

Crash before persistence, crash after persistence, lost response, replay, clock steps, concurrent renewal, and partition all need executable schedules.
A common provider, shared credential, or shared disk invalidates failure independence.

## Acceptance criteria

Each criterion requires a test or a reproducible review record. The quoted behavior is the observable result.

- AC-switch-protocol-1 covers FR-switch-protocol-1: The protocol defines a two-of-three release threshold.
- AC-switch-protocol-2 covers FR-switch-protocol-2: The protocol requires three durable acknowledgments for a successful renewal.
- AC-switch-protocol-3 covers FR-switch-protocol-3: The protocol binds each check-in to an increasing sequence number.
- AC-switch-protocol-4 covers FR-switch-protocol-4: The protocol rejects a renewal at or after the local release boundary.
- AC-switch-protocol-5 covers FR-switch-protocol-5: The review models partial renewal and lost acknowledgments.
- AC-switch-protocol-6 covers FR-switch-protocol-6: The review models old shares retained by public observers.
- AC-switch-protocol-7 covers FR-switch-protocol-7: The model releases content by Bmax plus E after main-server loss plus one agent outage.
- AC-switch-protocol-8 covers FR-switch-protocol-8: The review records a maintainer decision before production switch use.

- AC-switch-protocol-9 covers FR-switch-protocol-5: Lost final receipts report B0 through Bmax even when all agents accept the renewal.
- AC-switch-protocol-10 covers FR-switch-protocol-7: Each unavailable-agent schedule releases by Bmax plus E after permanent main-server and owner-client loss.
- AC-switch-protocol-11 covers FR-switch-protocol-7: The review records explicit clock limits and publication bounds for two reachable, honest agents with intact durable state.
- AC-switch-protocol-12 covers FR-switch-protocol-3: Restarted clients retry journaled bytes with the same sequence and deadline without arrival-time extension.
- AC-switch-protocol-13 covers FR-switch-protocol-6: Observer-retained shares remain counted after release, restart, cancellation, and later renewal attempts.
- AC-switch-protocol-14 covers FR-switch-protocol-6: An agent that ever releases its share refuses renewal of the same content key.

## Out of scope

Claims of Byzantine fault tolerance and release after all three agents disappear.

## Open questions

The technical gates in the project spec apply. This feature has no additional product questions.
