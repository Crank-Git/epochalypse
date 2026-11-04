# External interface rules

## Scope

Read this file before an external library, protocol adapter, service, CLI, or provider API change.

## Evidence

Read the external interface's own schema, help output, or version-specific official documentation before the first call.
Record the documentation URL, exact version, and relevant capability beside the decision or pull request.
Verify operation names, required fields, response fields, errors, limits, and permissions.
Use official examples only after their version matches the selected dependency.
Treat conflicting examples as an unresolved compatibility question.
Place an unconfirmed capability in the spec's Risks & open questions section.
Do not implement an assumed API shape.

## Project references

- Go: the spec initially targets Go 1.27.3; use https://go.dev/doc/devel/release.
- DNSSEC: use RFC 4033, RFC 4034, and RFC 4035 from https://www.rfc-editor.org/.
- DNS errors: use RFC 8914 from https://www.rfc-editor.org/rfc/rfc8914.html.
- DNS stale data: use RFC 8767 from https://www.rfc-editor.org/rfc/rfc8767/.
- DNS adapter candidate: use https://github.com/miekg/dns/tree/v1.1.73.
- Timelock candidate: use https://github.com/drand/tlock/tree/v1.2.0.
- Beacon assumptions: use https://docs.drand.love/docs/timelock-encryption/.
- Resolver behavior: use the selected version's manual from https://unbound.docs.nlnetlabs.nl/.

The foundation verifies the DNS adapter's maintenance status before selection.
The tlock adapter verifies the chain hash and public key rather than an example's network default.
Pin every selected resolver image by digest. Record the resolver version with each observation.
Select the durable store, password library, threshold library, and lint tools through documented foundation decisions.
No provider-specific API has approval in v1.

## Calls and configuration

Use bounded timeouts, bounded bodies, explicit error checks, and local signature verification where applicable.
Keep real credentials and private infrastructure values outside tracked files.
Use read-only discovery to verify a provider's behavior.
Infrastructure purchases and new paid dependencies require a separate owner decision.
A malformed external command requires a documentation check before a retry.
