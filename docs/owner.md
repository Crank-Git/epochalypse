# Local owner commands

Create an owner-only password file outside tracked paths. Give the file mode 0600.
The password contains 1 to 1024 bytes. The command removes one final newline.

Run `epochalypse owner init --password-file /path/to/password --state-dir var/state`.
The command creates one owner. A second initialization refuses to replace that owner.

For local recovery, stop the server to release the database lock.
Run `epochalypse owner recover --password-file /path/to/new-password --state-dir var/state`.
Recovery preserves the owner identity. Recovery removes all sessions in the same transaction.
Restart the server after recovery.

The commands read credentials from a protected file. Command arguments contain no password values.
The database uses `var/state/v1/owner.db` with owner-only access.
Keep credentials and state outside version control.
