# Completed-feature deployment verification

Review date: 2026-11-05. Source commit: `aef217561f7230791f1d8ac0e4360bdb59ec0b69`.
The deployment templates and this record accompany issue #40.
The check uses Docker Desktop's Linux arm64 daemon and Docker Compose v5.6.1.
It creates only synthetic local containers, state, signing keys, and TLS material.

## Reproduce the check

Follow [the local procedure](operations.md#local-containers) with a unique `COMPOSE_PROJECT_NAME`.
Capture the JSON from both offline create commands. Preserve each returned identifier, DNS name, and the window's original RRSIG.
Query both names over UDP and TCP. Expect `NOERROR`, `aa`, the TXT record, and its RRSIG.
The window content remains public before its 2030 inception.
Run both `lab verify` commands. Each result must report `verified` and `explicit-local-anchor`.

For capsule recovery, stop `web` before the offline create and publish commands.
Use a public capsule with synthetic text and a future requested release time.
The [capsule procedure](capsules.md) supplies the supported CLI flags and public bundle route.
Start `web` before downloading `/api/v1/public/capsules/RETURNED_ID/bundle` over verified HTTPS.
Pass that bundle to `epochalypse capsule open --bundle /tmp/bundle.json --output /tmp/recovered` inside the container.
Compare the recovered bytes with the original synthetic text.
Use `docker compose exec -T web sh -c 'cat > /tmp/bundle.json'` with the bundle on standard input.
Docker's file-copy operation cannot write into this read-only container's temporary mount reliably.

Read `/windows/RETURNED_WINDOW_ID` and `/events/RETURNED_COUNTDOWN_ID` over verified HTTPS.
Restart `web`. Wait for its healthy state before repeating those reads and DNS queries.
Compare the window RRSIG byte-for-byte after whitespace normalization in dig's display.
Compare the public capsule bundle byte-for-byte after restart.
Confirm `/version` equals the requested full source commit.
Confirm `docker compose -f deploy/main/compose.yaml ps --services --status running` prints only `web`.

## Observed results

| Check | Result |
|---|---|
| Container build and Compose configuration | Pass; the pinned Go/Alpine image builds the unchanged main service. |
| HTTPS health and version | Pass; the client verifies the synthetic certificate and localhost hostname. |
| Main-only startup | Pass; one running main service, two completed material jobs, one completed owner job, zero agents. |
| Window and countdown CLI creation | Pass; each uses its separate zone and DNSSEC key. |
| Window and countdown UDP/TCP answers | Pass; both return authoritative signed TXT records. |
| Window and countdown infrastructure | Pass; both local-anchor checks report `verified`. |
| Capsule authenticated recovery | Pass; the public bundle recovers `synthetic capsule content`. |
| Main process restart | Pass; identifiers, the original window signature, and the public capsule bundle remain unchanged. |
| Public feature pages | Pass; window and countdown pages respond over verified HTTPS. |
| Production Compose configuration | Pass; `config --quiet` accepts explicit example image digest and absolute mount variables. No production container starts. |
| Unit checks | `make test-unit`: 641 Go tests pass with zero skips; 11 Python checks pass. |

## Resource observation

The observation follows owner initialization and creation of one window and one countdown.
It runs without concurrent traffic. It measures an idle process, not deployment capacity.

```sh
docker stats --no-stream --format json PROJECT-web-1
docker compose -f deploy/main/compose.yaml exec web sh -c 'du -k /state/v1/*.db; id'
docker image ls epochalypse-main --format '{{.Size}}'
```

| Measurement | Observation |
|---|---|
| Container memory | 8.676 MiB with a 512 MiB limit |
| CPU sample | 0.00 percent |
| Process and thread count | 8 |
| Owner database allocated disk | 24 KiB |
| Installation database allocated disk | 24 KiB |
| Runtime image size | 48.8 MB |
| Service identity | UID 65534, GID 65534 |

Database allocation, encoded quotas, and filesystem capacity differ.
The default object limit is 100. The default encoded quota is 1073741824 bytes.
The default capsule plaintext limit is 10 MiB; the production configuration example selects 1 MiB.
Measure uploads, beacon failures, resolver jobs, traffic, and disk growth before selecting a paid host or changing resource limits.
This record supplies no cost estimate or paid-host recommendation.

## Identity and startup review

The production template requires `MAIN_UID` and `MAIN_GID` from the dedicated host service identity.
Compose configuration with UID 12345 and GID 12346 resolves `web.user` to `12345:12346`.
The local synthetic template retains its separate UID/GID 65534 contract.
The Linux identity check creates fresh named volumes with protected synthetic configuration, DNSSEC pairs, TLS files, and owner state.
Every fixture directory uses mode 0700. Every protected fixture file uses mode 0600.
UID 12345 and GID 12346 own the fixtures and their private parent directories.
The check uses those identifiers without creating a host account or changing host ownership.

The identity can read each required file and create a file in its state directory.
The same identity initializes the synthetic owner, then starts the main process with both authorities and verified HTTPS.
The process uses a read-only root, no capabilities, and a network namespace without external access or published ports.
Its protected files reside on the Linux volume. This check does not claim macOS bind mounts enforce Linux permissions.

The check omits `owner.initialized` while preserving the existing owner database.
Running `lab_owner.sh` returns `owner already exists` and a nonzero exit status.
Direct main-process startup bypasses that job and succeeds without a marker.
The existing synthetic password authenticates through `/api/v1/session` with the current CSRF proof.
The authenticated `/api/v1/dashboard` read returns HTTP 200.
This proves the direct startup contract for an existing owner database. It does not implement or verify encrypted archive restoration.
The check removes only its own container and volumes.

## Applicability limits

This change adds documentation and deployment examples. It changes no Go, web, or existing script behavior.
TDD pre-patch failure and new Go coverage do not apply to this change.
The integration pull request still requires the full native suite and its aggregate coverage gate.
The systemd template has a source review against systemd 257 documentation.
No host unit installation, real VPS, public delegation, parent DS publication, provider API, or production promotion occurs.
The isolated encrypted restore test belongs to issue #42. This check does not claim that restore test passes.
