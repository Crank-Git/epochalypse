# ADR 0005: Preserve the switch release guarantee

## Date

2026-11-04

## Status

Accepted owner decision to retain the guarantee and keep switch implementation blocked.
No production protocol has acceptance.

## Context

The candidate model fails release at the owner-confirmed boundary after partial renewal and one agent outage.
A concrete trace appears in parked PR #67 and the protocol research reports.
The model proves a failure of that candidate under its stated assumptions.
It does not prove that every replacement protocol fails.

## Decision

The owner states: “Keep the original guarantee; research a replacement protocol (recommended)”.
Preserve the original guarantee during replacement research in issue #70.
Compare documented alternatives before any production switch implementation.
Keep committed state distinct from client-observed confirmation.
Replacement research compared replicated decisions and prepare/commit evidence.
Independent review accepted its scoped final-response contradiction, not a universal impossibility claim.
The owner then states: “Keep the guarantee and leave switch implementation blocked (recommended)”.
Keep switch implementation blocked while other features continue.

## Consequences

The baseline research remains useful as a reproducible counterexample.
Research #70 is complete. Issue #27 remains blocked because no accepted protocol satisfies its recovery criterion.
Issue #28 remains the protocol acceptance gate.
The project does not substitute eventual release at a later local deadline for the original guarantee.
Production switch use stays disabled.
