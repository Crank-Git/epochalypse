# Pull request checks

The `CI` workflow runs the `Required checks` job for pull requests into `dev` and `main`.
The job installs Go 1.27.3 and golangci-lint 2.15.0.
It verifies the linter archive with its SHA-256 checksum before extraction.
Both actions use full commit identifiers.
The job receives read access to repository content.
The checkout step disables persistent credentials.

The workflow uses JSON syntax, which is a subset of YAML.
This format lets the Python standard library test the workflow without a new parser dependency.
Run `python3 .github/tests/test_ci.py` to check the workflow contract.
Run `make check` to execute the current checks from [Local checks](checks.md).
The workflow consumes that command as the check scope expands.
A failed installation or check fails the job.

Internal draft pull requests target an epic integration branch.
The branch filter excludes these pull requests.
Each internal head commit also ends with `[skip ci]`.
GitHub applies that instruction to `pull_request` events.
The epic integration pull request targets `dev` and uses a head commit without a skip instruction.
The workflow has no push trigger, so each update creates one run.
Promotion from `dev` to `main` requires a separate owner-approved pull request.

Both branches exist as of 2026-11-04.
The repository reports `protected: false` for both branches on that date.
After the first integration check succeeds, the PM configures required status checks for both branches.
The intended required check name is `Required checks`.
The PM preserves the approved autonomous merge policy without a human review requirement.
A skip instruction on an integration head leaves required checks pending.
Push a clean head commit to request the required run.

Verified interfaces, retrieved 2026-11-04:

- [GitHub workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax).
- [GitHub skip instructions](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/skip-workflow-runs).
- [checkout v7.0.1 inputs](https://github.com/actions/checkout/blob/v7.0.1/action.yml), commit `3d3c42e5aac5ba805825da76410c181273ba90b1`.
- [setup-go v7.0.0 inputs](https://github.com/actions/setup-go/blob/v7.0.0/action.yml), commit `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`.
- [golangci-lint 2.15.0 assets](https://api.github.com/repos/golangci/golangci-lint/releases/tags/v2.15.0), including the archive digest.
- [golangci-lint binary installation](https://golangci-lint.run/docs/welcome/install/local/).
