---
name: run
description: Start epochalypse locally for manual checks. Use when asked to run, launch, or inspect the project.
---

# Run the project

1. Read the environment steps in `docs/specs/spec.md`.
2. Check that Epic 0 has implemented the planned commands.
3. If the commands are absent, report that implementation has not started.
4. Start the local lab with `make lab`.
5. Verify HTTP 200 at `http://localhost:8080/healthz`.
6. If synthetic content is useful, run `make demo`.
7. Report the local URL and any failed readiness check.

Use synthetic content for manual checks.
The main server alone starts through `make run` after the foundation implements that target.
