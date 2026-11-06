# Local metadata performance

Run this command from a clean source checkout with Docker available:

```sh
python3 scripts/metadata_benchmark.py --output var/metadata-benchmark.json
```

The command builds the actual main executable with the pinned images in `deploy/lab/Dockerfile`.
It uses the Go 1.27.3 builder and the Alpine 3.22.2 runtime.
It reuses `tests/backup/fixture_test.go` for synthetic owner state, a window, a released capsule, a countdown, and TLS material.
The fixture creates one record per feature.
The command rejects changes to tracked build inputs before it records the source commit.

The server has a two-CPU quota, a 256 MiB memory limit, and no additional swap allowance.
Docker inspection verifies these limits before the workload starts.
The server has no external network or published ports.
The client shares only the server network namespace.
The command uses unique container names and a private state volume.
It removes these containers, image tags, and the volume after the experiment.
The existing lab and its ports remain unchanged.

The command stores synthetic credentials in a temporary directory with mode 0700.
The credential file has mode 0600.
The client receives credentials through standard input.
The report excludes credentials, cookies, response bodies, and private keys.
The client verifies the fixture certificate and the `localhost` certificate name.
It keeps TLS verification enabled.

## Workload

The client checks service readiness, the source commit, owner authentication, and each workload response before the sample starts.
These checks warm the server and establish reusable HTTPS connections.
The measured workload sends 200 requests at absolute intervals of 50 ms.
It offers 20 requests per second across a ten-second schedule.
The last scheduled request starts at 9.95 seconds.
Requests run independently of earlier response completion.

| Read | Requests | Required result |
|---|---:|---|
| `GET /windows/{id}` | 50 | HTTP 200 with the synthetic window content |
| `GET /api/v1/capsules/{id}` | 50 | HTTP 200 with the expected capsule identifier |
| `GET /api/v1/public/countdowns/{id}` | 50 | HTTP 200 with the expected countdown identifier |
| `GET /api/v1/dashboard` | 50 | HTTP 200 with all four metadata lists |

The capsule and dashboard reads use the owner session.
The workload excludes uploads, preparation, content downloads, and DNS queries.
Window metadata uses the implemented public HTML path.
The dashboard measures the current bounded summary, rather than a large retained inventory.

The report records every request, status, error, start time, and duration.
Each request has a three-second timeout and a bounded response body.
The duration covers request dispatch through the complete response body.
The p50 and p95 use the nearest-rank method across all 200 samples, including errors.
The report also records latency from the scheduled start to expose client delays.
Any request error, missing sample, p95 at or above 250 ms, or start delay at or above 50 ms fails the command.
The JSON report remains available after a measured failure.
Setup failures stop before a measurement report exists.

## Initial result

The initial experiment uses source `12f29ec9e9a841ecc59fa90d3cd2a9f8b268a12b` at `2026-11-06T02:55:29Z`.
All 200 requests return their expected HTTP 200 responses.
The error count is zero.
The p50 is 2.601 ms, the p95 is 4.333 ms, and the maximum is 6.769 ms.
The measured p95 passes the local target of less than 250 ms.
The maximum start delay is 11.129 ms.
The elapsed sample time is 9959.727 ms, which gives 20.081 completed requests per second.

The Linux host uses Docker Desktop 4.98.0 with Docker Engine 29.9.2 on `aarch64`.
The kernel is `7.0.14-linuxkit`.
The Docker virtual machine exposes ten CPUs and 8,319,438,848 bytes of memory.
The server quota limits execution to two CPU equivalents within that shared virtual machine.
This quota does not reserve two exclusive physical cores.
The client runs Node.js v26.13.0 in the pinned Playwright image already used by the recovery checks.

The initial synthetic installation occupies 116 KiB before startup.
Docker reports 12.32 MiB of server memory before the workload and 81.73 MiB after it.
These point samples include owner authentication allocations.
They do not measure peak memory or sustained CPU use.
The report retains Docker statistics and the exact container limits for review.

The first raw build fails because Buildx attempts to write its default cache under the home directory.
The runner now uses the existing `Makefile` convention, `BUILDX_CONFIG=<checkout>/var/cache/buildx`.
It keeps the default Docker context and authentication configuration.

The final integrated source requires a repeat of this short experiment before acceptance.
This result supplies local evidence for this small, warm workload only.
It supplies no arbitrary-VPS performance guarantee or paid deployment recommendation.
