# Local lab

Install Docker with Compose support.
Start the Docker daemon.
Run `make lab` from the repository root.
The command builds the pinned image and waits for five healthy containers.
If a tool, port, or configuration fails, inspect the command error.
Use `docker compose -f deploy/lab/compose.yaml logs` to inspect process errors.

The lab starts one web server, one DNS server, and three separate agent containers.
Each container has its own named state volume with a `v1` directory.
The web server binds `127.0.0.1:8080` on the host.
The DNS server binds `127.0.0.1:1053` for UDP and TCP on the host.
Agent health routes remain inside the container network.
Set `LAB_HTTP_PORT` or `LAB_DNS_PORT` to change a host port.
Set `COMPOSE_PROJECT_NAME` to isolate a second lab.

Run `curl --fail http://127.0.0.1:8080/healthz`.
The route returns HTTP 200 with `ok` and a newline.
Run `docker compose -f deploy/lab/compose.yaml ps` to inspect all five health states.
Run `docker compose -f deploy/lab/compose.yaml down` to stop the lab.
Use the same project name and port variables for subsequent Compose commands.
The stop command preserves the state volumes.

The DNS and agent modes are foundation scaffolds.
The DNS server returns REFUSED for complete query headers.
It reads the complete TCP frame before it returns a 12-byte response.
The two-byte length field limits each query to 65,535 bytes.
The UDP receive buffer holds 65,535 bytes to prevent silent input truncation.
Complete UDP queries above 512 bytes receive the same 12-byte REFUSED response.
The response preserves the query identifier and clears the recursion-available flag.
Short headers and incoming responses receive no reply.
The server closes each TCP connection after one frame or a one-second deadline.
Incomplete frames receive no reply.
It serves neither records nor DNSSEC signatures.
The agent processes provide health routes and versioned state directories.
They provide no switch, receipt, key share, or release behavior.
The containers share one Docker host and do not provide independent failure domains.

For a web server outside Docker, run `make run`.
This target uses `config/local.example.json` unless `EPOCHALYPSE_CONFIG` selects another file.
Use `epochalypse server`, `epochalypse dns`, or `epochalypse agent` after a native build.
For direct executable commands, copy `config/local.example.json` to `config/local.json` first.
The service commands accept these flags:

| Flag | Environment variable | Default |
|---|---|---|
| `--config` | `EPOCHALYPSE_CONFIG` | `config/local.json` |
| `--http-addr` | `EPOCHALYPSE_HTTP_ADDR` | `127.0.0.1:8080` |
| `--dns-addr` | `EPOCHALYPSE_DNS_ADDR` | `127.0.0.1:1053` |
| `--state-dir` | `EPOCHALYPSE_STATE_DIR` | `var/state` |
| `--agent-id` | `EPOCHALYPSE_AGENT_ID` | Required for agent mode |

Flags override environment variables.
The local configuration contains one JSON document with `version` set to `1`.
The loader rejects unknown fields and unsupported versions.
Future features extend this format before they add DNS and agent settings.
Interrupt or terminate a service process to close its listeners.

Verified interfaces, retrieved 2026-11-04:

- [Docker Compose service attributes](https://docs.docker.com/reference/compose-file/services/), Compose 5.5.1.
- [Docker Buildx state location](https://docs.docker.com/build/building/variables/#buildx_config).
- [Docker official Go image](https://hub.docker.com/_/golang), Go 1.27.3 Alpine image.
- [Docker official Alpine image](https://hub.docker.com/_/alpine), Alpine 3.22.2 image.
- [DNS message and transport formats](https://www.rfc-editor.org/rfc/rfc1035.html), RFC 1035 sections 4.1 and 4.2.
- [TCP message length field](https://www.rfc-editor.org/rfc/rfc7766.html#section-8), RFC 7766 section 8.

The Dockerfile pins both image indexes by digest.
`docker buildx imagetools inspect` verifies each digest before its first use.
The installed Compose help verifies `up --build --detach --wait --wait-timeout` and `config --quiet`.
