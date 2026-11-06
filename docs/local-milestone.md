# Local milestone walkthrough

Run every command from the repository root. Use synthetic content only.
The [README](../README.md) gives the prerequisites and default lab startup.

## Enable the three features

The default lab configures the window zone only. This override adds a separate countdown zone and key through supported server flags.
It reuses the existing synthetic material generator in a separate directory.
The generated countdown certificate and password remain unused.
Keep `var/local-milestone/` until you stop this lab; Compose mounts its generated script.

Create the local override:

```sh
mkdir -p var/local-milestone
python3 - <<'PY'
from pathlib import Path
p = Path('var/local-milestone')
p.joinpath('countdown_material.mjs').write_text(
    Path('scripts/lab_material.mjs').read_text()
    .replace("'/state/lab'", "'/state/countdown'")
    .replace('windows.lab.invalid.', 'events.lab.invalid.'))
p.joinpath('compose.yaml').write_text('''services:
  lab-material:
    entrypoint: [/bin/sh, -c, "node /setup/lab_material.mjs && node /setup/countdown_material.mjs"]
    volumes:
      - ../../var/local-milestone/countdown_material.mjs:/setup/countdown_material.mjs:ro
  web:
    command: [server, --dns-addr, "0.0.0.0:1053", --tls-cert, /state/lab/tls.crt, --tls-key, /state/lab/tls.key, --countdown-zone, events.lab.invalid., --countdown-key-file, /state/countdown/window.private]
''')
PY
make lab
docker compose -f deploy/lab/compose.yaml -f var/local-milestone/compose.yaml up --detach --wait --wait-timeout 60
docker compose -f deploy/lab/compose.yaml cp web:/state/lab/tls.crt var/local-milestone/tls.crt
curl --fail --cacert var/local-milestone/tls.crt https://localhost:8080/healthz
curl --fail --cacert var/local-milestone/tls.crt https://localhost:8080/version
docker compose -f deploy/lab/compose.yaml -f var/local-milestone/compose.yaml ps --all
```

Expect `ok` from health and a commit identifier from version.
Readiness means the two setup jobs exit successfully and the main process and three agents report healthy.
The project has no separate readiness endpoint. Agent health does not establish switch release behavior.

If the default ports conflict, set `LAB_HTTP_PORT` and `LAB_DNS_PORT` before startup.
If another lab shares this checkout, also set `COMPOSE_PROJECT_NAME` and `LAB_IMAGE_TAG`.
Keep these variables for every later command. The [lab guide](lab.md#access) describes the port mappings.

## Owner access

1. Open `https://localhost:8080/owner`.
2. Enter the known synthetic password from the [lab guide](lab.md#setup-and-files).
3. Select **Sign in**.
4. Open the capsule and countdown sections from the dashboard.

The setup job creates the lab owner before the server opens its database.
For a separate installation, use the protected password-file procedure in [owner commands](owner.md).
The dashboard links to dedicated capsule and countdown controls. Its summaries do not list retained capsules or countdowns.
The switch section remains blocked. The agents share one host and provide no switch release behavior.

## DNSSEC experiment

The window interface uses an authenticated API or an offline CLI. The dashboard has no window creation form.
This offline example uses the lab's supported CLI:

```sh
docker compose -f deploy/lab/compose.yaml stop web
docker compose -f deploy/lab/compose.yaml run --rm --no-deps web window create \
  --zone windows.lab.invalid. --key-file /state/lab/window.private \
  --content 'Synthetic public experiment' \
  --inception 2030-01-01T00:00:00Z --expiration 2030-01-01T01:00:00Z
docker compose -f deploy/lab/compose.yaml -f var/local-milestone/compose.yaml up --detach --wait --wait-timeout 60
```

Use a future inception and later expiration if these example dates have passed.
Save the returned `id` and `name`. Open `https://localhost:8080/windows/RETURNED_ID`.
If you have `dig`, query the returned name directly:

```sh
dig @127.0.0.1 -p 1053 RETURNED_NAME TXT +dnssec
dig @127.0.0.1 -p 1053 RETURNED_NAME TXT +dnssec +tcp
docker compose -f deploy/lab/compose.yaml exec -T web epochalypse lab verify \
  --zone windows.lab.invalid. --address 127.0.0.1:1053 --anchor-file /state/lab/window.key
```

Expect public TXT content before inception. The verifier checks the explicit local anchor, independently of experimental TXT validity.
A future signature provides no confidentiality. Public delegation requires separate operator work.

## Capsules

1. Open `https://localhost:8080/owner/capsules` after sign-in.
2. Choose **public** mode.
3. Enter synthetic text and a future UTC release time.
4. Select **Create draft**.
5. Select **Publish capsule**.
6. Open `/c/RETURNED_ID` with the returned identifier.
7. Download the content from the public page.

Expect immediate public availability. Public mode treats the requested date as advisory.
Repeat these steps with **server-held** mode. Expect sealed content before the release time.
The server operator holds that mode's key. At release, the server must remain available.

Repeat with **tlock** mode to inspect its effective quicknet round and public metadata.
Web preparation passes plaintext through the server. CLI preparation occurs on the owner's machine.
Future tlock recovery needs the matching verified beacon; the local lab does not produce future beacons.
Use the [capsule guide](capsules.md#export-and-local-recovery) to export a recovery bundle or recover content locally.
Published public copies remain outside owner control after local deletion.

## Countdowns

1. Open `https://localhost:8080/owner/countdowns` after sign-in.
2. Enter a synthetic title and a future UTC event time.
3. Create the countdown.
4. Save its identifier and actual DNS name.
5. Open `https://localhost:8080/events/RETURNED_ID`.
6. Query its actual name with `dig @127.0.0.1 -p 1053 RETURNED_NAME TXT +dnssec`.

Expect the absolute event timestamp in TXT. The authoritative TTL decreases toward zero.
The page computes its display from the absolute event time and a server clock sample.
At the event, the page shows completion. The authority retains TXT with TTL zero until owner deletion.
Recursive TTL values remain advisory. They provide no release guarantee.

## Verification record

The isolated lab uses source commit `48315fa9bdfe9db331b41debdefe2e0eb85c9fbb` on 2026-11-05.
The check uses HTTPS port 18081 and DNS port 11054 with project `epochalypse-worker121`.
The walkthrough's default ports remain 8080 and 1053.

| Check | Actual result |
|---|---|
| Health and setup | HTTPS health returns 200 with `ok`; both setup jobs exit 0; four processes report healthy. |
| Version | `/version` returns the tested source commit. |
| Owner | CSRF-protected sign-in returns 204; the dashboard returns 200. |
| Window | Creation returns 201; UDP/TCP serve signed TXT before inception; a 512-byte fixture triggers explicit TCP fallback; public page returns 200; local-anchor verification returns `verified`. |
| Public capsule | Draft returns 201; publication and page return 200; authenticated download matches the synthetic input. |
| Server-held capsule | Publication succeeds; the early key request returns 425. |
| Tlock capsule | Preparation and publication succeed; public page returns 200; live future-beacon release remains untested. |
| Countdown | Creation returns 201 under `events.lab.invalid.`; public page returns 200; metadata reports 3599 remaining seconds; a later signed DNS answer reports TTL 3525. |

The required browser harness verifies release boundaries with a fake clock and the signed quicknet round 1000 fixture.
Those controls exist only in the test executable. The production server exposes no clock or beacon override.
The resolver harness checks countdown validation and completed TTL zero through default Unbound, strict Unbound, and BIND.
See [required checks](checks.md) for commands, fixtures, and retained result artifacts.

## Stop and deployment limits

Stop this lab and preserve state:

```sh
docker compose -f deploy/lab/compose.yaml -f var/local-milestone/compose.yaml down
```

The [lab reset procedure](lab.md#health-and-reset) deletes synthetic state when you explicitly need a fresh owner or certificate.
`make demo` creates offline synthetic fixtures; it does not publish this walkthrough's feature records into the running lab.

This milestone verifies local behavior. It does not verify a production deployment, independent agent hosts, or public DNS delegation.
Production requires protected credentials, valid TLS, separate DNS zones and keys, clock maintenance, and verified recovery endpoints.
Use the [deployment guide](operations.md) before production configuration.
Switch runtime software is now implemented and verified in the isolated Linux harness. This walkthrough still uses healthy agent scaffolds. Actual-host verification and production promotion remain deferred.
