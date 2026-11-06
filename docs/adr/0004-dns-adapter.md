# ADR 0004: Select the maintained DNS adapter

## Date

2026-11-04

## Status

Accepted after synthetic compatibility checks. Production integration remains separate.

## Context

The pinned legacy adapter directs new development to its successor.
The project needs explicit signature windows and authoritative DNS behavior.
Source compatibility alone cannot prove DNSSEC interoperability.

## Decision

Select `codeberg.org/miekg/dns v0.6.128` for production integration.
Keep `github.com/miekg/dns v1.1.73` only as the comparison dependency in the isolated research module.
The research report records exact module versions, resolver images, API constraints, and negative controls.
See [the compatibility report](../research/dns-compatibility.md).
Use the successor's version-specific documentation before production calls.

## Consequences

The signed fixtures verify against both adapters.
Unbound 1.22.0 and BIND 9.20.24 validate the current signature through explicit local trust anchors.
Default Unbound accepts the tested signature 20 minutes before inception.
Strict Unbound and BIND reject that signature.
These observations do not establish exact release times for arbitrary resolvers.
The server must unpack requests before it reads DO.
The client must perform explicit TCP fallback after a truncated UDP answer.
Production signing, public delegation, failure-cache transitions, and readiness diagnostics remain separate work.
