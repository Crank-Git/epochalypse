# 0013 — Accept the reviewed switch design for implementation

Date: 2026-11-05

Status: accepted for implementation; production use remains gated.

## Context

ADR 0012 accepts bounded receipt-loss uncertainty. It does not establish a protocol implementation.
The earlier projected-clock proof requires a continuous exact elapsed timeline.
[Issue 106](https://github.com/Crank-Git/epochalypse/issues/106) supplies a conservative fresh-UTC policy that permits new clock epochs.
[Issue 107](https://github.com/Crank-Git/epochalypse/issues/107) supplies authenticated byte, enrollment, freshness, and lifecycle contracts.
Independent review finds no remaining design contradiction under the stated premises.
The [review](../research/switch-protocol/review.md) records evidence and the separate maintainer decision for issue 28.
Requiring an unbuilt runtime before design acceptance would make the implementation gate circular.

## Decision

Accept the reviewed design for implementation under ADR 0012 and the explicit clock, state, authentication, and publication obligations.

## Consequences

Use fresh `L=U-epsilon` decisions without elapsed-clock release authority.
The candidate ceilings are epsilon=2 seconds, sampling gap S=5 seconds, and remaining publication and retrieval work W=25 seconds.
Thus P=30 seconds and E=34 seconds under the recorded assessment contract.
These are design ceilings, not measured host capabilities.
Validate durable authority, irreversible flags, and freshness evidence outside restored backups before trusted clock samples.
Require distinct canonical agent signing keys and share recipients, authenticated enrollment, and exact durable intent retries.
Retain public observer shares and refuse renewal of an ever-released content key.

Implement and verify persistence, transport, certificate binding, process recovery, and independent public retrieval before production use.
Actual hosts must demonstrate the clock and publication ceilings and independent failure domains.
Missing readiness evidence blocks the production claim.
This decision authorizes no host, service, domain, deployment, or live promotion.
Preserve the former exact-confirmation counterexamples and their assertions as historical evidence.
