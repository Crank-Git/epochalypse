# 0009: Share retained installation state

Date: 2026-11-05 UTC
Status: accepted technical choice for #18

## Decision

Keep the existing `v1/windows.db` path and version 1 window and observation documents.
`installation.Open` owns one bbolt handle for retained content. Attached domain handles share its transactions and configured limits.
The main service owns this handle even when DNS remains disabled.
Offline commands use the same database and fixed layout. A second process must acquire the same database lock.

Closing a domain handle stops new operations for that domain. It preserves access through other domain handles.
Shutdown stops domain users before it closes the installation handle.
The separate owner store retains its existing credential format and lifetime.

## Atomic budget

Admission scans the whole fixed layout inside the mutation's transaction.
It counts retained domains even when their wrappers remain unopened.
It requires no private DNS signing key.

| Bucket | Object charge | Byte charge |
|---|---|---|
| `windows-v1` | One for each record | Encoded JSON value |
| `observations-v1` | One for each run | Greater of encoded JSON value and valid pending reservation |
| `capsules-v1` | One for each record | Encoded JSON value, including its content object |
| `capsule-keys-v1` | Zero | Each separate 32-byte content key |
| `configuration-v1` | Zero | Zero for the fixed configuration record |

Database pages and storage keys do not consume this logical byte allowance.
The existing defaults remain 100 objects and 1 GiB.
Configure the shared limits before concurrent operations start.

A pending observation reserves exactly 16384 bytes. It has version 1 and no completion time.
A terminal observation has a completion time and no reservation.
Admission rejects unknown reservation versions, states, and charges.
It also rejects unknown content buckets, nested content buckets, and unsupported document versions.
Subtractive arithmetic rejects additions before a count or byte total can overflow.

Window operations retain their domain-specific signature and observation-context checks.
The shared scan validates the reservation envelope independently of those checks.
Future domains must extend the fixed layout and its tests before they admit retained content.

## Alternatives

Independent domain quotas could each admit the full installation allowance.
Per-domain registration could omit records when an offline command or disabled feature leaves another wrapper unopened.
Persisted counters would require another migration and corruption policy for the existing documents.
The whole-layout scan avoids these sources of inconsistent admission.

## Migration and downgrade

The shared owner changes no legacy path or document bytes.
Window attachment retains its additive bucket initialization and original DNSKEY binding.
The shared scan recognizes the capsule buckets before the capsule wrapper opens.

This decision guarantees preservation of the tested version 1 fixtures. It does not promise compatibility with arbitrary future schemas.
Older executables lack the new whole-layout scan. They can omit capsule charges from admission.
Do not use an older executable to mutate a database after capsule content enters it.
A future schema change requires explicit compatibility evidence or a reviewed migration.

## Verification

`TestBudgetCountsEveryDomainWithoutWrappers` verifies whole-layout accounting without a DNS signer.
`TestBudgetRejectsCorruptReservationEnvelopes` verifies valid-JSON corrupt reservations.
`TestBudgetAdmitsOnlyOneConcurrentCreation` verifies atomic cross-domain admission.
`TestInstallationPreservesLegacyBytesAfterRestart` verifies exact fixture bytes after database reopen.
`TestDomainHandleClosePreservesOtherDomains` verifies independent domain lifetime.
`TestMainStateOwnsContentLifetimeWithoutDNS` verifies the server's content lock without DNS configuration.
The committed pre-patch proof reports three behavioral failures against `11855f3`, then restores a clean HEAD.

Verified against [bbolt v1.5.0 shipped interfaces](https://github.com/etcd-io/bbolt/blob/v1.5.0/db.go): `Open`, `Update`, `View`, and `Close`.
