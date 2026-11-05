# Resolver observations

This slice supplies bounded queries, durable run records, and the real resolver matrix.
The configured job runner and observation API remain incomplete in issue #9.

`resolver.Observe` queries one trusted configured resolver for a public TXT record.
It uses `authority.Query` for the two-second UDP/TCP bound and explicit TCP fallback.
Each result records UTC start and completion times, transport, response code, and available DNS flags.
An unavailable resolver supplies an error code with no response code or flags.
The result preserves resolver identity, exact version, and metadata provenance.

A configured clock offset does not establish a measured clock offset.
The test records configured zero offset and an interval from a Docker clock sample.
All test containers share the Docker VM clock.
The test rejects an interval wholly outside one second from the host clock.
These wide signature intervals do not establish exact boundary acceptance.

Run the resolver matrix from the repository root:

```sh
make test-resolvers
```

Use the worktree-local Go cache variables from [the check guide](checks.md).
The test compiles a static Linux helper for the Docker daemon architecture that consumes the production window store and authority.
The test uses the pinned resolver images from [the compatibility report](research/dns-compatibility.md).
It verifies the executable version through `unbound -V`.
The test verifies BIND's executable version through `named -V`.
Each current-signature positive control must return NOERROR with AD.
Default Unbound accepts the signature twenty minutes before inception.
Strict Unbound and BIND reject that signature.
All configurations reject far-future and expired signatures.

The test queries a separate boundary record before inception to populate warm caches.
Cold caches first query that record two seconds after inception.
Warm strict Unbound retains its failure while the cold resolver validates the record.
The test waits for the warm strict resolver to recover after its failure cache expires.
Default Unbound and BIND validate through both caches at the sampled time after inception.
`.test-artifacts/resolvers.json` records public windows and timestamped results.
The test removes only its own containers and network.

The local trust anchor does not establish public delegation.
Public content remains visible through direct authoritative access.
SERVFAIL and AD do not establish confidentiality.

## Durable run records

The pure `internal/observation` package defines resolver metadata, results, and run records.
The `internal/resolver` adapter performs network queries.
The window store owns both retained content types in `v1/windows.db`.
The additive `observations-v1` bucket uses `<window-id>/<run-id>` keys.
Opening an older version 1 database creates this bucket and preserves its signed windows.
The fixed DNSKEY record and original window format remain valid.
`testdata/observation-v1.json` preserves the observation document format.

`Store.ReserveRun` atomically reserves one document and 16384 bytes from the installation budget.
It records the pending run before a resolver job starts.
`Store.CompleteRun` atomically commits all configured results and releases unused reserved bytes.
The final document permits at most 16384 bytes.
A quota failure preserves accepted windows and runs.
Windows cannot consume bytes reserved for a pending run.

Run states are `pending`, `complete`, and `partial`.
An unavailable resolver produces a partial run with absent response fields.
A second completion fails because terminal records are immutable.
`Store.RecoverRuns` marks interrupted pending jobs partial after a process restart.
The service must call this method before it accepts new jobs.
It preserves completed runs and explicit unknown clock offsets.
UTC readings can move backward when the clock changes. The store preserves those readings.

`Store.ListRuns(windowID, after)` returns at most fifty runs in identifier order.
A nonempty cursor requests the next page. An empty next cursor identifies the final page.

Verified interfaces, retrieved 2026-11-04:

- [Successor v0.6.128 message types](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128#Msg) define the response fields.
- [Unbound 1.22.0 configuration](https://github.com/NLnetLabs/unbound/blob/release-1.22.0/doc/example.conf.in) defines the anchor, stub zone, and zero-skew controls.
- [BIND 9.20.24 reference](https://bind9.readthedocs.io/en/v9.20.24/reference.html) defines the static stub and trust anchor.
- [Docker bridge networks](https://docs.docker.com/engine/network/drivers/bridge/) define container isolation and published ports.
