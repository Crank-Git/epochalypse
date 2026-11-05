# Local lab

The lab contains public synthetic data and a known owner password.
Keep real content and production keys outside this lab.
Install Docker with Compose support. Start the Docker daemon.
Run `make lab` from the repository root.

The command waits for one main process and three healthy agent processes.
The main process owns HTTPS and authoritative UDP/TCP DNS with one window store.
Each agent retains its own process and state volume.
The containers share one host. They do not provide independent failure domains.
The agents still provide no switch release behavior.

## Setup and files

Two setup jobs complete before the main process starts.
The first generates an explicit synthetic DNSSEC key and a self-signed TLS certificate.
It uses the existing pinned Playwright image with networking disabled.
The verified image supplies Node.js v26.13.0, Python 3.12.3, and OpenSSL 3.0.13.
The pinned BIND image lacks `dnssec-keygen` and `openssl` executables.
No new image or product CLI command is required.

The setup publishes a complete lab namespace through a private temporary directory.
OpenSSL commands permit ten seconds and 16384 output bytes.
Generated files permit at most 16384 bytes and use mode 0600. Their directory uses mode 0700.
UID 65534 owns the material and runs the main process and agents.
The setup uses root only to assign volume ownership.
Existing valid material remains unchanged. Partial or unsafe material fails setup.

The second job initializes the synthetic owner before the live database lock exists.
It records successful initialization and preserves the owner on later starts.
The known lab password is `synthetic-lab-password`.
The DNS key has the explicit `epochalypse-lab-only` marker.
The certificate lasts 30 days and covers localhost, web, and 127.0.0.1.
Material remains under `/state/lab` in the main state volume. The runtime image contains no generated credentials.

Explicit `EPOCHALYPSE_ENV=lab` permits synthetic DNS material without weakening HTTP transport guards.
Published owner access uses TLS. The insecure HTTP option remains loopback-only.
Production rejects the lab zone, nameserver target, and marked key.
See [signed window configuration](windows.md) for production constraints.

## Access

HTTPS uses host address `127.0.0.1:8080`. UDP and TCP DNS share `127.0.0.1:1053`.
Agent health routes remain inside the container network.
Set `LAB_HTTP_PORT` or `LAB_DNS_PORT` to change a host port.
Set `COMPOSE_PROJECT_NAME` and `LAB_IMAGE_TAG` to isolate a second lab.
Use the same variables for subsequent commands.

Copy the public lab certificate for client verification:

```sh
mkdir -p var/lab-client
docker compose -f deploy/lab/compose.yaml cp web:/state/lab/tls.crt var/lab-client/tls.crt
curl --fail --cacert var/lab-client/tls.crt https://127.0.0.1:8080/healthz
```

The response is HTTP 200 with `ok` and a newline.
Browsers identify the certificate as self-signed. Trust it only for this synthetic lab.
The local container health probe skips certificate verification. The E2E owner client verifies the certificate and hostname.
Open `https://localhost:8080/owner` to sign in with the known password.
The [window API](windows.md#creation-api) creates records while the main process runs.
The lab publishes them immediately, including before inception. They remain public after expiration.

For an offline CLI experiment, stop the main process first:

```sh
docker compose -f deploy/lab/compose.yaml stop web
docker compose -f deploy/lab/compose.yaml run --rm --no-deps web window create \
  --zone windows.lab.invalid. --key-file /state/lab/window.private \
  --content 'public lab experiment' \
  --inception 2030-01-01T00:00:00Z --expiration 2030-01-01T01:00:00Z
docker compose -f deploy/lab/compose.yaml up --detach --wait --wait-timeout 60
```

Replace `RETURNED_NAME` with the returned name for direct queries:

```sh
dig @127.0.0.1 -p 1053 RETURNED_NAME TXT +dnssec
dig @127.0.0.1 -p 1053 RETURNED_NAME TXT +dnssec +tcp
```

The future RRSIG does not keep TXT content secret.
Resolver acceptance remains an observation. A local anchor does not establish public delegation.

## Health and reset

Use `docker compose -f deploy/lab/compose.yaml ps --all` to inspect health.
The setup jobs exit successfully. The main process and three agents report healthy.
Use `docker compose -f deploy/lab/compose.yaml logs` to inspect failures.
Use `docker compose -f deploy/lab/compose.yaml restart web` to restart only the main process.
Committed signatures and agent volumes remain unchanged.
Use `docker compose -f deploy/lab/compose.yaml down` to stop the lab and preserve state.

The following reset deletes all synthetic lab state, including its owner and windows.
Use it only for this lab's project:

```sh
docker compose -f deploy/lab/compose.yaml down --volumes
make lab
```

Reset the synthetic lab when its certificate expires.
The setup refuses expired material instead of silently replacing an existing key.

## Checks and verified interfaces

`make test-e2e` includes the real Compose test with a unique project and available loopback ports.
It verifies TLS owner creation, immediate UDP/TCP records, explicit fallback, and the original signature after a new main process.
It also verifies four distinct volumes, UID 65534, disabled setup networking, and three healthy agents.
Cleanup removes only that test's containers and volumes.
The legacy `epochalypse dns` refusal scaffold remains available. This lab does not use it.

Verified on 2026-11-04:

- [Compose services](https://docs.docker.com/reference/compose-file/services/) and installed Compose 5.5.1 help: dependencies, health, restart, and file copies.
- [Compose JSON Lines](https://docs.docker.com/reference/cli/docker/compose/ps/) and [volumes](https://docs.docker.com/engine/storage/volumes/): health and independent state mounts.
- [Node crypto](https://nodejs.org/api/crypto.html), v26.13.0: explicit ECDSA fixtures.
- OpenSSL 3.0.13 shipped `req -help` and `ecparam -list_curves`: prime256v1, certificate extensions, and lab key files.
- BusyBox v1.37.0: the actual HTTPS health command in the pinned Alpine runtime.
- Installed curl TLS help and dig help: certificate verification and direct UDP/TCP examples.

The Dockerfile pins Go and Alpine image indexes. Compose pins the existing setup image.
Generated material never enters the build context.
