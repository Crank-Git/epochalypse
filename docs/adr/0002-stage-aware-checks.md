# ADR 0002: Require tests for the implemented stage

## Date

2026-11-04

## Status

Accepted by the PM within the approved phased implementation plan.

## Context

The foundation precedes the DNSSEC and switch features.
A requirement to execute future feature scenarios during the foundation creates a dependency cycle.
An empty success target would conceal missing verification.

## Decision

The current required suite executes every applicable test for implemented behavior.
Each feature adds its scenarios to the required suite when it lands.
Unavailable standalone suite targets fail explicitly until their real tests exist.
The foundation includes real CLI, live-listener, integration, and container smoke evidence.
Every required test passes. Aggregate first-party Go coverage reaches at least 95.00 percent before rounding.

## Consequences

The foundation does not claim DNSSEC interoperability or a verified switch protocol.
The final v1 still requires all resolver, browser, integration, and fault scenarios in the approved spec.
The CI command consumes the evolving Makefile contract rather than a duplicate static suite list.
The PM checks criterion evidence at each sub-merge and the complete batch gate.
