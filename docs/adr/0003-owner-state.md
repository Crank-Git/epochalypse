# Local owner state and passwords

Date: 2026-11-04
Status: accepted technical choice for issue #13

Use bbolt v1.5.0 for local owner state. Use x/crypto v0.58.0 for Argon2id.
The installation has one owner. A local file avoids a separate database service.
Synced transactions persist each credential change before the command reports success.
Recovery changes the password and removes sessions in one transaction.
Versioned buckets and records identify the persistent format.

SQL supports relational queries and multiple writers across processes.
This owner store needs neither capability. bbolt supplies transactions without a SQL driver or server.
The database file requires mode 0600. Its directory requires mode 0700.
The file lock bounds concurrent administrative access with a one-second timeout.

Argon2id uses 64 MiB, three iterations, four lanes, and a 32-byte digest.
Each password has a random 16-byte salt. The record includes all algorithm parameters.
Validation accepts only this profile before it allocates hash memory.
Two concurrent hashes bound active Argon2 memory to 128 MiB.
The digest comparison uses constant time.

Verified interfaces, retrieved 2026-11-04:

- [bbolt v1.5.0](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0): Open, Options.Timeout, DB.Update, DB.View, and bucket operations.
- [Argon2 v0.58.0](https://pkg.go.dev/golang.org/x/crypto@v0.58.0/argon2): IDKey and Version.

Run `go test ./internal/owner -run '^$' -bench BenchmarkPasswordProfile -benchmem` to measure the profile.
