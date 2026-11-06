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

Use the [complete deployment status check](#complete-deployment-status) for the switch reference deployment.
Use the [separate promotion procedure](promotion.md) before production promotion.

## Legacy endpoint status

Run `scripts/deploy-status.sh <sha>` from a checkout with Go 1.27.3.
Use a full lowercase Git commit identifier.
Set `EPOCHALYPSE_STATUS_ENDPOINTS` to comma-separated service origins.
The default origin is `http://127.0.0.1:8080`.
The CLI accepts `status --endpoints <origins> <sha>`.
This legacy mode verifies endpoint liveness and commit only.
It prints a legacy scope notice on stderr. It cannot verify enrollment, readiness, or a complete switch deployment.
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

## Complete deployment status

```sh
scripts/deploy-status.sh --inventory /private/deployment.json --production TESTED_SHA
scripts/deploy-status.sh --inventory /private/deployment.json --production --json TESTED_SHA
```

Use the signed [readiness inventory](readiness.md). Flags precede the full lowercase SHA.
The checker requires the main service and all three canonical enrolled agents.
It reuses bounded pinned HTTPS probes for `/version`, `/healthz`, and `/readyz`.
Every service must report the requested commit, the expected identity, compatible formats, liveness, and functional readiness.
`--production` also requires conditional production readiness. Synthetic services cannot pass that mode.
This result certifies no actual host or physical failure domain.
The probe retains the five-second request limit, 4096-byte response limit, and 20-second overall limit.
It rejects redirects. An invalid or incomplete inventory produces `unknown` without probing a smaller subset.
Use `--json` to retain the requested commit, scope, and every service observation.
The JSON scope is `complete-inventory`. It uses the same state rules as the text output.
The command accepts no `--endpoints` override with an inventory.

| State | Complete inventory evidence |
|---|---|
| `success` | Every required service verifies the requested commit and applicable readiness. |
| `failure` | A verified service reports another commit, or all identities are available but applicable readiness fails. |
| `unknown` | A required identity, trust check, metadata response, or readiness response is unavailable or invalid. |

A known wrong commit takes precedence over unavailable evidence and returns `failure`.
Matching observed commits appear in the commit field, even when readiness is unavailable.
Different verified commits produce `mixed`. Missing verified commit evidence produces `unknown` in that field.
The text remains `<state> local <observed-sha-or-marker>`.
Exit zero means that the command writes the observation. It does not mean deployment success.
Argument errors return 2. Output errors return 1.

The command performs no provider job lookup. `local` denotes direct probes, including probes of remote hosts.
For a watcher adapter, map `success` to `succeeded` and `failure` to `failed`.
Keep `unknown` as unavailable evidence outside the watcher's terminal-success states.
If the watcher requires only its fixed state vocabulary, report unavailable evidence as `running` until its bounded deadline fails.
Never translate `unknown` to `succeeded` or infer a provider rollback.
Local checks never infer `pending` or `rolled-back`.
The wrapper stores Go and build caches under `var/` by default. It preserves Docker and GitHub authentication configuration.
