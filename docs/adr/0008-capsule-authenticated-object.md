# 0008: Authenticate capsule content with compact JWE

Status: accepted for the capsule foundation in #18.

## Context

Encrypted capsules require an established authenticated format. The format must bind the capsule identifier and version.
Later capsule slices reuse the content object. Server-held storage and tlock use separate mechanisms to retain or wrap its content key.

## Decision

Use `github.com/go-jose/go-jose/v4` at `v4.1.7` for RFC 7516 compact JWE.
Use `alg=dir` and `enc=A256GCM`. Generate a fresh random 32-byte key for each object.
Keep the key separate from the content object and public metadata.
The adapter returns plaintext only after the library authenticates the complete object.
It returns an error without plaintext after any failure.

The protected header has exactly these parameters:

| Parameter | Value |
|---|---|
| `alg` | `dir` |
| `capsule_id` | 32 lowercase hexadecimal characters |
| `capsule_version` | The JSON integer `1` |
| `enc` | `A256GCM` |
| `typ` | `epochalypse-capsule+jwe` |

The adapter requires canonical JSON with lexicographically sorted parameter names, no whitespace, and no escape sequences.
It compares the encoded protected header with the exact expected header before library parsing.
This comparison excludes unknown parameters, duplicates, compression, critical extensions, embedded keys, and alternate parameter types.
JSON serialization, external associated data, and multiple recipients have no representation in this profile.
The protected header supplies the authenticated associated data defined by RFC 7516.

The compact object has five segments. The encrypted-key segment is empty.
The nonce encodes 12 bytes. The authentication tag encodes 16 bytes.
All content segments use canonical unpadded base64url.

## Size policy

The default plaintext limit is 10 MiB, or 10,485,760 bytes.
The adapter validates a positive limit before arithmetic or encryption.
It rejects limits that can overflow the encoded allowance on the host architecture.

The independent protected-header allowance is 1,024 encoded bytes.
The complete encoded allowance is `ceil(4 * plaintext_limit / 3) + 1066` bytes.
The implementation computes this expression through quotient and remainder arithmetic to avoid multiplication overflow.
The fixed allowance includes the protected header, nonce, tag, and four separators.
The default complete allowance is 13,982,080 bytes.
The ciphertext segment has its own exact base64url length bound for the configured plaintext limit.
The adapter checks the complete allowance before segment processing.
It checks the header and all segment bounds before the library allocates decoded ciphertext.

Upload adapters must bound plaintext reads before allocation. They must apply the configured limit before they call `SealObject`.
Recovery adapters must bound encoded reads before they call `OpenObject`.
This content adapter does not define a recovery bundle or permit publication of an incomplete tlock capsule.
Later slices define finite metadata and key-wrapper allowances outside this content-object allowance.

## Dependency evidence

Verified against the following primary sources on 2026-11-05 UTC:

- [RFC 7516](https://www.rfc-editor.org/rfc/rfc7516.html), sections 3.1, 5, and 7.1.
- [Tagged encryption source](https://github.com/go-jose/go-jose/blob/v4.1.7/crypter.go): `NewEncrypter`, `EncrypterOptions.WithHeader`, and `Decrypt`.
- [Tagged serialization source](https://github.com/go-jose/go-jose/blob/v4.1.7/jwe.go): `ParseEncryptedCompact` and `CompactSerialize`.
- [Release v4.1.7](https://github.com/go-jose/go-jose/releases/tag/v4.1.7).
- [Vendor advisory feed](https://api.github.com/repos/go-jose/go-jose/security-advisories).

`go mod download -json` resolves the tag to commit `59611a121c781925dee4b9706bd75408b470a596`.
The module checksum is `h1:Nac/98uT5LPkfbiFuYa8ktf62a2VxwQ4xbq1GK8pV5E=`.
The module-file checksum is `h1:x4oUasVrzR7071A4TnHLGSPpNOm2a21K9Kf04k1rs08=`.
The dependency declares Go 1.24.0. The project uses Go 1.27.3.

The vendor feed identifies these October 30 advisories as fixed in v4.1.7:

- [GHSA-p9j4-2j95-j4rv](https://github.com/go-jose/go-jose/security/advisories/GHSA-p9j4-2j95-j4rv): protected-header override.
- [GHSA-27pc-rm86-68pq](https://github.com/go-jose/go-jose/security/advisories/GHSA-27pc-rm86-68pq): JWT claim panic.
- [GHSA-r39w-98wm-9pp6](https://github.com/go-jose/go-jose/security/advisories/GHSA-r39w-98wm-9pp6): RSA thumbprint panic.
- [GHSA-c67m-wr4h-2ccg](https://github.com/go-jose/go-jose/security/advisories/GHSA-c67m-wr4h-2ccg): JSON serialization denial of service.
- [GHSA-v6h2-qj3f-2h6v](https://github.com/go-jose/go-jose/security/advisories/GHSA-v6h2-qj3f-2h6v): certificate verification denial of service.

The feed lists no advisory whose affected range includes v4.1.7 at this review.
The application profile excludes these alternate parser surfaces before library parsing.
Future dependency updates require another advisory review.

## Verification

`internal/capsule/object_test.go` verifies size boundaries, fresh keys, identifier authentication, corrupt content, profile rejection, and absence of partial plaintext.
`internal/capsule/testdata/object-v1.json` contains a synthetic public fixture with a disclosed test key.
The fixture preserves the exact version 1 profile across later changes.
The fuzz target exercises malformed input through the bounded public content adapter.

## Consequences

Consumers must preserve the exact protected-header profile. They cannot add parameters without a format change.
The content object authenticates content and capsule identity. Public manifest fields require their own validation.
The shared store must protect retained keys independently of public content.
The upload, store, publication, and release interfaces use this profile through the shared capsule producer.
