# 0014 — Recover derived checkpoints from pinned journal authority

Date: 2026-11-05

Status: accepted for implementation; actual-host and production gates remain.

## Context

An agent can stop after journal synchronization and before checkpoint publication.
The complete committed journal then contains a newer state than the derived checkpoint.
Treating every older checkpoint as rollback would quarantine this ordinary crash boundary.
The [maintainer ruling](https://github.com/Crank-Git/epochalypse/issues/31#issuecomment-6666312262) resolves the authority distinction.

## Decision

The independently pinned complete journal is authoritative. The checkpoint is derived state.
Before reconstruction, validate the full committed chain and retained material against the pinned authority.
A missing checkpoint can be reconstructed. An older checkpoint must match an authentic committed prefix.
Reconstruct the current journal head and synchronize the checkpoint before readiness or output.

Preserve these committed values:

- The sequence.
- The deadline.
- The released latch.
- The unsafe-clock latch.

Reject missing or corrupt authority. Quarantine divergent, newer, corrupt, or unprovable checkpoints and material.
Recovery requires no main server or other agent.

## Consequences

A crash between journal synchronization and checkpoint publication can recover without weakening committed authority.
A whole authority or virtual-machine rollback remains outside the intact durable-state premise.
Recovery work must fit the reviewed sampling and publication bounds before a release-bound claim applies.
The implementation must validate retained material before readiness; an abstract recovery predicate alone supplies no such proof.
Actual-host verification remains separate. This decision authorizes no infrastructure operation or production promotion.
Historical exact-confirmation counterexamples remain unchanged.
