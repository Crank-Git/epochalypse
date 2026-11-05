# Resolver observations

The main process supplies bounded jobs, durable run records, and the observation API.
The resolver matrix compares the three pinned configurations against production signed windows.

`resolver.Observe` queries one trusted configured resolver for a public TXT record.
It uses `authority.Query` for the two-second UDP/TCP bound and explicit TCP fallback.
Each result records UTC start and completion times, transport, response code, and available DNS flags.
An unavailable resolver supplies an error code with no response code or flags.
The result preserves resolver identity, exact version, and metadata provenance.

## Service configuration

Add one to three trusted resolver targets to the version 1 configuration file:

```json
{
  "version": 1,
  "zone": "windows.example.net.",
  "nameserver": "ns.operator.example.org.",
  "public_url": "https://admin.example.net",
  "resolvers": [{
    "id": "unbound-strict",
    "address": "127.0.0.1:5353",
    "version": "1.22.0",
    "version_source": "configured",
    "clock": {"source": "unknown", "offset_nanoseconds": null}
  }]
}
```

Replace the example values with the installation's zone, nameserver, public origin, and reachable resolver targets.
The operator configures the resolver's delegation or local trust anchor separately.
The service sends DNS queries. It does not create resolver containers or change resolver settings.
Targets require distinct identifiers and explicit metadata provenance.
Version and clock provenance permit `configured` or `unknown`.
An unknown version uses `"version":"unknown"`. An unknown clock offset uses JSON `null`.
A configured offset records the operator's value. It does not measure the remote resolver clock.
The configuration cannot claim an executable or clock measurement from the main process.
An absent target list disables new jobs and preserves public reads of retained runs.

## Observation API

Send `POST /api/v1/windows/{id}/observations` to start a run.
Use an authenticated owner session, the current CSRF proof, and HTTPS outside the loopback lab.
An empty body or an empty JSON object is valid.
The body permits at most 1024 bytes. A nonempty body requires `application/json`.
Unknown fields and POST query parameters fail. Requests cannot supply resolver targets.
Bearer tokens cannot start a run or fall back to an owner session.

A successful request returns 202 with the durable pending run.
The `Location` header identifies the observation collection for that window.
The main process admits at most four active jobs. Each job queries at most three configured targets.
Each query has one absolute two-second bound across UDP and TCP fallback.
Cancellation closes the owned connection. Shutdown drains accepted work before the window store closes.
A failed durable completion disables the job service until restart and recovery.

`GET /api/v1/windows/{id}/observations` permits public reads.
Both methods use the shared TLS, error, and response safeguards.
Public responses omit configured resolver addresses.

```json
{"version":1,"runs":[],"next_cursor":"","poll_after_seconds":1}
```

Each page contains at most fifty runs.
If `next_cursor` is nonempty, request the next page with `?after=<next_cursor>`.
Identifiers define page order. They do not define chronological order.
Keep the run identifier from the POST response.
Poll from the first page at intervals of at least one second. Follow cursors until you find that run.
Continue while its state is `pending`. Display every result when the state becomes `complete` or `partial`.
A complete run can record SERVFAIL. The state means that all resolver queries return a response.
A partial run records unavailable or interrupted work with absent response fields.
Public data and validation flags do not establish confidentiality.

| Status | Result |
|---|---|
| 400 | An identifier, cursor, or request body is invalid. |
| 401 | The owner session fails, or a POST uses a bearer credential. |
| 403 | The POST CSRF proof or origin fails. |
| 404 | The window does not exist. |
| 405 | The request uses a method other than GET or POST. |
| 409 | The shared installation quota rejects the run reservation. |
| 413 | The POST body exceeds 1024 bytes. |
| 426 | Access requires HTTPS. |
| 429 | Four jobs are active. `Retry-After: 1` supplies the retry interval. |
| 503 | The signer, store, configured job service, or durable completion is unavailable. |

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
The service calls this method before it accepts new jobs.
It preserves completed runs and explicit unknown clock offsets.
UTC readings can move backward when the clock changes. The store preserves those readings.

`Store.ListRuns(windowID, after)` returns at most fifty runs in identifier order.
A nonempty cursor requests the next page. An empty next cursor identifies the final page.

Verified interfaces, retrieved 2026-11-04:

- [Successor v0.6.128 message types](https://pkg.go.dev/codeberg.org/miekg/dns@v0.6.128#Msg) define the response fields.
- [Successor v0.6.128 client](https://codeberg.org/miekg/dns/src/tag/v0.6.128/client.go) defines `ExchangeWithConn` and its fixed read timeout.
- [Go context cancellation](https://pkg.go.dev/context#AfterFunc) closes owned connections when the shared query context ends.
- [Unbound 1.22.0 configuration](https://github.com/NLnetLabs/unbound/blob/release-1.22.0/doc/example.conf.in) defines the anchor, stub zone, and zero-skew controls.
- [BIND 9.20.24 reference](https://bind9.readthedocs.io/en/v9.20.24/reference.html) defines the static stub and trust anchor.
- [Docker bridge networks](https://docs.docker.com/engine/network/drivers/bridge/) define container isolation and published ports.
