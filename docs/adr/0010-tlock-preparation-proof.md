# ADR 0010: Retain a digest for tlock publication

Status: Accepted by the batch PM.

## Context

Tlock preparation authenticates the JWE object with a fresh content key.
It encrypts that key for the pinned quicknet round.
Retaining the unwrapped key until publication would defeat local tlock preparation.
The shared installation layout already separates capsule records from 32-byte private values.

## Decision

Retain a SHA-256 digest of the canonical draft in the existing private-state slot.
Discard the content key after preparation.
The digest covers the object, wrapper, metadata, and effective release time.
Publication compares the digest before it changes the publication time.
The shared transaction charges the complete encoded record and the separate digest.

This check detects a changed draft.
It is not a signature against an attacker who controls both database buckets.
It does not prove future wrapper decryption independently of the preparation path.
Recovery verifies the pinned beacon, authenticates the wrapper, and authenticates the complete JWE object.

## Alternatives

Retaining the content key would let publication authenticate the object again.
It would also let the server read a locally prepared tlock capsule before release.
A separate signed preparation format would require another key and trust contract.
The current draft integrity requirement needs neither change.

## Evidence

The [batch finding](https://github.com/Crank-Git/epochalypse/issues/17#issuecomment-6648850961) records the accepted producer contract.
`TestTimelockDraftRetainsNoContentKey` verifies the retained digest and rejects its use as a content key.
`TestTimelockPublicationRejectsChangedDraft` verifies the publication check.
`TestTimelockWrapperConsumesSharedBudget` verifies atomic quota rejection.
ADR 0009 defines the shared storage layout and lifetime.
