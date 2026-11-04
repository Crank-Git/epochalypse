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

## CSRF proof

Every password sign-in request requires CSRF proof, including requests before authentication.
Cookie-authorized mutations also require proof. The local lab applies the same checks.
The owner forms supply a hidden `csrf_token` field automatically.

For an API client, request `GET /api/v1/csrf` first.
Save its `__Host-epochalypse-csrf` cookie and the `csrf_token` value from the JSON response.
The cookie uses Secure, HttpOnly, SameSite=Strict, and the host-only `/` path.
The token contains 256 random bits. The response uses `Cache-Control: no-store`.
Send the cookie with `X-CSRF-Token: <csrf_token>` on each mutation.
After sign-in, also send the session cookie.
The proof remains valid while the client retains the CSRF cookie.
The bootstrap endpoint and owner page reuse that cookie across tabs and failed sign-ins.

The server rejects missing, incorrect, or duplicate proof with 403 and `invalid_csrf`.
Query parameters cannot supply proof. Form bodies have an 8192-byte limit.
If an `Origin` header is present, it must match the request scheme and host exactly.
The server rejects cross-site browser requests, including requests with otherwise valid proof.
Clients without an `Origin` header still require proof.
Forwarded headers do not change the accepted origin.
Only a bearer token that passes the scope guard can omit CSRF proof.
Bearer tokens cannot authorize session deletion or token administration.

## Failed sign-in limits

The server permits five failed password checks per owner identity and peer address within 15 minutes.
The fifth failure starts a 15-minute cooldown for each limit.
During the cooldown, sign-in returns 429 with a generic credential error and a `Retry-After` header.
The server rejects correct passwords during the cooldown.
Successful sign-ins preserve failures until their window expires.
Form and API sign-ins share the limits. Concurrent requests cannot exceed the failure allowance.
The server uses the socket peer address. Forwarded headers cannot select another address.
The server retains at most 1024 source entries and removes expired entries before it admits another source.
At capacity, the server rejects a new source until an entry expires.
The limits use process memory. A server restart clears them.

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
Cookie: __Host-epochalypse-session=<session>; __Host-epochalypse-csrf=<csrf_token>
X-CSRF-Token: <csrf_token>

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
The guard admits a matching API token or a valid owner session with CSRF proof for mutations.
Use `Store.RequireOwner(next)` for credential administration.
The store methods require a trusted clock. They do not replace route authorization.

## Owner dashboard

After sign-in, `/owner` shows capsules, switches, countdowns, and agent health.
The page provides section links, visible keyboard focus, and a sign-out form.
The layout supports narrow screens and the browser's light or dark color scheme.
The sign-out form retains the session and CSRF safeguards.
If the owner session is absent or expired, the page shows sign-in.
If storage fails, the page shows an error and a retry link.

`GET /api/v1/dashboard` accepts an owner session or a bearer token with the exact `read` scope.
An explicit bearer credential prevents fallback to an owner cookie.
The response requires HTTPS outside the local lab and uses `Cache-Control: no-store`.
The optional `limit` parameter accepts one integer from 1 through 100. The default is 100.
The limit applies separately to each summary list. Invalid limits return 400 after authorization.
Unauthorized requests return 401. Tokens without `read` return 403. Storage failures return 503.
Errors use the shared JSON error format and response request identifier.

Each response has `capsules`, `switches`, `countdowns`, and `agents` fields.
Each field contains a textual `state`, a `message`, and an `items` array.
Future metadata items use `{id,name,state}`. Summary items contain no content or credentials.
`Store.Dashboard(limit)` supplies this contract after the caller authorizes access.

The current feature stores contain no capsule, switch, or countdown implementation.
Their lists therefore return empty arrays. Capsule and countdown states are `not_implemented`.
The switch state is `blocked`, consistent with the owner's production decision in ADR 0005.
Agent health is `unavailable` because the dashboard has no verified agent observations.
These states do not claim that an agent is healthy or that content has released.
The page explains feature availability instead of offering unsupported creation controls.

## Audit events

The owner database retains the newest 256 audit events in the versioned `audit-v1` bucket.
Each event contains only `version`, `at`, and `action`. The timestamp uses server UTC.
Fixed action names describe owner initialization, recovery, rejected passwords, session creation, session revocation, token creation, and token revocation.
Events contain no password, session identifier, token value, token name, plaintext, key, or share.
The event format accepts no caller-supplied text.
The log records password rejection after a password check. Requests rejected before that check create no event.
Repeated revocation of an absent session or token creates no event.

Successful mutations persist their event in the same transaction as the state change.
If the audit write fails, the transaction preserves the previous state.
The store reports an error when it cannot record a rejected password.
The retention limit removes the oldest event after each additional write.
This local history does not provide an external audit archive or protection against host access.

`Store.AuditEvents(limit)` returns the newest events first. The limit must be from 1 through 100.
The caller must authorize owner access before it calls this method.
Invalid limits, invalid event state, or unavailable storage return an error without partial results.
The store API exposes no audit write method for caller-supplied fields.
