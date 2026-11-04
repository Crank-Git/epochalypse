# tlock compatibility

## Result and scope

This isolated experiment verifies tlock v1.2.0 with the quicknet mainnet chain.
The experiment uses synthetic plaintext and a saved public beacon.
It creates no production capsule interface or dependency.
Production capsule work still depends on DNS issue #6.
This report does not approve production integration or complete epic #17.

The initial v1.2.0 target remains the latest published tlock release on 2026-11-04.
The drand documentation identifies tlock as a maintained library.
Neither source states a long-term support period for that release.
The selected configuration supports encryption, offline decryption, and local beacon verification.
Fastnet examples do not define this configuration.
The official fastnet retirement notice requires quicknet instead.

## Exact pins

| Item | Value |
|---|---|
| Go | 1.27.3 |
| tlock | v1.2.0, commit `7b54141a9733fd6fa207587a11148280e6fb020d` |
| Module checksum | `h1:YmbH2PXsq6UeUXljq+GMZcDicUlVnLIW9QbLqYoDp6g=` |
| drand library | github.com/drand/drand/v2 v2.0.2, from the tlock module |
| age | filippo.io/age v1.1.1, from the tlock module |
| Chain | quicknet mainnet |
| Chain hash | `52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971` |
| Scheme | `bls-unchained-g1-rfc9380` |
| Genesis | Unix seconds `1692803367` |
| Period | 3 seconds |
| Group hash | `f477d5c89f21a17c863a7f937c6a6d15859414d2be09cd448d4279af331c5d3e` |
| Public key | `83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a` |

The nested module records all dependency versions and checksums in `go.mod` and `go.sum`.
The root module stays unchanged.
The experiment starts no drand node and exposes no drand control service.
The [drand v2.0 security post-mortem](https://docs.drand.love/blog/2025/03/21/drand-v2-0-postmortem/) concerns a node control interface.
It describes a fix in v2.1.0.
The v2.0.2 pin here reproduces the tlock release dependency, rather than approving that version for a production node.
Production #20 must review current dependency advisories before adoption.

## Round contract

Round 1 occurs at genesis.
For a positive round `r`, the effective time is `genesis + (r - 1) * period`.
For a requested UTC second after genesis, use `1 + ceil((requested - genesis) / period)`.
Use integer division and a remainder test.
Check the effective time before multiplication.

The helper returns round 1 for a nonnegative request before genesis.
The production create interface must separately reject past release dates against its injected clock.
The helper rejects a negative timestamp, a negative genesis, and a nonpositive period.
It rejects a result whose effective time exceeds signed 64-bit UTC seconds.
The contract accepts UTC seconds only.
A future subsecond interface must round the requested instant upward before this calculation.

For quicknet, requested second `1692806365` selects round 1001 at `1692806367`.
Round 1000 occurs at `1692806364`, which precedes that request.
An exact request at a round keeps that round.
The tests assert that the preceding round occurs before the request.
Upstream `CurrentRound` and HTTP `RoundNumber` select an available round at a time.
They do not supply this ceiling boundary.

## Offline fixtures and failure evidence

`tests/compatibility/tlock/testdata/quicknet-info.json` saves the public chain response.
`quicknet-1000.json` saves the public signature for round 1000.
`synthetic.age` preserves a binary age/tlock ciphertext from the selected release.
Both responses come from an explicit chain path on `https://api.drand.sh` on 2026-11-04.
The official quicknet announcement independently supplies the public key, hash, scheme, genesis, and period.
The test checks these identity fields before it constructs the network.
The test network has no HTTP client and rejects every chain switch.

| Test | Observable evidence |
|---|---|
| `TestSelectsFirstRoundAtOrAfterTime` | Exact, between-round, genesis, invalid input, and signed overflow boundaries |
| `TestSelectsQuicknetRound` | Explicit quicknet request selects round 1001 |
| `TestVerifiesBeaconLocally` | The saved signature verifies; an altered round or signature fails |
| `TestEncryptsAndDecryptsSyntheticContent` | Supported tlock APIs recover the exact synthetic bytes offline |
| `TestDecryptsVersionedCiphertextFixture` | The selected library decrypts the saved age/tlock bytes |
| `TestRejectsInvalidRecoveryInputs` | Wrong chain, altered signature, unavailable beacon, corrupt ciphertext, and unknown age format fail |

Encryption produces fresh random ciphertext on each run.
The tests use a historical round because its public signature permits offline reproduction.
They demonstrate format compatibility, rather than a new secrecy claim for already released content.
The supported `Tlock.Encrypt` and `Tlock.Decrypt` APIs use age authenticated encryption.
The experiment adds no cryptographic primitive.

The first round tests fail against commit `fbcb194` with eight behavioral assertion failures.
That commit retains the tests with a constant round implementation.
The corrected helper passes those assertions with 100 percent statement coverage.
The final verification record must also include the committed pre-patch check and full project checks.

From the repository root, set repository-local Go caches as described in `docs/checks.md`.
Run these commands:

```sh
go test -C tests/compatibility/tlock -count=1 -race -coverprofile=coverage.out ./...
go vet -C tests/compatibility/tlock ./...
make check
```

The nested module requires no network after dependency retrieval.
To refresh the public observations, inspect the pinned client source before these bounded reads:

```sh
curl -fsSL --max-time 20 --max-filesize 65536 https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/info
curl -fsSL --max-time 20 --max-filesize 65536 https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/public/1000
```

Verify the signature locally after retrieval.
A successful HTTP response alone does not authenticate a beacon.

## Producer notes for #20

Use an explicit pinned network with `tlock.New(network).Strict()`.
The default constructor trusts a chain hash from ciphertext and can switch networks.
The strict negative test asserts `ErrWrongChainhash` and zero switch attempts.
Compare the full pinned identity before accepting external chain information.
Retain the requested time, selected round, and effective time separately.

Use a versioned capsule manifest around the established age/tlock format.
Reject unknown capsule versions before decryption.
The unknown-age-version fixture tests the library's format boundary only.
It does not implement the project's future capsule version boundary.
Include these fields in the recovery bundle:

- Capsule identifier and capsule format version.
- Mode and exact ciphertext bytes.
- Ciphertext digest and its algorithm.
- Requested UTC second, effective UTC second, and round.
- Chain hash, public key, scheme, genesis, and period.
- The age/tlock format identifier and library compatibility version.

The upstream encrypt method accepts no capsule associated-data parameter.
The producer must bind the identifier and capsule version inside authenticated encrypted content using an established format.
The consumer must check that binding before it offers plaintext.
An unauthenticated outer manifest or ciphertext digest alone does not supply that binding.
This experiment does not select a new capsule container format.

Bound the plaintext to the configured limit before encryption.
The default plaintext limit is 10 MiB.
Set a separate ciphertext limit that accounts for authenticated format overhead.
Bound each HTTP response and supply an explicit timeout.
Use explicit relay configuration and fallback policy for availability.
Keep the chain identity fixed across relays.

`Tlock.Decrypt` streams to its writer.
Large content can expose earlier authenticated chunks before a later corrupt chunk fails.
Decrypt into bounded temporary storage and complete authentication before a public download.
The small corrupt fixture exposes zero bytes; it does not prove all-or-nothing output for arbitrary content.

The upstream identity maps every `Network.Signature` error to `ErrTooEarly`.
A missing relay, an unavailable beacon, and an early round therefore share that library error.
Preserve availability classification outside that error before the consumer displays a release state.
The saved-beacon experiment does not assert live relay availability or an exact publication time.

CLI preparation keeps plaintext on the owner's machine.
Web preparation sends plaintext through the server in v1.
The web flow must state that the server operator can see plaintext before upload.
Compromise of the preparation client can expose plaintext in either flow.

Published ciphertext remains recoverable outside the project after local deletion.
Fixed capsules have no renewal or recall mechanism.
The effective round time does not guarantee beacon or ciphertext availability.
A threshold compromise can permit early decryption.
A network shutdown can prevent future decryption permanently.
The scheme does not provide post-quantum secrecy.

## Primary interfaces

All sources below were retrieved on 2026-11-04.

- [tlock latest release](https://github.com/drand/tlock/releases/tag/v1.2.0): release version and commit.
- [tlock v1.2.0 module](https://github.com/drand/tlock/blob/v1.2.0/go.mod): dependency pins.
- [tlock v1.2.0 API](https://github.com/drand/tlock/blob/v1.2.0/tlock.go): `Strict`, age stream API, and `TimeUnlock` signature verification.
- [tlock v1.2.0 identity](https://github.com/drand/tlock/blob/v1.2.0/tlock_age.go): chain switch behavior and `ErrTooEarly` conversion.
- [tlock HTTP network](https://github.com/drand/tlock/blob/v1.2.0/networks/http/http.go): network construction and round helper semantics.
- [drand v2.0.2 time API](https://github.com/drand/drand/blob/v2.0.2/common/time.go): round 1 and genesis relationship.
- [go-clients v0.2.0 HTTP API](https://github.com/drand/go-clients/blob/v0.2.0/client/http/http.go): explicit chain paths and beacon response retrieval.
- [quicknet announcement](https://docs.drand.love/blog/2023/10/16/quicknet-is-live/): independent chain identity.
- [public quicknet info](https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/info): saved observation.
- [public quicknet round 1000](https://api.drand.sh/52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971/public/1000): saved signature.
- [fastnet retirement](https://docs.drand.love/blog/fastnet-to-be-sunset/): obsolete examples and permanent shutdown risk.
- [timelock security assumptions](https://docs.drand.love/docs/timelock-encryption/): maintenance statement, threshold trust, shutdown, and quantum limits.
