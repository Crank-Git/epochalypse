# Deploy completed features

See [deployment readiness](readiness.md) for the local inventory, public metadata, and direct recovery checks.

This guide covers the main service, DNSSEC windows, capsules, and countdowns.
These features require no agent or switch protocol acceptance.
Switch production use remains gated on actual-host verification.
The default lab agent scaffolds provide no switch release behavior.
The [switch runtime guide](switch-runtime.md) describes the separate implemented runtime.
The [four-process lab](lab.md) serves development checks. Use the main-only example below for this deployment slice.

## Local containers

Use synthetic content only. The example creates a known password and self-signed certificates.
Docker stores both databases on its Linux filesystem. A macOS bind mount cannot establish the required database lock behavior.
Run these commands from the repository root with Docker Compose 5.6.1:

```sh
mkdir -p var/main-example var/cache/buildx
export BUILDX_CONFIG="$PWD/var/cache/buildx"
export COMPOSE_PROJECT_NAME=completed-main
export EPOCHALYPSE_BUILD_COMMIT="$(git rev-parse HEAD)"
python3 -c 'from pathlib import Path; Path("var/main-example/countdown-material.mjs").write_text(Path("scripts/lab_material.mjs").read_text().replace("windows.lab.invalid.","countdowns.lab.invalid."))'
docker compose -f deploy/main/compose.yaml build web
docker compose -f deploy/main/compose.yaml up --detach --wait --wait-timeout 60
docker compose -f deploy/main/compose.yaml ps --all
docker compose -f deploy/main/compose.yaml cp web:/state/lab/tls.crt var/main-example/tls.crt
curl --fail --cacert var/main-example/tls.crt https://localhost:8443/healthz
curl --fail --cacert var/main-example/tls.crt https://localhost:8443/version
```

Expect one healthy `web` container and three successfully completed setup containers. No agent container starts.
The countdown setup uses a separate volume and independently generated DNSSEC key.
Its additional TLS files and password remain unused synthetic setup artifacts.
The health probe skips verification inside the container. The operator's curl command verifies the copied certificate and hostname.
Open `https://localhost:8443/owner`. Sign in with `synthetic-lab-password` after you trust this synthetic certificate locally.
The origin in `deploy/main/local.json` matches port 8443. If you change that port, also change the origin.
Set `MAIN_DNS_PORT` to change the loopback DNS port. Preserve all variables for later commands.

Stop the main service before offline CLI commands:

```sh
docker compose -f deploy/main/compose.yaml stop web
docker compose -f deploy/main/compose.yaml run --rm --no-deps web window create \
  --zone windows.lab.invalid. --key-file /state/lab/window.private \
  --content 'synthetic public window' \
  --inception 2030-01-01T00:00:00Z --expiration 2030-01-01T01:00:00Z
docker compose -f deploy/main/compose.yaml run --rm --no-deps web countdown create \
  --zone countdowns.lab.invalid. --key-file /countdown/lab/window.private \
  --title 'synthetic event' --event-at 2030-01-01T00:00:00Z
docker compose -f deploy/main/compose.yaml up --detach --wait --wait-timeout 60
docker compose -f deploy/main/compose.yaml exec web epochalypse lab verify \
  --zone windows.lab.invalid. --address 127.0.0.1:1053 --anchor-file /state/lab/window.key
docker compose -f deploy/main/compose.yaml exec web epochalypse lab verify \
  --zone countdowns.lab.invalid. --address 127.0.0.1:1053 --anchor-file /countdown/lab/window.key
```

Use returned record names for UDP and TCP checks:

```sh
dig @127.0.0.1 -p 1153 RETURNED_NAME TXT +dnssec
dig @127.0.0.1 -p 1153 RETURNED_NAME TXT +dnssec +tcp
```

The [capsule guide](capsules.md) describes capsule creation, publication, export, and authenticated content recovery.
The [countdown guide](countdowns.md) describes advisory TTL behavior and public checks.
Restart `web` to check persistence. Compare the original window signature and the retained countdown identifier after restart.
Use `docker compose -f deploy/main/compose.yaml down` to remove this deployment's containers while preserving its volumes.
Use `down --volumes` only when you intend to delete this synthetic deployment's state and keys.

## Linux VPS example

The systemd template targets systemd 257 or later on Linux.
This repository verifies the service command in Linux containers. It does not claim a tested VPS installation or host systemd startup.
An operator must review the host's installed systemd documentation and verify the unit before installation.
The commands below describe operator actions. This issue performs no host installation or real deployment.

1. Select a previously tested commit. Preserve its full SHA and the build artifact's digest outside the server.
2. Run `make check` at that commit before deployment. Use the Go toolchain and pinned dependencies from the repository.
3. Build with `make build`. The Makefile embeds the checked-out commit in the executable.
4. Create a dedicated `epochalypse` system user and group without an interactive login.
5. Install the matching `bin/epochalypse-linux-amd64` or `bin/epochalypse-linux-arm64` at `/opt/epochalypse/epochalypse`, owned by root and readable/executable by the service.
6. Create `/var/lib/epochalypse`, owned by `epochalypse:epochalypse`, with mode 0700 on a persistent local disk.
7. Create `/etc/epochalypse/keys` and `/etc/epochalypse/tls` with service-owned mode 0700.
8. Copy `deploy/main/production.example.json` to `/etc/epochalypse/main.json`. Replace every example domain with operator-controlled names.
9. Set the configuration file's owner to `epochalypse:epochalypse` and mode to 0600.
10. Supply separate algorithm 13 BIND v1.3 key pairs for the two dedicated, nonoverlapping zones.
11. Install each pair as `window.private`/`window.key` or `countdown.private`/`countdown.key` in the protected keys directory.
12. Set all key files to mode 0600 with service ownership. Preserve each public key's original zone name.
13. Supply a valid certificate chain for the public HTTPS hostname at `/etc/epochalypse/tls/fullchain.pem`.
14. Supply its separate TLS private key at `/etc/epochalypse/tls/private.pem`. Use service ownership and mode 0600.
15. Create the owner before service startup, with a protected temporary password file:

```sh
sudo -u epochalypse /opt/epochalypse/epochalypse owner init \
  --state-dir /var/lib/epochalypse --password-file /etc/epochalypse/initial-password
```

Remove the initial password file after successful initialization. Preserve the password in the owner's separate password manager.
Never use the lab material script or known password for a real deployment.
Keep the administrator hostname and nameserver outside both served zones.
Use a public address for the nameserver's A/AAAA records. Configure parent delegation as described below.
Verify host time synchronization independently. `After=time-sync.target` does not measure clock error.

Review `deploy/main/epochalypse.service` before an operator copies it to `/etc/systemd/system/epochalypse.service`.
Run `systemd-analyze verify /etc/systemd/system/epochalypse.service` on that host.
After successful review, the operator runs `systemctl daemon-reload` and `systemctl enable --now epochalypse.service` with host administration rights.
Inspect `systemctl status epochalypse.service` and `journalctl -u epochalypse.service` for startup failures.
The unit gives only the bind capability for privileged ports. Its writable persistent directory is `/var/lib/epochalypse`.
It limits memory to 512 MiB, CPU to one core, tasks to 128, and open descriptors to 4096.
These example limits require workload measurements before production use.

Permit inbound UDP 53, TCP 53, and TCP 443 in both host and upstream firewalls.
Keep host administration private. The main service exposes no separate agent-control listener.
The example binds IPv4. Configure and test IPv6 separately before publishing AAAA records.
Outbound HTTPS supports beacon retrieval for tlock capsules. Resolver experiments also require access to explicitly configured DNS targets.
If the beacon stops, tlock availability can fail. Public windows provide no confidentiality.

For containers on a Linux VPS, replace synthetic setup with operator-supplied keys, TLS files, configuration, and initialized durable state.
Use `deploy/main/production.compose.yaml` for the exact production mounts and port mappings.
Set `MAIN_IMAGE` to the tested image digest, including the image name and `@sha256:` digest.
Set these absolute host paths before the operator starts Compose:

```sh
export MAIN_STATE=/var/lib/epochalypse
export MAIN_CONFIG=/etc/epochalypse/main.json
export MAIN_KEYS=/etc/epochalypse/keys
export MAIN_TLS=/etc/epochalypse/tls
export MAIN_UID="$(id -u epochalypse)"
export MAIN_GID="$(id -g epochalypse)"
docker compose -f deploy/main/production.compose.yaml config --quiet
docker compose -f deploy/main/production.compose.yaml up --detach
```

The production template requires the dedicated host service user's numeric UID and GID.
Use that identity for owner initialization and container runtime. The local synthetic template separately uses UID/GID 65534.
Assign `MAIN_STATE`, its database files, `MAIN_CONFIG`, `MAIN_KEYS`, and `MAIN_TLS` to this host service identity.
Use mode 0700 for state, configuration, DNSSEC, and TLS directories. Use mode 0600 for the configuration, databases, and key files.
Give the service identity execute access through every parent directory, including `/etc/epochalypse` and `/var/lib`.
For a private parent, use service ownership and mode 0700. Keep shared system parents under their existing administrative ownership.
Verify that the identity can traverse those parents and read every mounted file before startup.
Verify that it can create files in `MAIN_STATE`. Docker requires existing source paths and permits no automatic directory creation here.
Docker's daemon also needs access to each source path. If user namespaces remap identifiers, verify the resulting host identity separately.
Do not run the systemd service and the container against the same state or ports.
Use the same server flags as the unit, with container ports 8080 and 1053.
Map public TCP 443 to 8080 and both UDP/TCP 53 to 1053.
Keep the service UID, read-only root, dropped capabilities, resource limits, and persistent state from the local template.
Mount signing material and TLS material read-only. Match their numeric ownership to `MAIN_UID:MAIN_GID`.
Do not run the local template unchanged on public interfaces.
A managed platform qualifies only when it supports both DNS transports, HTTPS, durable local filesystem locks, ownership, and protected key mounts.
No provider purchase or paid-host recommendation follows from this guide.

## DNSSEC delegation checks

Repeat these checks for each zone. Use the countdown key only for the countdown zone.
The [window guide](windows.md#deployment-trust-check) defines the local-anchor check's exact limits.

1. Obtain the authoritative server's address and public DNSKEY through an independent trusted operator channel.
2. Confirm current DNSKEY and SOA signatures directly over UDP and TCP:

```sh
dig @AUTHORITY_IP windows.example.net. DNSKEY +dnssec
dig @AUTHORITY_IP windows.example.net. SOA +dnssec +tcp
epochalypse lab verify --zone windows.example.net. \
  --address AUTHORITY_IP:53 --anchor-file /protected/window.key
dnssec-dsfromkey -2 -f /protected/window.key windows.example.net.
```

Use BIND 9.20.21's `dnssec-dsfromkey` for the SHA-256 DS record.
Record the full digest, algorithm, and key tag. A key tag alone does not identify a key.
Coordinate the child NS delegation and derived DS with the parent zone's operator through the parent's authenticated process.
Publish the DS only after both DNS transports and current infrastructure signatures pass.
Do not derive the parent DS from a deliberately future experimental TXT signature.

```sh
dig @PARENT_AUTHORITY windows.example.net. NS +norecurse
dig @PARENT_AUTHORITY windows.example.net. DS +dnssec +norecurse
dig +trace +dnssec windows.example.net. SOA
delv windows.example.net. SOA
```

Compare the parent's DS with the independently derived record. Confirm the parent NS target resolves to the intended authority.
Use a validating resolver with a trusted root anchor for `delv`.
Investigate an unsigned parent, missing DS, mismatched digest, expired infrastructure signature, or blocked TCP before release.
`dig +trace +dnssec` displays the chain; it does not validate that chain.
The project's `lab verify` validates only an explicit local anchor. It does not implement complete public delegation verification.
An AD flag alone supplies no independent trust proof.
Record timestamps, commands, addresses, public records, and validation results in the operator's deployment record.
Recheck after delegation changes and TTL expiry. Coordinate key changes as a separate migration; retained windows bind the original DNSKEY.

## State contract for isolated backup and restore

This contract covers completed main-service features only. Issue #42 implements the encrypted backup and isolated restore test.
The archive contains sensitive owner state and unreleased capsule keys. Encrypt it with a separate operator-controlled backup key.
DNSSEC keys, TLS keys, capsule keys, and the backup key have different purposes. Never reuse one for another purpose.

| Included item | Meaning and required format |
|---|---|
| `state/v1/owner.db` | Complete bbolt database: owner identity, Argon2id password profile, sessions, tokens, and audit state. Version 1 records. |
| `state/v1/windows.db` | Complete bbolt installation database, including every bucket below. |
| `config/main.json` | Exact version 1 settings, including both zones, public origin, resolver provenance, and applicable content limits. |
| `keys/window.private`, `keys/window.key` | Original algorithm 13 BIND pair; private-key format v1.3. |
| `keys/countdown.private`, `keys/countdown.key` | Separate algorithm 13 BIND pair; private-key format v1.3. |
| `tls/fullchain.pem`, `tls/private.pem` | PEM certificate chain and separate TLS private key for the deployment. |
| Backup metadata | Source commit, archive format version, UTC snapshot time, included paths, sizes, digests, numeric ownership, and selected limits. |

The installation database contains `windows-v1`, `observations-v1`, `capsules-v1`, `capsule-keys-v1`, `countdowns-v1`, and `configuration-v1`.
Capsule objects and manifests reside inside the database. `capsule-keys-v1` contains private content keys or tlock preparation digests, depending on the mode.
The version 1 configuration bucket binds the window DNSKEY. Restore the entire database without bucket-level reconstruction.
Treat unknown record or bucket formats as incompatible. Retain the matching executable commit and build digest in the operator's separate artifact store.
`GET /version` reports only the commit today. The archive must record format versions separately.

Exclude caches, executable build output, logs, test artifacts, plaintext input files, initial password files, demo state, and all agent state.
Exclude the backup encryption key from its own archive. Keep its recovery copy through a separate operator-controlled channel.
In the local example, map `/state/v1` to `state/v1` and both `/state/lab/window.*` files to `keys/window.*`.
Map `/countdown/lab/window.*` to `keys/countdown.*`. Map `/state/lab/tls.*` to the corresponding TLS paths above.
Exclude `owner.initialized`, synthetic passwords, setup metadata, and the countdown setup's unused TLS files.
The `owner.initialized` marker controls fresh lab setup only. It does not authenticate or validate an owner database.
Include the effective configuration and limits, rather than assuming defaults on restore.

The internal archive codec uses age v1 encryption with a separate X25519 recipient.
Its plaintext contains an eight-byte manifest length, one version 1 JSON manifest, and the declared file payloads in manifest order.
The manifest has a 64 KiB limit. File payloads use bounded streams instead of JSON byte arrays.
The codec requires an explicit positive byte limit for the complete plaintext, including the manifest.
The suggested default is 4 GiB. This permits the default 1 GiB retained installation plus database overhead.
Select a larger finite limit when the complete databases and required files exceed that limit.
Private temporary disk holds up to twice the selected plaintext limit during restore.
The codec uses mode 0700 for temporary directories and mode 0600 for temporary files.
It authenticates the complete age stream before it parses the manifest or exposes restored file paths.
It removes temporary plaintext after failure. The caller removes successful temporary state through the archive's `Close` method.
The internal `Snapshot` function copies a stopped installation with the canonical paths above.
It holds exclusive bbolt locks on both complete databases until every file copy completes.
It rejects either active database, replaced database files, source links, unsafe file modes, cancellation, and excess bytes.
It records numeric file ownership and copies files into a private temporary directory.
The caller must call the returned archive's `Close` method to remove these copies.
Keep configuration and key writers stopped throughout the call. Database locks do not control those files.
The `backup create` command copies this stopped installation into an encrypted archive.
The internal `owner.ValidateSnapshot` function checks a read transaction from a private, authenticated owner database copy.
It requires the four version 1 owner buckets and their supported record formats.
It checks the password profile, credential owner identifiers, token digests and scopes, and audit sequences.
It preserves expired credentials and their original deadlines without a database mutation.
This domain check does not open the database, validate the installation database, or authorize service startup.
The archive's `ValidateOwner` method applies this domain check to the authenticated private owner file.
It rechecks the file path, protected mode, size, and digest before bbolt reads the database.
It uses read-only access and preserves all database bytes. Cancellation and unavailable temporary state fail the check.
The archive's `Install` method completes validation before it publishes a fresh installation.
Supply a destination path, an expected source commit, and the destination service's numeric user and group identifiers.
The expected commit must match the separately retained executable artifact.
Installation rejects existing destinations, including empty directories and links.
It validates all six installation buckets through the existing domain decoders in a read-only transaction.
It requires the original window DNSKEY binding and authenticated capsule private state.
It checks the archived quotas, version 1 settings, separate DNSSEC pairs, and matching TLS certificate and key.
It rejects changed authenticated files and unsupported numeric ownership before publication.
It preserves both complete databases and every original deadline without record reconstruction.
It maps file ownership to the explicitly selected destination service identity.
It copies files into a private directory beside the destination, then syncs files and directories before publication.
An exclusive rename publishes the complete directory without replacement.
Linux requires `renameat2` with `RENAME_NOREPLACE`. macOS uses `renameatx_np` with `RENAME_EXCL` for local tests.
The implementation returns an error when the filesystem cannot support exclusive publication.
It removes unpublished temporary files after cancellation or failure.
If the final parent-directory sync fails, the complete destination can exist despite the error.
Inspect that destination before a retry. The retry never replaces it.
Use a trusted destination parent. Keep source configuration, keys, and temporary state inaccessible to other writers.
Successful installation does not start a service or run a bootstrap job.
The CLI exposes these checks through `backup restore`.
`scripts/test_backup.py` exercises encrypted backup and direct recovery in isolated Linux volumes.
Run it through `make test-e2e` or `python3 -m unittest discover -s scripts -p 'test_backup.py'`.
The exercise uses synthetic state, a private network namespace, and no published ports.
It verifies the existing owner, retained window signature, server-held capsule recovery, countdown state, and source isolation.
The [verification record](operations-verification.md#isolated-encrypted-recovery) reports its evidence and limits.

### Backup commands

Keep the source process and all offline writers stopped until `backup create` returns.
Create a separate age X25519 identity through the operator's trusted age tool.
Keep its private recovery copy outside the installation and archive.
Place its single public recipient string in a separate file. Use mode 0600 for both files.
The CLI accepts a raw recipient or identity string with optional surrounding whitespace.
It rejects age key files with comments, multiple keys, links, public permissions, or more than 1024 bytes.
Use trusted parent directories for the source, input, keys, output, destination, and temporary state.
The default finite plaintext limit is 4294967296 bytes. Select a larger limit for a larger supported installation.
Temporary plaintext can require several times that capacity. Keep that storage private.

```sh
epochalypse backup create --source "$STOPPED_ROOT" --output "$NEW_ARCHIVE" \
  --recipient-file "$BACKUP_RECIPIENT" --commit "$SOURCE_COMMIT" \
  --max-objects "$SOURCE_MAX_OBJECTS" --quota-bytes "$SOURCE_QUOTA_BYTES" \
  --max-bytes 4294967296 --temp-dir "$PRIVATE_TEMP"

epochalypse backup restore --input "$NEW_ARCHIVE" --destination "$FRESH_ROOT" \
  --identity-file "$BACKUP_IDENTITY" --commit "$EXECUTABLE_COMMIT" \
  --uid "$MAIN_UID" --gid "$MAIN_GID" \
  --max-bytes 4294967296 --temp-dir "$PRIVATE_TEMP"
```

Use the source's actual effective quotas. Retain its tested executable separately with the exact commit.
The restore command requires that commit to match the authenticated archive.
Both commands report JSON with `commit`, `max_objects`, and `quota_bytes` after success.
Use those quotas for the restored main process's `--max-objects` and `--quota-bytes` flags.
Neither command starts the main process.
The create command syncs encrypted output before an exclusive rename to its fresh path.
It syncs the parent directory before reporting success. Existing output remains unchanged.
If that final sync or result output fails, a complete archive can exist despite the error.
Inspect the path before a retry. The command never replaces it.
The restore command has the same fresh-path and final-sync rules for the installation directory.

Verified interface: [age v1.3.2 source](https://github.com/FiloSottile/age/blob/v1.3.2/age.go), retrieved 2026-11-05.
`Encrypt` requires the caller to close its writer. `Decrypt` returns a plaintext stream that authenticates through its final chunk.
Verified interface: [age v1.3.2 X25519 parser](https://github.com/FiloSottile/age/blob/v1.3.2/x25519.go), retrieved 2026-11-05.
The CLI uses `ParseX25519Recipient` and `ParseX25519Identity`. It keeps parser input outside error messages.
Verified interface: [bbolt v1.5.0 source](https://github.com/etcd-io/bbolt/blob/v1.5.0/db.go), retrieved 2026-11-05.
Read-write `Open` obtains an exclusive file lock. `Options.Timeout` bounds the wait. `Options.OpenFile` permits an existing-file check.
Verified interface: [x/sys v0.49.0 source](https://cs.opensource.google/go/x/sys/+/v0.49.0:unix/), retrieved 2026-11-05.
The pinned Unix package supplies exclusive directory rename operations for Linux and macOS.

For a coherent snapshot, block external access and stop the main process with SIGTERM through its service manager.
Wait for process exit and database lock release. Stop all offline CLI writers before copying either database.
Copy both complete databases and all referenced configuration and key files while they remain unchanged.
Do not copy live database files or mix snapshots from different stops.
Write the encrypted archive atomically. Sync the archive and its parent directory before reporting success.
Verify the authenticated archive before extraction. Keep the service stopped until the snapshot completes or fails safely.
Resume the original deployment only after the copy ends. Record any unavailable interval.

For an isolated restore, use a new directory or fresh Linux volume with no original deployment mount.
Use a private container network without published ports or external egress. Keep synthetic restore names outside public delegation.
Restore regular files only. Reject links, path traversal, duplicate paths, unexpected files, and unsupported formats before installation.
Use mode 0700 for directories and 0600 for databases and private material, owned by the restored service identity.
Start the same commit with paths redirected to the isolated files. Keep both zones and DNSKEY bindings unchanged.
For restored state, bypass all material and owner bootstrap jobs. Run the main executable directly against the validated files.
Do not run `lab_owner.sh` or `owner init` against the restored owner database.
Without its excluded marker, the lab bootstrap attempts initialization and correctly rejects the existing owner.
Issue #42 must validate the archive, required database files, format versions, and ownership before it starts the isolated main process.
Startup must open both databases successfully. Then verify the restored owner identity through an authenticated owner read.
Use the synthetic owner's existing credential for that read. Never use a marker's presence as evidence of a valid database.
The production Compose template has no bootstrap job. The local Compose template requires a direct startup that bypasses its dependencies.
After validation, use this direct startup shape for the canonical archive paths in a fresh restored volume:

```sh
docker run --rm --name isolated-restored-main --network none \
  --user "$MAIN_UID:$MAIN_GID" --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true --tmpfs /tmp \
  --mount type=volume,source="$RESTORED_VOLUME",target=/restore \
  "$MAIN_IMAGE" server --environment lab \
  --state-dir /restore/state --config /restore/config/main.json \
  --http-addr 0.0.0.0:8080 --dns-addr 0.0.0.0:1053 \
  --key-file /restore/keys/window.private \
  --countdown-key-file /restore/keys/countdown.private \
  --tls-cert /restore/tls/fullchain.pem --tls-key /restore/tls/private.pem
```

This command applies only to the isolated synthetic test. Set its image to the archived source commit's tested image.
Issue #42 supplies archive validation and the fresh volume. The direct command starts no bootstrap job and publishes no port.
Run test clients inside the same network namespace. Preserve TLS hostname verification with the restored synthetic certificate.
Verify HTTPS, commit, both DNS transports, retained identifiers/signatures, capsule authenticated recovery, and countdown behavior inside that isolated network.
Use synthetic released content for this test. Never expose restored owner sessions, tokens, or unreleased keys to public traffic.
This procedure makes no live-agent monotonic-state or switch recovery claim.

## Release checks and evidence

Check HTTPS with hostname verification, `/healthz`, `/version`, both direct DNS transports, and local anchors before release.
`/healthz` proves process liveness only. `/readyz` and the later operations scripts are not implemented in this deployment slice.
Use feature reads and an isolated backup/restore exercise for the applicable state checks.
Promote `dev` to `main` only through the separate owner-approved step. This guide grants no production promotion approval.

Verified interfaces, retrieved 2026-11-05:

- [Docker Compose services](https://docs.docker.com/reference/compose-file/services/) and installed Compose 5.6.1 help: dependencies, limits, volumes, and ports.
- [Docker statistics](https://docs.docker.com/reference/cli/docker/container/stats/) and installed help: one-sample resource observations.
- [systemd 257 execution options](https://github.com/systemd/systemd/blob/v257/man/systemd.exec.xml): identity, filesystem access, and bind capability.
- [systemd 257 resource controls](https://github.com/systemd/systemd/blob/v257/man/systemd.resource-control.xml): memory, CPU, and task limits.
- [systemd 257 service options](https://github.com/systemd/systemd/blob/v257/man/systemd.service.xml): startup, restart, and stop timeout.
- [BIND 9.20.21 manuals](https://bind9.readthedocs.io/en/v9.20.21/manpages.html): `dig`, `delv`, and `dnssec-dsfromkey -2`.
- [RFC 4035](https://www.rfc-editor.org/rfc/rfc4035.html), sections 2.4 and 5: parent DS coordination and validation.
- [Node.js crypto](https://nodejs.org/api/crypto.html): the existing pinned synthetic material generator uses v26.13.0.

The [local verification record](operations-verification.md) reports the measured example and its limits.
