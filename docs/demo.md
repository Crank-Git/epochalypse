# Synthetic demo and deployment status

Run `make demo` to create `var/state/demo/fixtures.json`.
Set `EPOCHALYPSE_STATE_DIR` to select another state directory.
The command creates four synthetic fixtures: a capsule, a switch, a countdown, and a window.
Each fixture has `implemented` set to `false`.
These fixtures provide no release behavior, cryptographic material, or DNS records.
The command rejects existing fixtures.

Run `make clean-demo NAMESPACE=demo` to remove the demo namespace.
The command requires the exact namespace `demo`.
It rejects an empty namespace, other names, and traversal paths.
Cleanup removes only the `demo` entry below the selected state directory.
Symbolic links cannot direct cleanup outside that directory.
The demo command rejects a symbolic link as its namespace.

The CLI also accepts `demo --state-dir <directory>` and `clean-demo --state-dir <directory> --namespace demo`.
The local filesystem controls access to these commands.

Run `scripts/deploy-status.sh <sha>` from a checkout with Go 1.27.3.
Use a full lowercase Git commit identifier.
Set `EPOCHALYPSE_STATUS_ENDPOINTS` to comma-separated service origins.
The default origin is `http://127.0.0.1:8080`.
The CLI accepts `status --endpoints <origins> <sha>`.
List every required recovery endpoint when those endpoints have public origins.
The current lab exposes only the web origin to the host.

The command prints `<state> local <observed-sha>`.
The `local` job identifier denotes direct endpoint checks.
It identifies no provider job.
The command exits zero when it produces a status line.
Argument errors and output errors produce a nonzero exit.

| State | Evidence |
|---|---|
| `success` | Every origin reports the requested commit and returns `ok` with a newline from `/healthz`. |
| `failure` | Origins report one valid commit, but health or the requested commit differs. |
| `unknown` | An identity is unavailable, malformed, unknown, or inconsistent across origins. |
| `pending` | Reserved for a future provider adapter. Local checks never infer this state. |

An unknown result prints `unknown` in the commit field.
Requests have a two-second timeout and a 4096-byte response limit.
The checker rejects redirects and credentials in origins.
No provider configuration or deployment monitor exists in this slice.

`GET /version` returns JSON with a `commit` field.
The value comes from the build, rather than the process environment.
`make build`, `make run`, and `make lab` stamp the checkout identity at build time.
A dirty checkout adds `-dirty`, which cannot satisfy a successful status check.
Direct Go builds without the version linker flag report `unknown`.
For a direct Compose build, set `EPOCHALYPSE_BUILD_COMMIT` from `scripts/build-commit.sh` before the build.

Verified against the Go 1.27.3 shipped documentation for `net/http.Client`, `io.LimitReader`, and `os.Root` on 2026-11-04.
The shipped `go tool link -h` documents the `-X` build flag.
See [Go package documentation](https://pkg.go.dev/std@go1.27.3) and [Docker build arguments](https://docs.docker.com/build/building/variables/).
