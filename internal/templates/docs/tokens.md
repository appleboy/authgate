# Tokens & Revocation

Everything integrators need to know about AuthGate tokens *after* the flow completes: lifecycles, refreshing, revoking, and checking validity in real time.

## Token Lifecycle

After a successful flow you hold one or more of:

| Token           | Format              | Lifetime (per client profile)                        | Used for                                          |
| --------------- | ------------------- | ---------------------------------------------------- | ------------------------------------------------- |
| Access token    | JWT                 | `short` 15m · `standard` 10h · `long` 24h (approx.)  | `Authorization: Bearer` on API calls              |
| Refresh token   | JWT (treat as opaque) | `short` 1d · `standard` 30d · `long` 90d (approx.) | Exchanging for a new access token (`/oauth/token`) |
| ID token        | JWT                 | Same as access token                                 | Client-side identity — [see OIDC](./oidc)         |

> Refresh tokens happen to be JWTs internally, but you should **treat them as opaque** — don't parse their claims in client code; you gain nothing and risk coupling to an internal detail.

The exact numbers depend on the per-client **token profile** set by the administrator. **Always trust `expires_in`** from the token response — never hardcode.

## Refreshing Tokens

At any point before the refresh token itself expires:

```bash
curl -X POST https://your-authgate/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=REFRESH_TOKEN" \
  -d "client_id=YOUR_CLIENT_ID"
# Confidential clients: add -u "$CLIENT_ID:$CLIENT_SECRET" instead of client_id in body
```

**Response** (same shape as the original token exchange).

**When to refresh**: proactively, e.g. 30–60 seconds before expiry, not on 401. This avoids mid-request failures and the noise of retry logic.

> If your deployment uses rotation mode (next section), you must also **serialize concurrent refreshes per session** — two tabs refreshing at once will blow up the session.

### Rotation Mode: the Reuse Detection Gotcha

Some AuthGate deployments run in **rotation mode** (`ENABLE_TOKEN_ROTATION=true`). In that mode:

- Each refresh issues a **new** refresh token and **invalidates** the old one.
- If the old refresh token is ever used again (e.g. a race between two tabs, a retry after a network hiccup, a stolen copy), AuthGate detects the reuse and **revokes the entire token family**.
- Subsequent calls return `{"error": "invalid_grant"}`.

**Practical implications for integrators:**

- **Serialize refresh calls** per user/session (mutex, single-flight). Two tabs refreshing simultaneously will both try to cash in the same old refresh token, one will win, and the other — with the *just-invalidated* old token — will trip reuse detection and kill the session.
- **Persist the new refresh token immediately**. Don't issue another request with the old one while you're updating storage.
- **Treat `invalid_grant` on refresh as terminal** — show a login screen; don't retry.

You can't tell from the token response alone whether rotation is on. If your integration must work on both modes, always persist the returned `refresh_token` (even if it looks identical — under rotation it will differ).

## Sign Out — `/oauth/revoke` (RFC 7009)

On logout, revoke the **refresh token** (and optionally the access token) so a stolen copy is inert:

```bash
curl -X POST https://your-authgate/oauth/revoke \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=REFRESH_TOKEN" \
  -d "token_type_hint=refresh_token" \
  -d "client_id=YOUR_CLIENT_ID"
# Confidential clients: include client_secret or use HTTP Basic
```

| Parameter         | Required | Values                                   |
| ----------------- | -------- | ---------------------------------------- |
| `token`           | yes      | The token to revoke                      |
| `token_type_hint` | no       | `access_token` or `refresh_token`        |
| `client_id`       | yes      | Plus `client_secret` for confidential    |

Per RFC 7009, the endpoint returns **`200 OK`** whether or not the token existed. Don't rely on the response to tell you anything — just assume the token is gone.

> Revoking a refresh token also invalidates the whole token family (in rotation mode). Revoking an access token does **not** automatically revoke the matching refresh token — revoke both, or revoke the refresh token on logout and let the short-lived access token expire on its own.

## Checking Validity in Real Time

For local JWT verification at resource servers, see [JWT Verification](./jwt-verification). That's fast and scales horizontally but **cannot** detect revoked/disabled tokens — a revoked JWT stays cryptographically valid until `exp`.

When you need real-time revocation awareness, call one of these endpoints:

### `/oauth/introspect` (RFC 7662) — Preferred

Requires client authentication (the calling service must itself be a registered AuthGate client):

```bash
curl -X POST https://your-authgate/oauth/introspect \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=TOKEN_TO_CHECK" \
  -d "token_type_hint=access_token"
```

**Response:**

```json
{
  "active": true,
  "scope": "openid profile email",
  "client_id": "client-uuid",
  "username": "alice",
  "token_type": "Bearer",
  "exp": 1700000000,
  "iat": 1699996400,
  "sub": "user-uuid",
  "iss": "https://your-authgate",
  "jti": "unique-token-id"
}
```

If the token is invalid, expired, revoked, or disabled, the response is:

```json
{ "active": false }
```

Use this for **policy enforcement** where freshness matters — admin dashboards, high-value operations, anything where you can't tolerate a ≤ 1-hour window of stale validity.

### `/oauth/tokeninfo` — Lightweight Alternative

Takes the token as a Bearer header and returns a subset of fields. No client credentials needed (the token itself is the auth):

```bash
curl -H "Authorization: Bearer TOKEN_TO_CHECK" https://your-authgate/oauth/tokeninfo
```

```json
{
  "active": true,
  "user_id": "user-uuid",
  "client_id": "client-uuid",
  "scope": "openid profile email",
  "exp": 1700000000,
  "iss": "https://your-authgate",
  "subject_type": "user"
}
```

`subject_type` is `"client"` for tokens issued by Client Credentials. Invalid tokens return `401` with an OAuth `invalid_token` error.

### Which One?

| Need                                                                   | Use                                        |
| ---------------------------------------------------------------------- | ------------------------------------------ |
| Resource server validates tokens at scale, can tolerate short staleness | **Local JWKS verify** (no call to AuthGate) |
| Need real-time revocation state, calling service can authenticate      | **`/oauth/introspect`**                    |
| Lightweight check from within a user session, no client creds handy    | **`/oauth/tokeninfo`**                     |
| Calling service is itself the token's owner                            | **`/oauth/tokeninfo`**                     |

## Rate Limits

AuthGate applies per-IP rate limits to token-path endpoints. Defaults (an operator may tune these):

| Endpoint                | Default limit        |
| ----------------------- | -------------------- |
| `POST /oauth/token`     | 20 req/min           |
| `POST /oauth/device/code` | 10 req/min         |
| `POST /device/verify`   | 10 req/min           |
| `POST /oauth/introspect` | 20 req/min          |
| `POST /login`           | 5 req/min            |

Exceeded limits return `429 Too Many Requests`. Honor any `Retry-After` header; otherwise back off exponentially. Batch your work — don't poll `/oauth/tokeninfo` per request if you can verify locally via JWKS.

## Related

- [Getting Started](./getting-started)
- [Authorization Code Flow](./auth-code-flow)
- [Device Authorization Flow](./device-flow)
- [Client Credentials Flow](./client-credentials)
- [JWT Verification](./jwt-verification)
- [OpenID Connect](./oidc)
- [Errors](./errors)
