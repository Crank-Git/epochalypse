# Production promotion

Finish software verification on `dev` before deployment work.
Software approval does not approve a host, domain, or production promotion.
The owner approves a separate pull request from `dev` to `main`.
The status command performs read-only requests. It never creates that pull request, merges code, or changes a provider.

## Prepare the review record

Keep this record and actual host evidence outside tracked files.
Record a full lowercase 40-character commit from `dev` as `tested_commit`.
Use a clean checkout. Record the immutable executable or image digest built from that exact commit.
Retain the build command and its source identity. A branch name or mutable image tag is insufficient.

1. Record the successful `Required checks` run URL and its exact source commit.
2. Record `make check` results, the unrounded coverage totals, and zero skipped required tests.
3. Retain the isolated backup/restore report for the applicable features.
4. For live switches, retain the accepted protocol review and synthetic recovery report.
5. Record each persistent format and retain the versioned compatibility fixtures.

CI must test the proposed promotion merge as well as the source artifact.
If the source commit or promotion diff changes, repeat verification and request approval again.
Keep `dev` at the reviewed source commit until the promotion completes.

## Verify the staging deployment

Use the tested immutable artifact on the operator-supplied staging hosts.
This procedure authorizes no host purchase, domain registration, or provider change.
For the complete switch deployment, run these commands with the actual inventory:

```sh
scripts/deploy-status.sh --inventory /private/deployment.json --production TESTED_SHA
scripts/deploy-status.sh --inventory /private/deployment.json --production --json TESTED_SHA > /private/status.json
epochalypse readiness --inventory /private/deployment.json --production > /private/readiness.json
```

Replace `TESTED_SHA` with `tested_commit`. Put flags before the SHA.
Require `success local TESTED_SHA` from the first command.
Require four matching service commits and verified identities in the retained JSON.
Require conditional production readiness on every required service.
The status command exits zero for observed failures and unknown results. Inspect the result, not only its exit code.

Review actual-host DNS delegation, DNSKEY and parent DS evidence.
Verify DNS over UDP and TCP port 53 and HTTPS recovery on port 443.
Review durable storage, per-service identities, resource limits, and private control access.
For each live agent, review the clock profile against its actual host, current kernel, storage, and clock sources.
Assess three physical failure domains under the stated fault model.
Review shared provider, region, host, network, and power declarations.
`physical_independence_verified` remains false in software reports. Record the operator's physical assessment separately.
Verify direct agent recovery with released synthetic content and the retained recovery bundle.
The local synthetic drill supplies software evidence only.

For completed features without live switches, record the slice as `completed-features`.
Verify main-service `/version`, `/healthz`, and applicable `/readyz` checks through trusted HTTPS.
Retain DNS, capsule, countdown, and backup recovery evidence for the features in use.
Legacy `--endpoints` status checks liveness and commit only. It cannot certify readiness or a complete switch inventory.
Exclude live switches explicitly from this approval. Agents and switch protocol acceptance are unnecessary for this slice.

## Request separate owner approval

Prepare a concrete promotion pull request from the reviewed `dev` commit into `main`.
Attach the complete evidence record. Leave deployment actions for the owner-approved deployment phase.
Use this record shape:

```json
{
  "tested_commit": "FULL_LOWERCASE_SHA",
  "artifact_digest": "IMMUTABLE_DIGEST",
  "slice": "complete-switch-inventory OR completed-features",
  "source_ci_url": "SUCCESSFUL_RUN_URL",
  "source_ci_commit": "FULL_LOWERCASE_SHA",
  "promotion_pr": "SEPARATE_PR_URL",
  "promotion_ci_url": "SUCCESSFUL_RUN_URL",
  "promotion_merge_commit": "TESTED_MERGE_SHA",
  "status_evidence": "PRIVATE_RECORD_PATH",
  "recovery_evidence": "PRIVATE_RECORD_PATH",
  "host_dns_storage_clock_domains": "PRIVATE_ASSESSMENT_PATH",
  "format_compatibility_evidence": "FIXTURE_RESULTS_PATH",
  "owner_approval": null
}
```

The placeholders are incomplete evidence. An empty approval prevents promotion.
The owner records the approved source commit, artifact digest, deployment slice, promotion pull request, and evidence revision.
The owner records the approval date and identity outside public configuration.
Approval applies only to that record. Changed evidence requires another approval.
After approval and successful required CI, the authorized operator merges the separate promotion pull request.
Record the resulting `main` commit and its relationship to `tested_commit`.
If the merge creates another commit, preserve the tested artifact's original source identity.
Test a new artifact before deployment if the operator rebuilds from the merge commit.
Deploy only the approved immutable artifact during the separately authorized deployment phase.
Repeat the applicable commit, readiness, DNS, and recovery checks after deployment.
Hold promotion when any required check is unavailable, failed, or missing.

## Code rollback

Keep current agent state and monotonic authority throughout rollback.
Never restore an earlier agent volume, checkpoint, deadline, journal, or clock baseline.
Before code rollback, prove that the older executable reads the current persistent formats.
Equal format numbers alone supply no compatibility proof.
Run retained versioned fixtures and an isolated copy of current-format synthetic state with the older executable.
Verify irreversible state, receipts, restart behavior, and direct recovery under that executable.
Record the exact old and current commits, artifact digests, fixtures, and results.
If compatibility is unproved, hold rollback and obtain a reviewed forward repair or migration.
Request separate owner approval for the rollback artifact and its evidence.

## Review-only exercise

Use an isolated local Git repository with no remote to rehearse the branch relationship.
Create `main`, create `dev`, and commit a synthetic change on `dev`.
Record `git rev-parse dev` as the tested commit.
Use `git merge-base --is-ancestor main dev` to check ancestry.
Review `git diff main...dev` and retain the diff with synthetic CI and status evidence.
Prepare the record above with `owner_approval: null`. Stop before any merge.
The exercise passes when the review record identifies the exact change and leaves `main` unchanged.
A stale SHA, mixed service commits, failed readiness, missing host evidence, or absent approval must hold promotion.
This exercise proves the review procedure. It supplies no production approval or actual-host evidence.

On 2026-11-05, Git 2.53.0 passed this exercise in an isolated repository without a remote.
The retained record contains the exact synthetic `dev` SHA, the reviewed diff, and identical `main` values before and after review.
Its approval remains null. No merge, provider request, or production action occurs.
The executable status tests separately verify matching commits, mixed commits, unavailable identity, and failed readiness.
