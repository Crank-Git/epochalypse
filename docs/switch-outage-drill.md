# Switch outage drill

This isolated Linux drill verifies AC-operations-4, AC-switches-10, and AC-switches-15 against the switch runtime.
Run `scripts/recovery-drill.sh` from the repository root.
From another directory, use the command's absolute path.
The command accepts no arguments except `--help`. It rejects target addresses before it creates local state.
It sets repository-local Go, temporary, XDG, and Buildx cache paths before it executes `scripts/test_switch_arm.py`.
It preserves the runner's exit status. The runner requires all three outage schedules without skipped tests.
The runner requires Docker and Go 1.27.3.
`make test-e2e` also executes the same runner.

Run this synthetic software check after the [deployment readiness checks](readiness.md).
Retain the JSON and log described below with the tested commit's verification record.
The command creates its own isolated processes. It accepts no deployment inventory and stops no deployed service.
Live switch exercises require separate protocol acceptance and actual-host evidence.

The container has no external network. Its repository mount is read-only.
It uses random loopback ports and temporary owner state.
An anonymous Docker volume holds separate agent journals on Linux ext4.
The runner also executes the physical reservation tests on that volume.
It does not use the owner's HTTPS 8080 or DNS 1053 listeners.
It removes only its container and temporary fixture files.

## Schedule

`TestSwitchOutageRecoversAfterMainAndEachAgentStop` repeats the schedule for each unavailable agent.

1. Create synthetic content through the production CLI.
2. Prepare, export, and arm through three production HTTPS agent processes.
3. Restart each agent through the test executable with the production service and publisher.
4. Start the production main process against the armed owner state.
5. Verify its exact `/version` commit and `/healthz` response through the production status CLI.
6. Confirm one signed renewal through three durable receipts.
7. Send another signed renewal directly to the agents.
8. Discard all three responses after their handlers accept the renewal.
9. Verify the uncertain outcome, B0, Bmax, and absent receipts.
10. Retry from a new owner process.
11. Verify identical journaled request bytes and unchanged bounds.
12. Stop the main process and the selected agent before release.
13. Verify that both listeners refuse connections.
14. Check irreversible unsafe-clock quarantine on the unavailable agent.
15. Remove the owner key, private plan, plaintext, main state, and client journal from their original paths.
16. Restart the two surviving agents from their independent journals.
17. Verify that standalone recovery returns no plaintext before release.
18. Advance the controlled clock to Bmax plus two seconds.
19. Wait six real seconds without a public request.
20. Stop the surviving agents and verify their durable release latches.
21. Restart them with an invalid clock source.
22. Recover authenticated plaintext through the production standalone CLI.
23. Check that the elapsed work fits the remaining 32 seconds.
24. Retain signed public manifests and ciphertext through HTTPS.
25. Stop every remaining agent endpoint.
26. Recover from the two retained, verified public shares.

The unavailable agent first restarts with an invalid clock sample.
Its journal must retain the unsafe-clock latch.
A later restart with a valid sample must still refuse readiness.
The drill does not count that agent toward recovery.

The original private files remain in the temporary fixture with a `.lost` suffix.
Only public bundle, owner public key, certificate trust, and endpoint pins reach the recovery CLI.
No recovery argument references the main server, DNS, owner key, or private plan.
The main process remains stopped throughout publication and recovery.

## Clock and allowance

The test executable supplies clock injection. The production executable exposes no clock override endpoint.
The helper uses the production `OpenService`, HTTP handler, journal, and `RunPublisher`.
The synthetic clock has a configured UTC origin and advances with real elapsed wall time.
The drill assumes that the local wall clock does not step during each short observation.
Its UTC jump is explicit test control. It is not evidence of a verified clock epoch on a real host.
The runtime still subtracts the candidate epsilon of two seconds before a release decision.

The design ceilings are epsilon=2 seconds, S=5 seconds, and W=25 seconds.
The candidate allowance is E=34 seconds. ADR 0013 does not accept a numeric production allowance.
The drill starts its stopwatch before it sets synthetic UTC to Bmax+2.
It requires publication, durable restart, and authenticated CLI recovery within another 32 real seconds.
The log reports the measured elapsed time and the conservative total after Bmax.
The scheduler must persist release before a public request can trigger publication.

The interval remains at the production default of 24 hours.
The test advances synthetic UTC instead of shortening the permitted interval or waiting one day.
No main process, owner process, or inter-agent request authorizes release after the outage.

## Evidence and limits

The runner writes `.test-artifacts/switch-outage.log` and `.test-artifacts/switch-outage.json`.
The JSON records the exact commit, executable hashes, Go version, Docker version, architecture, pinned image, source hashes, and measured results.
The runner compares source hashes before and after the drill.
It fails if a required outage subtest is absent or skipped.
The regular aggregate coverage check includes the instrumented CLI executions.

This change adds verification without changing production behavior.
Existing runtime behavior therefore passes the new verification before any production patch.
A controlled negative run withholds release eligibility and must fail the scheduler assertion.
That check establishes drill sensitivity. It does not constitute a new protocol implementation or a production regression fix.

Local processes and containers do not prove independent provider failure domains.
These observations do not certify actual-host clock error, publication latency, physical infrastructure durability, or production readiness.
Production switch use remains gated by the reviewed protocol and actual-host evidence.

Verified interfaces: Go 1.27.3 `go doc net/http.Server.ServeTLS` and the installed Docker `run --help`.
See the [Go HTTP documentation](https://pkg.go.dev/net/http#Server.ServeTLS) and [Docker run reference](https://docs.docker.com/reference/cli/docker/container/run/).
