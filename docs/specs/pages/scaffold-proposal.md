---
id: scaffold-proposal
title: Scaffold proposal
---

# Scaffold proposal

## Approval boundary

The owner approved these files on 2026-11-04. The planner created the approved scaffold.
The planner keeps `.codex/` unchanged. No hook is proposed for v1.
Required checks run through `make check`, CI, and the test skill.
This avoids a dependency on a hook format or a tool that has not been verified.

## Proposed files

| File | Purpose |
|---|---|
| `AGENTS.md` | State the spec path, branch model, commands, rules, and issue filing gate. |
| `docs/rules/ste.md` | Carry the supplied prose standard with the project Terms pointer. |
| `docs/rules/quality.md` | Require test-first behavior changes, coverage, safe boundaries, and measurable source checks. |
| `docs/rules/external-apis.md` | Require version-specific primary documentation before external calls. |
| `.agents/skills/run/SKILL.md` | Start the lab through `make lab` and verify `/healthz`. |
| `.agents/skills/test/SKILL.md` | Run `make check` and report each required check. |
| `docs/adr/0000-template.md` | Provide the decision format. |
| `docs/adr/0001-release-trust.md` | Record validation limits and independent switch agents. |
| `docs/external.md` | Record configuration names and external documentation pointers without values. |
| `.gitignore` | Ignore local state, secrets, build output, and worktrees. |

Use Codex instructions only for the initial scaffold. Add Claude pointers later if requested.
Product source, Makefile targets, CI workflows, and deployment scripts belong to Epic 0 or later implementation epics.
The planner does not represent these planned commands as runnable today.

## Proposed quality rules

- Every required check passes before merge.
- Aggregate first-party Go statement coverage reaches 95.00 percent.
- Behavior changes include fail/pass evidence from TDD.
- Every security boundary has an explicit assertion.
- Complexity above 15 requires refactoring before merge.
- Nesting above four levels requires refactoring before merge.
- The lint check rejects unused first-party code.
- The race check reports no race.
- External calls use bounded timeouts and response sizes.
- Errors preserve context without secret values.
- Protocol changes preserve versioned fixtures or include a reviewed migration.
- The switch remains unavailable for production until the protocol review passes.

## Proposed filing gate

A finding earns a tracker issue for behavior, user-visible output, an ineffective guard, a blocked epic, or a maintainer decision.
A spec that describes the wrong product earns a spec-update issue.
Repair other findings in the change set that finds them.
During issue work, keep the repair in that issue's PR and its touched files.
During batch review, commit the documentation repair to the batch branch before the CI-trigger commit.
With no open work, collect documentation repairs in one maintenance branch into `dev`.
If a repair changes behavior, stop the repair and file a behavior issue.
Record decisions at batch gates. Do not require a changelog row for each member change.

## Proposed ignore entries

```gitignore
# issue-flow
.worktrees/
.issue-flow.local.json
# local state and secrets
var/
config/local.json
.env
.env.*
!.env.example
*.key
# build and checks
bin/
coverage.out
coverage.html
.test-artifacts/
.DS_Store
```

Keep `docs/specs/`, its generated HTML, and its vendored assets tracked.

## Repository setup

The owner approved Git initialization on 2026-11-04.
The local repository uses `main` and `dev`. The public remote is `Crank-Git/epochalypse` on GitHub.
The personal GitHub repository name and remote remain setup values.
The package must reach the public remote before issue creation.
