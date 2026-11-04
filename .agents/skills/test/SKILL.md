---
name: test
description: Run the epochalypse checks. Use when asked to test a change or verify the project.
---

# Test the project

1. Read the test strategy in `docs/specs/spec.md`.
2. Read `docs/rules/quality.md`.
3. Check that Epic 0 has implemented the planned commands.
4. If the commands are absent, report that product tests do not exist yet.
5. Run `make test-unit`.
6. Run the integration, browser, and resolver tests that apply to the change.
7. For an integration pull request, run `make check`.
8. Report each command and its result.
9. Report the measured coverage separately from the test pass rate.

Every required test passes. The first-party Go coverage threshold is 95.00 percent before rounding.
Missing tests or commands cannot count as a passing suite.
Run fail/pass checks for behavior changes before the final verification.
