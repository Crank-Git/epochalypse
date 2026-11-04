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

## Scoped tokens

For local token creation, stop the server to release the database lock.
Run the following command with host access:

```sh
epochalypse token create --state-dir var/state --name automation \
  --scopes read,switch:checkin --expires-at 2027-01-01T00:00:00Z
```

The command writes JSON with `id` and `token` to standard output once.
Save the token in protected storage. The database stores only its SHA-256 digest and access metadata.
Restart the server after local token creation. A concurrent local command fails after the database lock timeout.

For token creation while the server runs, use the authenticated owner session:

```http
POST /api/v1/tokens
Content-Type: application/json
Cookie: __Host-epochalypse-session=<session>

{"name":"automation","scopes":["read"],"expires_at":"2027-01-01T00:00:00Z"}
```

Success returns 201 with `{id,token}`. The response uses `Cache-Control: no-store`.
The token contains 256 random bits. Its separate identifier contains 128 random bits.
The API does not return the token again.
The name contains 1 to 128 UTF-8 bytes and includes a non-space character.
Supply at least one unique scope and a future RFC 3339 expiry.
The supported scopes are:

- `read`
- `capsule:write`
- `switch:write`
- `switch:checkin`
- `countdown:write`

For scoped API requests, send `Authorization: Bearer <token>` over HTTPS.
Each request checks the exact scope, expiry, and current durable state.
The `switch:checkin` scope does not permit `switch:write` operations.
Token use does not extend expiry. The server rejects a token at its expiry boundary.
An invalid token returns 401. A missing scope returns 403 with a `WWW-Authenticate` bearer challenge.
Token values in query parameters do not authorize requests.

`DELETE /api/v1/tokens/{id}` requires the owner session and returns 204.
Repeated revocation returns 204. Revocation takes effect on subsequent requests without a restart.
Revocation survives a restart. Password recovery revokes sessions and preserves scoped tokens.
API tokens cannot create or revoke tokens. An explicit bearer header prevents fallback to an owner session.
Malformed requests return 400. Bodies over 8192 bytes return 413.
Token routes use the same TLS and error safeguards as session routes.

For API route authors, use `Store.RequireScope(scope, next)` inside the owner handler's TLS and response safeguards.
The guard admits a matching API token or a valid owner session.
Use `Store.RequireOwner(next)` for credential administration.
The store methods require a trusted clock. They do not replace route authorization.

The complete owner epic adds CSRF checks, failed sign-in limits, and the dashboard before integration into `dev`.
