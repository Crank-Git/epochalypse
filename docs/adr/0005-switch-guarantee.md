# ADR 0005: Preserve the switch release guarantee

## Date

2026-11-04

## Status

Accepted research direction. No production protocol has acceptance.

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
Any infeasible contract requires an explicit blocker report and maintainer decision.

## Consequences

The baseline research remains useful as a reproducible counterexample.
Issue #27 remains blocked on replacement research.
Issue #28 remains the protocol acceptance gate.
The project does not substitute eventual release at a later local deadline for the original guarantee.
Production switch use stays disabled.
