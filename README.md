# epochalypse

epochalypse provides DNSSEC window experiments, time capsules, and DNS countdowns.
Owner access protects administration. Switch runtime work remains deferred.

## Start locally

Install Docker with Compose support, GNU Make, Python 3, and curl. Start the Docker daemon.
Run these commands from the repository root:

```sh
make lab
mkdir -p var/lab-client
docker compose -f deploy/lab/compose.yaml cp web:/state/lab/tls.crt var/lab-client/tls.crt
curl --fail --cacert var/lab-client/tls.crt https://localhost:8080/healthz
```

Open **https://localhost:8080/owner**.
The lab creates a synthetic owner automatically. The [lab guide](docs/lab.md#setup-and-files) supplies its known password.
Browsers identify the certificate as self-signed. Trust it only for this synthetic lab.

Follow the [local walkthrough](docs/local-milestone.md) to enable countdowns and exercise all three features.
The default lab enables windows and capsules. Countdowns need the separate synthetic key and zone in that walkthrough.

Stop the lab and preserve its state:

```sh
docker compose -f deploy/lab/compose.yaml down
```

## Guides and checks

- [Signed TXT windows](docs/windows.md)
- [Owner access](docs/owner.md)
- [Time capsules](docs/capsules.md)
- [DNS countdowns](docs/countdowns.md)
- [Local lab and reset](docs/lab.md)
- [Deployment obligations](docs/operations.md)
- [Required checks](docs/checks.md)
- [Approved specification](docs/specs/spec.md)

For native development and checks, install Go 1.27.3 and golangci-lint 2.15.0.
Run `make check` for every required check. Docker supplies the browser and resolver environments.
Run `make run` for the main service alone with `config/local.example.json`; that example does not configure signed DNS or an owner.

Use synthetic content locally. Keep real credentials, private keys, and production infrastructure values outside tracked files.
