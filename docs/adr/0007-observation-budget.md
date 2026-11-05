# ADR 0007: Reserve observation bytes in the window transaction

Date: 2026-11-04
Status: accepted technical choice for issue #9

## Decision

Windows and observation runs use one installation budget in `v1/windows.db`.
Each retained window and run counts as one object.
A pending run reserves 16384 bytes before the main process starts resolver work.
The same bbolt transaction checks the object and byte limits before it stores that reservation.
Window creation includes pending reservations in its quota calculation.

Completion replaces the reservation with the actual encoded observation document size.
The final document cannot exceed 16384 bytes.
The transaction commits all resolver results before it releases unused bytes.
A completed or partial run remains one retained object.
Physical bbolt page overhead and the fixed DNSKEY configuration record do not enter this JSON-byte budget.

Independent observation allowances could let observations and windows each consume the full installation quota.
A separate quota check could race with window creation before either operation commits.
One transaction and a pending reservation prevent both failures.

## Durable context and migration

The additive `observations-v1` bucket uses `<window-id>/<run-id>` keys.
Each document carries the version 1 run and its current byte reservation.
Every load binds its embedded identifiers to the storage key.
The same transaction verifies the original signed window and the run's window name.
It also binds every resolver result to that window name.
Reads, completion, recovery, and quota traversal reject mismatched context.

Opening an older version 1 database creates the observation bucket.
The migration preserves existing signed windows, their DNSKEY binding, and their exact signatures.
The versioned observation fixture preserves unknown clock offsets and unavailable response fields.

## Recovery and lifetime

Startup marks interrupted pending jobs partial before it admits new work.
Each interrupted result has `run_interrupted` with absent response code and flags.
Recovery replaces its reservation with the actual terminal document size.
It preserves complete and partial runs.
Terminal runs reject a second completion.

The main process owns the store and the bounded job service.
Shutdown cancels resolver connections and drains accepted jobs before it closes the store.
A failed durable completion makes the job service unavailable until restart and recovery.

## Future domains

Capsules, switches, and countdowns must join the installation budget before they admit retained state.
They must define a shared transaction or reservation mechanism.
They cannot receive separate full installation allowances.

## Evidence

`TestObservationAndWindowCreationShareOneAtomicQuota` verifies concurrent admission.
`TestObservationCompletionReleasesOnlyItsByteReservation` verifies byte accounting.
`TestObservationStatePreservesVersionOneWindowsAfterRestart` verifies migration and recovery.
`TestObservationLoadsRejectMismatchedDurableContext` verifies valid-JSON corruption failures.
The PM accepts the storage design after its focused re-review.
