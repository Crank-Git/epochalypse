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

Open `/owner` to sign in with the local password. Select **Sign out** to revoke the session.
The page uses a labeled password field and reports generic credential errors.
The browser cookie uses Secure, HttpOnly, SameSite=Strict, and the `__Host-` prefix.
The server stores only the session identifier hash.
Idle sessions expire after 30 minutes. Every session expires after 12 hours.

For HTTPS, start the server with `--tls-cert /path/to/cert.pem --tls-key /path/to/key.pem`.
Keep the private key outside tracked paths. Owner routes reject HTTP requests by default.
For local tests, use `--local-lab` with a loopback listen address.
The local lab keeps the cookie Secure. Chromium permits secure cookies on loopback origins.
Forwarded headers do not establish TLS for owner access.

`POST /api/v1/session` accepts JSON with one `password` field. Success returns 204 with the session cookie.
`DELETE /api/v1/session` revokes the supplied session cookie and returns 204.
Malformed credentials return 400. Invalid credentials return 401. An unavailable owner store returns 503.
Errors use the shared JSON error format and exclude credential values.

The complete owner epic adds scoped tokens, CSRF checks, failed sign-in limits, and the dashboard before integration into `dev`.
