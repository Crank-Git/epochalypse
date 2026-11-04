# External configuration references

## Operator setup

The public repository belongs to the owner's personal GitHub account.
The public remote is https://github.com/Crank-Git/epochalypse.
The owner supplies the domain and VPS hosts during deployment setup.
The owner can use owner-operated agents or independent trusted operators.
Keep real addresses, credentials, and private keys outside this document.

## Configuration names

The complete descriptions appear in docs/specs/spec.md under Environments & config.

- EPOCHALYPSE_ENV
- EPOCHALYPSE_CONFIG
- EPOCHALYPSE_STATE_DIR
- EPOCHALYPSE_HTTP_ADDR
- EPOCHALYPSE_DNS_ADDR
- EPOCHALYPSE_PUBLIC_URL
- EPOCHALYPSE_AGENT_ID
- EPOCHALYPSE_KEY_FILE
- EPOCHALYPSE_MAX_UPLOAD_BYTES
- EPOCHALYPSE_MAX_OBJECTS
- EPOCHALYPSE_QUOTA_BYTES
- EPOCHALYPSE_LOG_LEVEL

## Documentation and support

Use docs/rules/external-apis.md for the interface evidence rules.
Use docs/specs/pages/research-evidence.md for DNSSEC, drand, and dependency sources.
Use docs/specs/features/07-operations.md for deployment and recovery requirements.
Use the selected dependency's official issue tracker for dependency support.
No real test account, secret, or credential belongs in the public repository.
