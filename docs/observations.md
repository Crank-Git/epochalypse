# Resolver observations

This initial slice supplies a bounded query and a real strict-resolver test.
The observation API and durable observation records remain incomplete in issue #9.

`observation.Observe` queries one trusted configured resolver for a public TXT record.
It uses `authority.Query` for the two-second UDP/TCP bound and explicit TCP fallback.
Each result records UTC start and completion times, transport, response code, and available DNS flags.
An unavailable resolver supplies an error code with no response code or flags.
The result preserves resolver identity, exact version, and metadata provenance.

A configured clock offset does not establish a measured clock offset.
The test records configured zero offset and an interval from a Docker clock sample.
All test containers share the Docker VM clock.
The test rejects an interval wholly outside one second from the host clock.
These wide signature intervals do not establish exact boundary acceptance.

Run the initial strict-resolver test from the repository root:

```sh
go test -tags=resolvers ./tests/resolvers -count=1 -v -timeout=120s
```

Use the worktree-local Go cache variables from [the check guide](checks.md).
The test compiles a static Linux arm64 helper that consumes the production window store and authority.
The test uses the pinned resolver images from [the compatibility report](research/dns-compatibility.md).
It verifies the executable version through `unbound -V`.
The current-signature positive control must return NOERROR with AD.
The future-signature query must return SERVFAIL without AD.
The test removes only its own containers and network.

The local trust anchor does not establish public delegation.
Public content remains visible through direct authoritative access.
SERVFAIL and AD do not establish confidentiality.

Verified interfaces, retrieved 2026-11-04:

- [Successor v0.6.128 message types](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128#Msg) define the response fields.
- [Unbound 1.22.0 configuration](https://github.com/NLnetLabs/unbound/blob/release-1.22.0/doc/example.conf.in) defines the anchor, stub zone, and zero-skew controls.
- [Docker bridge networks](https://docs.docker.com/engine/network/drivers/bridge/) define container isolation and published ports.
