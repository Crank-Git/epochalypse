---
name: epochalypse
slug: epochalypse
repo: null
status: approved
spec_version: 2
created: 2026-11-04
approved: 2026-11-04
html_generated: 2026-11-04
branch_model: dev-and-live
visibility: public
features:
  - id: foundation
    file: features/00-foundation.md
  - id: dns-window
    file: features/01-dns-window.md
  - id: owner
    file: features/02-owner.md
  - id: capsules
    file: features/03-capsules.md
  - id: switch-protocol
    file: features/04-switch-protocol.md
  - id: switches
    file: features/05-switches.md
  - id: countdowns
    file: features/06-countdowns.md
  - id: operations
    file: features/07-operations.md
  - id: research
    file: features/08-research.md
pages:
  - id: research-evidence
    file: pages/research-evidence.md
  - id: scaffold-proposal
    file: pages/scaffold-proposal.md
---

# epochalypse

## Overview

epochalypse provides time capsules, renewable dead man’s switches, and DNS countdowns.
Developers use a Go CLI and a resolver lab. Individuals use an owner dashboard and public release pages.
The project supports local containers and Linux VPS hosts.

DNSSEC signature windows demonstrate validator behavior. They do not keep published data secret.
Encrypted capsules use server-held keys or a verified drand beacon.
Independent agents support switch release after the main server fails.

The owner states: “outage shouldnt stop this as it undermines the entire deadman switch behavior.”
The initial guarantee covers main-server loss plus one unavailable agent.
A separate research epic assesses loss of all project-operated servers.

## Terms

| Term | Part of speech | Meaning in this project | Do not use |
|---|---|---|---|
| project | noun | The epochalypse software and its specifications. | app |
| owner | noun | The single administrator of an installation. | tenant |
| visitor | noun | A person who reads public content. | anonymous owner |
| agent | noun | An independent service that stores switch state and a key share. | worker |
| capsule | noun | Content with a fixed release time. | vault |
| switch | noun | Content with a renewable release deadline. | capsule when renewal applies |
| countdown | noun | A public event time with an advisory DNS TTL. | timer |
| window | noun | A DNSSEC signature inception and expiration pair. | secrecy lock |
| deadline | noun | The latest accepted owner check-in boundary. | expiration |
| grace | noun | The duration between the deadline and release eligibility. | timeout |
| release boundary | noun | The deadline plus grace for one accepted state. | deadline alone |
| share | noun | One agent's part of the content key. | whole key |
| receipt | noun | A signed acknowledgment of durable agent state. | success hint |
| recovery bundle | noun | Exported data needed for independent content recovery. | backup alone |
| manifest | noun | Versioned public content and release metadata. | payload |
| check-in | noun | A signed owner request to extend a switch deadline. | DNS query |
| ciphertext | noun | Encrypted content. | hidden TXT |
| beacon | noun | A publicly verifiable drand round signature. | arbitrary randomness |
| DNS | noun | Domain Name System. | name database |
| DNSSEC | noun | DNS Security Extensions. | encryption |
| RRSIG | noun | A DNS record that carries a signature and its time window. | key |
| RRset | noun | DNS records with one owner name, class, and type. | arbitrary record group |
| TXT | noun | The DNS text record type. | text file |
| TTL | noun | Time to live, in seconds, in a DNS response. | release guarantee |
| AD | noun | Authentic Data flag in a DNS response. | secrecy flag |
| CD | noun | Checking Disabled flag in a DNS query. | lock bypass exploit |
| UTC | noun | Coordinated Universal Time. | local stored time |
| CLI | noun | Command-line interface. | terminal dashboard |
| API | noun | Application programming interface. | unspecified endpoint |
| HTTP | noun | Hypertext Transfer Protocol. | transport security |
| HTTPS | noun | HTTP protected by TLS. | plain HTTP |
| TLS | noun | Transport Layer Security. | DNSSEC |
| CSRF | noun | Cross-site request forgery. | authentication |
| VPS | noun | Virtual private server. | managed service guarantee |
| CI | noun | Continuous integration checks. | coverage alone |
| TDD | noun | Test-driven development through fail, pass, and refactor steps. | tests after behavior only |
| E2E | noun | End-to-end verification through public interfaces. | unit test |
| create | verb | Make a new entity. | provision when entity creation applies |
| arm | verb | Activate a switch after the required durable receipts. | publish before ready |
| renew | verb | Extend a switch through the reviewed check-in protocol. | reset |
| release | verb | Make sufficient recovery material public. | delete |
| recover | verb | Retrieve and decrypt content through the recovery path. | restore agent state |
| delete | verb | Remove a local copy under owner control. | recall public copies |

## Goals

- Provide all three variants through a CLI and web pages.
- Demonstrate future-signature behavior through real validating resolvers.
- Distinguish validation, confidentiality, and availability in every release mode.
- Recover switch content after main-server loss plus one agent outage.
- Pass every required test before merge.
- Maintain at least 95.00 percent aggregate statement coverage for first-party Go code.
- Record a reproducible total-loss research result.

## Non-goals

V1 excludes multiple owner accounts, billing, and a managed hosted service.
Future work includes multiple accounts with separate dashboards.
V1 excludes exact-time acceptance by arbitrary resolvers and secrecy from public RRSIG windows.
V1 does not guarantee switch recovery after every agent disappears.
V1 does not claim resistance to collusion by two agents or a compromised preparation client.

## Users & personas

| Persona | Need | Permission |
|---|---|---|
| Owner | Create content and maintain switches. | Authenticated administration and signed check-ins. |
| Visitor | Read released content. | Public read and recovery only. |
| Researcher | Compare resolver behavior. | Owner-created lab experiments. |
| Agent operator | Maintain an independent release service. | Agent host administration, not owner administration. |

## Feature map

| Feature | Spec | Epic | Mockup |
|---|---|---|---|
| Foundation | [foundation](features/00-foundation.md) | Epic 0: Foundation | None |
| DNSSEC signature windows | [dns-window](features/01-dns-window.md) | Epic 1: DNSSEC lab | [dns-window](mockups/dns-window.html) |
| Owner access | [owner](features/02-owner.md) | Epic 2: Owner access | [owner](mockups/owner.html) |
| Time capsules | [capsules](features/03-capsules.md) | Epic 3: Time capsules | [capsules](mockups/capsules.html) |
| Switch protocol review | [switch-protocol](features/04-switch-protocol.md) | Epic 4: Switch protocol review | None |
| Dead man’s switches | [switches](features/05-switches.md) | Epic 5: Dead man’s switches | [switches](mockups/switches.html) |
| DNS countdowns | [countdowns](features/06-countdowns.md) | Epic 6: DNS countdowns | [countdowns](mockups/countdowns.html) |
| Deployment and recovery | [operations](features/07-operations.md) | Epic 7: Deployment and recovery | None |
| External release research | [research](features/08-research.md) | Epic 8: External release research | None |

Mockups communicate interface intent. Functional requirements and acceptance criteria define behavior.

## Architecture & stack

```mermaid
flowchart LR
 Owner[Owner CLI or browser] --> Main[Main Go server]
 Owner -->|Signed check-in| Agents[Three independent agents]
 Main --> DNS[Authoritative DNS server]
 DNS --> Resolver[Validating resolver]
 Resolver --> Visitor[Visitor]
 Main --> Store[Durable local state and ciphertext]
 Main -->|Initial replication| Agents
 Visitor -->|Recovery bundle| Agents
 Visitor --> Beacon[Verified drand beacon]
 Agents -->|Two release shares and ciphertext| Visitor
```

Use Go 1.27.3 with standard HTTP, templates, JSON, and CLI flags.
The [Go release history](https://go.dev/doc/devel/release) documents this version and its security fixes.
Use server-rendered HTML with a small local JavaScript layer. The web interface requires no external frontend service.
Use `cmd/epochalypse` for the executable and `internal/` for domain modules.
Separate modules cover authentication, DNS, capsules, switches, agent state, content, and the clock.
Use injected clocks for deterministic tests. Use monotonic durations for local scheduling, with explicit UTC release boundaries.

Evaluate [miekg/dns v1.1.73](https://github.com/miekg/dns/tree/v1.1.73) through a compatibility spike.
Its README describes DNSSEC support but directs new work toward a successor.
The spike chooses a maintained adapter and records its exact version before product code uses it.
Use [tlock v1.2.0](https://github.com/drand/tlock/tree/v1.2.0) as the initial compatibility target.
Verify quicknet configuration explicitly. Legacy examples contain conflicting network references.
The durable store, password library, threshold library, and lint tools remain foundation decisions with version-specific documentation.
The spec does not assume their unverified interfaces.

Use provider-neutral Linux services and container examples. Managed platforms must meet port and state requirements.
Each agent provides its own public recovery endpoint and durable ciphertext copy.
The main server is absent from the switch recovery path.

## Data model

```mermaid
erDiagram
 OWNER ||--o{ TOKEN : authorizes
 OWNER ||--o{ CAPSULE : creates
 OWNER ||--o{ SWITCH : maintains
 OWNER ||--o{ COUNTDOWN : creates
 CAPSULE ||--|| OBJECT : references
 SWITCH ||--|| OBJECT : references
 SWITCH ||--|{ RECEIPT : records
 AGENT ||--o{ RECEIPT : signs
 AGENT ||--o{ SHARE : stores
 SWITCH ||--|{ SHARE : protects
 WINDOW ||--o{ OBSERVATION : produces
```

| Entity | Fields and constraints | Lifecycle |
|---|---|---|
| Owner | Random ID, password hash, creation time; exactly one. | Initial setup through local recovery. |
| Token | ID, digest, scopes, expiry, revocation time. | Active to expired or revoked. |
| Capsule | ID, title, mode, requested time, effective time, object digest, format version. | Draft, sealed, available, deleted. |
| Object | Digest, byte count, media type, encrypted bytes, format version. | Prepared to retained or locally deleted. |
| Switch | ID, owner public key, interval, grace, sequence, accepted state, object digest. | Draft, armed, degraded, released. |
| Agent | ID, public key, endpoints, failure domain, protocol version. | Enrolled to ready, unavailable, or retired. |
| Share | Switch ID, agent ID, encrypted share, release status. | Private to irreversibly public. |
| Receipt | Agent ID, switch ID, sequence, deadline, digest, signature. | Append-only evidence. |
| Countdown | ID, title, event time, DNS name. | Active, completed, deleted. |
| Window | ID, zone, content, inception, expiration, key reference. | Draft, published, expired. |
| Observation | Window ID, resolver version, clock offset, query flags, response, observation time. | Immutable lab result. |

Use UTC integer seconds for protocol times. Use bounded integer durations for intervals and grace.
Keep owner content keys separate from public metadata. Give each agent a distinct share.
Version persistent formats. Validate migrations against preserved fixtures before rollout.

```mermaid
stateDiagram-v2
 [*] --> Draft
 Draft --> Armed: All durable receipts and export
 Armed --> Armed: Confirmed renewal
 Armed --> Degraded: Missing receipt
 Degraded --> Armed: Reviewed recovery
 Armed --> Released: Release boundary
 Degraded --> Released: Reviewed release rule
 Released --> [*]
```

## Cross-cutting concerns

Authenticate mutations. Authorize each token scope. Sign direct check-ins with an owner key.
Use TLS outside the isolated local lab. Require anti-replay checks on agent messages.
Use generic authentication errors and specific validation errors.
The API error shape is `{error:{code,message,fields,request_id}}`.
Use 400 for malformed input, 401 for missing authentication, and 403 for insufficient scope.
Use 404 for absent public objects, 409 for state conflicts, 413 for size limits, and 429 for rate limits.
Use 503 for an unavailable dependency. Use 425 for a server-held key before release.
Use request IDs across logs. Exclude plaintext, passwords, tokens, private keys, and key shares from logs.
Bound request time, memory, disk quotas, and concurrent work. Default to 100 retained objects and 1 GiB per installation.
The owner can change storage quotas. Failed quota checks leave no armed switch.

Use keyboard-accessible controls, semantic labels, visible focus, and textual state labels.
Support responsive light and dark layouts. Use English copy in v1.
Show UTC with a local-time display. Never derive release authority from a visitor's browser clock.
Target a local p95 below 250 ms for metadata reads at 20 requests per second on a two-core host.
Exclude uploads and cryptographic preparation from that latency target. Record the benchmark host and workload.
Treat this target as a benchmark gate before release, not a claim about arbitrary VPS hosts.

## Environments & config

Environments are `lab`, `test`, `staging`, and `production`.
Synthetic demo content uses the `demo` namespace. CI never needs real owner credentials.
Use a versioned configuration file for structured agent and DNS settings.
Use these environment variables for process-level configuration:

| Name | Purpose | Default |
|---|---|---|
| EPOCHALYPSE_ENV | Select environment. | lab |
| EPOCHALYPSE_CONFIG | Locate the relative configuration file. | config/local.json |
| EPOCHALYPSE_STATE_DIR | Locate durable state. | var/state |
| EPOCHALYPSE_HTTP_ADDR | Bind the web listener. | 127.0.0.1:8080 |
| EPOCHALYPSE_DNS_ADDR | Bind UDP and TCP DNS. | 127.0.0.1:1053 |
| EPOCHALYPSE_PUBLIC_URL | Set the public origin. | http://localhost:8080 in lab |
| EPOCHALYPSE_AGENT_ID | Select an agent identity. | Required for agent mode |
| EPOCHALYPSE_KEY_FILE | Locate a protected key file. | Required when a private key applies |
| EPOCHALYPSE_MAX_UPLOAD_BYTES | Limit plaintext content. | 10485760 |
| EPOCHALYPSE_MAX_OBJECTS | Limit retained objects. | 100 |
| EPOCHALYPSE_QUOTA_BYTES | Limit retained local content. | 1073741824 |
| EPOCHALYPSE_LOG_LEVEL | Set log detail. | info |

Configuration holds the zone name, agent endpoints, failure domains, public keys, beacon chain, and protocol version.
Private key files require owner-only filesystem access. Configuration contains key references rather than key values.
Production startup rejects lab keys and insecure public origins.
Use a small synthetic capsule, switch, countdown, and window as demo records.
Real domains, VPS addresses, and the personal GitHub identity remain operator setup values.
No provider budget or spending authority exists.

## Testing strategy

Required pass rate is 100 percent. Coverage is a separate minimum of 95.00 percent.
Measure Go statement coverage across every first-party production package, including CLI and HTTP adapters.
Exclude dependencies and generated third-party code. Do not exclude difficult branches to reach the threshold.
Fail the coverage check when the profile is absent, empty, malformed, or below the unrounded threshold.
Require explicit tests for every security and deadline boundary regardless of coverage.

Use TDD for behavior changes. First demonstrate a failing assertion against the existing behavior.
Then implement the smallest change that passes. Refactor while the checks remain green.
A new test that fails only because of a syntax error does not demonstrate the behavior.
Record the fail/pass commands in the PR description without mandatory changelog edits.

Planned commands become executable in Epic 0:

| Command | Required result |
|---|---|
| `make test-unit` | Unit tests pass with fake clocks and temporary state. |
| `make test-integration` | Real listeners, durable stores, and three agents pass. |
| `make test-e2e` | Browser and CLI happy paths plus critical errors pass. |
| `make test-resolvers` | Pinned default Unbound, strict Unbound, and BIND scenarios pass. |
| `make test-race` | `go test -race ./...` reports no races. |
| `make coverage` | Instrumented aggregate Go coverage reaches 95.00 percent. |
| `make fuzz-smoke` | Bounded parser and protocol fuzz tests pass for 30 seconds each. |
| `make lint` | Format, vet, complexity, nesting, dead-code, and security checks pass. |
| `make build` | Linux amd64 and arm64 builds succeed. |
| `make check` | All required checks above pass. |

Unit tests cover validation, authorization, TTL arithmetic, round selection, and state transitions.
Integration tests cover durable receipts, restarts, uploads, replication, and verified decryption.
E2E tests cover owner setup, sign-in, creation, publication, check-in, and independent recovery.
Test empty, active, unavailable, released, invalid, and unauthorized states.
Resolver tests cover trust chains, CD, direct queries, skew, bogus caches, expiry, truncation, and TCP.
Test clocks before, at, and after each boundary. Test backward and forward clock changes.
Test partial renewal, late renewal, lost replies, stale backups, repeated requests, and parallel check-ins.
Test corrupted ciphertext, signatures, shares, and beacon responses without partial plaintext output.
Stop the main server and one agent before the switch boundary. Recover through the remaining agents.
Live public beacon probes remain optional diagnostics. Deterministic verified fixtures gate CI.
Separate scheduled extended fuzz and fault runs from required short PR checks.
A skipped required test fails CI. Retries cannot turn a flaky first failure into a passing required result without diagnosis.

## Epics

Each epic maps to one integration batch. The estimates describe three to six focused changes per batch.
Research can return a blocker without enabling unsafe production behavior.

| Epic | Estimated focused days | Depends on | Exit criterion |
|---|---|---|---|
| Epic 0: Foundation | 5 | None | A fresh clone can check, build, and start the lab. |
| Epic 1: DNSSEC lab | 5 | 0 | The resolver matrix demonstrates the public window. |
| Epic 2: Owner access | 4 | 0 | Owner sessions and scoped tokens protect mutations. |
| Epic 3: Time capsules | 6 | 1, 2 | All three modes pass recovery tests. |
| Epic 4: Switch protocol review | 5 | 0 | The model and maintainer decision permit implementation. |
| Epic 5: Dead man’s switches | 6 | 2, 4 | Independent recovery passes the stated outage test. |
| Epic 6: DNS countdowns | 3 | 1, 2 | Signed countdown records pass resolver tests. |
| Epic 7: Deployment and recovery | 5 | 3, 5, 6 | A staged deployment passes the recovery exercise. |
| Epic 8: External release research | 5 | 4 | The report supports a maintainer decision. |

Feature files define each epic's complete behavior. Split an epic before issue creation if the work exceeds six focused changes.
Do not compress the protocol review to preserve an estimate.

## Milestones

1. Establish a green repository and a reproducible DNS demonstration.
2. Provide owner access and all capsule modes.
3. Complete the protocol review before switch implementation.
4. Provide switches and countdowns behind verified behavior gates.
5. Validate the Linux deployment and independent recovery.
6. Present the external release report as a separate decision.

A shippable v1 passes all required checks and the deployment exercise.
A switch production claim additionally requires the accepted protocol review.
`dev` holds integrated work. The owner approves promotion to `main` separately.

## Assumptions

Use epochalypse as the project name. Use the owner's personal GitHub repository when one exists.
Use one owner per installation. Preserve multiple accounts as a future requirement.
Use English web copy and a Go CLI. Use local credentials instead of an external identity provider.
Use 24 hours as the initial switch interval and one hour as the initial grace.
Use three acknowledgments for renewal as the approved initial design, subject to the protocol review.
An unavailable agent can therefore prevent renewal. The owner approved this tradeoff on 2026-11-04.
Use bounded clock error as a protocol prerequisite. The review establishes the supported bound.
Use a 10 MiB content limit and owner-controlled retention.
Keep provider selection and actual infrastructure cost outside this planning decision.

## Risks & open questions

### Review decisions

- The owner approved three-agent renewal and its availability tradeoff on 2026-11-04.
- The owner approved the scaffold proposal on 2026-11-04.
- The owner accepted server exposure during v1 web preparation on 2026-11-04.

### Technical gates

- Partial renewal can leave inconsistent deadlines. The model must establish safe semantics before production switch use.
- Published shares can survive later renewals. The protocol must account for their cumulative disclosure.
- Two colluding agents can disclose a secret early. V1 does not cover this attack.
- Agent clock error can cause early or late release. The protocol review defines a measured bound and an unsafe-clock response.
- A lost signing key can prevent renewal. Export and recovery procedures must state this consequence.
- The DNS adapter requires a maintenance and compatibility decision.
- The tlock adapter requires an explicit verified network configuration and version check.
- The maintained store, threshold library, password library, and lint tools require pinned documentation before code uses them.
- The reference deployment needs independent endpoints and storage, not only three containers on one VPS.
- External networks can stop. Fixed capsules depend on beacon continuity; renewable external release remains research.
- A public repository exposes specs and issues. Never place real secrets or host credentials in it.

No unanswered product question blocks the approved plan. Technical gates block the affected implementation or production claim.

## Changelog

- 2026-11-04: Create the first draft after the product interview. Record public visibility and the dev-and-live branch model.

- 2026-11-04: The owner approved the spec, release tradeoffs, scaffold, and Git initialization. Technical gates remain in force.
