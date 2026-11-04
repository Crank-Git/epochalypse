# ADR 0001: Separate validation from secret release

## Date

2026-11-04

## Status

Accepted for the project plan. The switch protocol remains subject to its dedicated review.

## Context

DNSSEC validates authenticity and signature windows. DNSSEC does not provide confidentiality.
A direct authoritative query can retrieve an early public record.
A key published beside ciphertext does not protect that ciphertext until inception.
The owner requires switch release despite main-server loss.

Sources: RFC 4033, RFC 4034, and RFC 4035 from https://www.rfc-editor.org/.
The research evidence appears in docs/specs/pages/research-evidence.md.

## Decision

Provide a public DNSSEC demonstration mode.
Provide server-held key capsules with an explicit operator-trust statement.
Provide tlock capsules after the adapter verifies the supported beacon configuration.
Use three independent agents for the initial renewable switch design.
Require all three durable receipts for a successful renewal.
Require two valid release shares for recovery.
Keep recovery independent of the main server.
Research release after loss of all project-operated hosts in a separate epic.

## Consequences

One unavailable agent can prevent renewal. Two available agents can support release under the reviewed fault model.
Two colluding agents can disclose content early.
Web preparation exposes plaintext to the server in v1.
CLI tlock preparation avoids that preparation exposure to the server.
A retained fixed-time ciphertext cannot support a later renewable deadline.
The protocol review must resolve partial renewal and retained-share behavior before production switch use.
The owner approved these planning tradeoffs on 2026-11-04.
