# Candidate switch rules

Issue #24 defines a research model for FR-switch-protocol-1 and FR-switch-protocol-2.
The model uses synthetic identities and shares.
Production switch use remains disabled until the maintainer decision in #28.
This report proposes messages for review. It does not approve a cryptographic protocol.

## Model contract

`internal/switchprotocol` provides these types:

| Type | Meaning |
| --- | --- |
| `AgentID` | One enrolled agent identity. |
| `State` | The switch ID, protocol version, sequence, deadline, grace, and content digest. |
| `Receipt` | An agent identity, accepted state, durable sequence, and durability assertion. |
| `PublicShare` | An agent identity bound to a switch ID, protocol version, and content digest. |
| `Rules` | Exactly three distinct, nonempty agent identities. |
| `Observer` | An accumulated record of public shares for one content key. |

`NewRules` rejects empty or repeated identities.
`Rules.RenewalConfirmed` requires three distinct agents with durable receipts for the exact requested state.
A receipt with a different durable sequence contributes nothing.
Repeated receipts contribute once. An unknown agent contributes nothing.
Extra invalid receipts cannot invalidate three valid receipts.

`Rules.NewObserver` creates an empty public history.
`Observer.Observe` retains each matching agent's public share.
`Observer.ReleaseAvailable` requires shares from two distinct enrolled agents.
Repeated shares contribute once. Unrelated shares contribute nothing.
Any two of the three agents suffice.

A public share has no sequence or deadline filter.
The candidate uses the same content key across renewals.
A later renewal cannot recall an earlier public share.
The observer has no delete or clear operation.
Create one observer per content key. Retain it across renewals.

`State.Deadline` uses signed UTC seconds from the Unix epoch.
`State.Grace` uses unsigned seconds.
Issue #25 owns sequence validation, clock checks, boundary arithmetic, and agent transitions.
These predicates compare supplied evidence. They do not validate owner authorization or state eligibility.
The caller must provide authenticated identities and valid shares.
The durability flag asserts a model fact. It does not prove persistence across a real restart.
The package does not implement signatures, share distribution, a store, transport, or key reconstruction.

## Candidate messages

Every signed message contains these common fields in this order:

1. Switch ID.
2. Protocol version.
3. Sequence.
4. Deadline.
5. Grace.
6. Content digest.

The content digest contains exactly 32 bytes.
The candidate requires a reviewed digest algorithm before production use.
Each message then contains these fields:

| Message | Signer | Additional fields in order |
| --- | --- | --- |
| `prepare` | Owner | Three agent identities and their encrypted shares, ordered by identity bytes. |
| `arm` | Owner | Three durable receipt envelopes, ordered by agent identity bytes. |
| `checkin` | Owner | No additional fields. |
| `status-request` | Owner | Agent identity, request nonce. |
| `status-response` | Agent | Agent identity, request nonce, accepted state, durable sequence, public-share flag. |
| `release` | Agent | Agent identity, share bytes. |
| `receipt` | Agent | Agent identity, accepted state, durable sequence. |

An accepted state repeats the six common fields in their defined order.
The `status` operation uses separate request and response message domains.
Each envelope contains the signed bytes, signer identity, and signature bytes.
The signature covers the domain and body defined below.
The verifier must bind the signer identity to the enrolled public key.
A receipt attests that the agent persists the entire accepted state before it signs.
The receipt's durable sequence must equal its accepted sequence.
The model's durability flag is an input for fault schedules, not a wire field.

## Candidate byte format

The candidate specifies a custom binary format for later independent review.
No encoder or decoder ships in this issue.
Use these exact rules:

1. Start the signed bytes with the ASCII bytes `epochalypse/switch/research-v1/`.
2. Append the message name from the table.
3. Append one zero byte.
4. Append the common fields in their defined order.
5. Append the additional fields in their defined order.

Use these field representations:

| Field | Bytes |
| --- | --- |
| Protocol version | Four bytes, unsigned, most significant byte first. |
| Sequence, grace, durable sequence | Eight bytes, unsigned, most significant byte first. |
| Deadline | Eight bytes, signed two's complement, most significant byte first. |
| Content digest | Exactly 32 bytes, with no length prefix. |
| Public-share flag | One byte: zero for private, one for public. |
| Switch ID, agent identity | Four-byte unsigned length, then ASCII bytes. |
| Nonce, encrypted share, share, signature, signed bytes | Four-byte unsigned length, then opaque bytes. |
| Accepted state | The six common fields, without a domain prefix. |
| Array | Four-byte unsigned count, then each element in order. |
| Envelope | Signed bytes, signer identity, signature bytes, each encoded as above. |

IDs contain 1–128 ASCII letters, digits, hyphens, or underscores.
A nonce contains exactly 32 bytes.
Other variable fields contain at most 1,048,576 bytes.
An envelope uses its own message domain inside its signed bytes.
Arrays in `prepare` and `arm` contain exactly three distinct identities.
Reject trailing bytes, duplicate identities, invalid lengths, unknown versions, and unknown message names.
Reject noncanonical identity order and flags other than zero or one.
These rules separate message signatures across operations and protocol versions.
The production format needs versioned fixtures and a review before implementation.

## Fault assumptions and open evidence

The initial model assumes honest agents with bounded clock error.
The model permits one unavailable agent.
Two colluding agents can disclose the content key early.
A shared provider, credential, or disk can violate failure independence.

An unavailable agent prevents successful renewal.
Two available agents can still supply the release threshold.
A failed renewal leaves the old deadline as the owner's conservative deadline.
An agent can retain a later local deadline after partial renewal.
These threshold predicates do not establish a release time after that partial renewal.
Issues #26 and #27 must retain counterexamples and measure the resulting uncertainty.
No guaranteed outage bound follows from these tests.

The later review must specify these controls before production use:

- Secure share distribution to enrolled agent keys.
- Authenticated transport and local signature verification.
- Replay defense and concurrent check-in behavior.
- Durable state before a receipt, including crash and lost-response schedules.
- Backup rollback detection and an append-only public-share history.
- Cancellation that preserves public copies.
- Agent replacement that accounts for every previously disclosed share.

## Reproduce the evidence

Run `make test-unit` for the rule tests.
`TestRequiresTwoDistinctPublicShares` rejects a missing share and a repeated share.
`TestRequiresThreeDistinctDurableReceipts` rejects missing, repeated, nondurable, and mismatched receipts.
`TestRetainsPublicSharesAcrossRenewal` retains an old share after a later deadline.
These assertions fail against the initial predicate stubs in commit `6a80b7e`.
The tests establish threshold rules only. They do not establish maintainer acceptance.
