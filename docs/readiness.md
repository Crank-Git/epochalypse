# Deployment readiness

Run `epochalypse readiness --inventory /private/deployment.json` on the operator's machine.
No HTTP route accepts an inventory or performs these probes.
Keep actual host addresses and evidence outside tracked files.

The version 1 inventory contains these fields:

- `version`: 1.
- `owner_public_key`: the pinned Ed25519 public key, encoded as base64.
- `certificate`: the canonical signed enrollment body, encoded as base64.
- `certificate_signature`: the enrollment signature, encoded as base64.
- `main`: the main service description.
- `agents`: exactly three service descriptions, in canonical enrollment order.

Each description contains `id`, `endpoint`, and `tls_spki_sha256`.
The main ID is `main`. Agent IDs and endpoints match the signed enrollment exactly.
Use an HTTPS origin without a path, query, fragment, or user information.
The pin is the hexadecimal SHA256 digest of the certificate's SubjectPublicKeyInfo.
Certificate chain and hostname validation also apply. Use the system trust store or `SSL_CERT_FILE` for a private certificate authority.
Pins establish identity independently of DNSSEC. They do not establish availability.

Each agent also contains `failure_domain`, `domain_evidence`, and optional `failure_domains`.
`failure_domain` names the primary fault boundary under the operator's stated fault model.
`domain_evidence` identifies the operator's host assessment record.
`failure_domains` maps auxiliary dimensions, such as `provider`, `region`, `host`, `network`, and `power`, to declared values.
Shared auxiliary values appear in the report. Different providers and operators are optional.
Missing evidence, unknown primary domains, and shared primary domains prevent inventory readiness.
The report omits evidence text and domain values. It identifies affected agents and dimensions.
Declarations remain operator claims. The command never certifies physical independence.

The decoder limits the inventory to 64 KiB. It rejects unknown fields, duplicate JSON keys, and trailing documents.
It rejects missing agents, repeated endpoints, absent pins, and invalid enrollment signatures.
The inventory preserves the signed certificate format.

## Public runtime metadata

`GET /healthz` returns `ok` for a live process. Liveness alone establishes no release guarantee.
`GET /version` returns the commit, service role, agent ID, and state and protocol format versions.
`GET /readyz` adds current readiness measurements. An unavailable runtime returns HTTP 503.
These routes require no owner session. They omit keys, shares, ciphertext, private preparation data, paths, and clock profile evidence.

An agent checks its open authority, pinned volume marker, checkpoint slots, irreversible safety state, and current clock assessment.
The checks read state without a publication, renewal, receipt, or durable clock decision.
An unsafe clock prevents readiness. A durable release remains available through public recovery without a fresh clock.
Synthetic clock or storage modes report `synthetic: true` and `production_ready: false`.
The legacy `agent --id` scaffold reports unavailable readiness.

The main service checks its configuration and both open stores.
Main-service readiness requires no agent clock profile.
If these checks pass in a non-synthetic production configuration, the main service reports `production_ready: true`.
Lab configurations and local lab mode remain synthetic.
The main service keeps `clock_ready: false` because its wall time has no verified error bound.
Completed-feature operation can proceed with its functional readiness checks.

Probes reuse the existing TLS pin verifier. Each request has a five-second limit and a 4096-byte response limit.
The complete probe has a 20-second limit. Redirects fail the check.
Probes verify role, agent ID, commit syntax, formats, liveness, and consistent readiness metadata.
The report separates `ready`, `production_ready`, and `physical_independence_verified`.
`production_ready` reports conditional software prerequisites for each service role.
It does not certify an actual host or authorize production promotion.
The last field remains false. Actual-host assessments and operator approval remain separate requirements.
Exit status 0 means functional inventory readiness. Exit status 1 means an unavailable or blocked check.
Use `--production` to require the additional production conditions. Synthetic deployments cannot pass that mode.

## Direct public recovery

Run `epochalypse readiness --inventory /private/deployment.json --recovery-bundle /private/recovery.json` after release.
The command checks that the bundle matches the inventory's enrollment.
It calls the existing standalone recovery verifier against pinned public agents.
It reports `public_recovery_verified` and clears recovered content without printing it.
Recovery can pass while other readiness checks remain unavailable. That result still returns exit status 1.
An unreleased or invalid bundle cannot pass public recovery verification.

To retrieve authenticated content, use the existing command:

```sh
epochalypse switch recover --bundle /private/recovery.json \
  --owner-public-key /private/owner.pub --pins /private/pins.json > recovered.bin
```

This path requires no main service or owner session.
Two public agents supply authenticated release shares and ciphertext.
The executable fixture verifies this path after main-state removal and one agent outage.
It also checks recovery through the readiness command without content disclosure.

Verified against Go 1.27.3 shipped documentation for `net/http.NewRequestWithContext` and `encoding/json.Decoder.DisallowUnknownFields`.
See the [HTTP client documentation](https://pkg.go.dev/net/http) and [JSON decoder documentation](https://pkg.go.dev/encoding/json).
