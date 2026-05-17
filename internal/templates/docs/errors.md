# Errors

Reference for OAuth error codes AuthGate returns, and how an integrator should handle each. All error responses follow RFC 6749 §5.2:

```json
{
  "error": "invalid_grant",
  "error_description": "Human-readable description of what went wrong"
}
```

`error_description` is for logging and debugging — **do not** expose it to end users.

## Error Codes by Scenario

### Authorization Endpoint Redirect Errors

When `/oauth/authorize` fails after the user has been redirected to your `redirect_uri`, the error is passed via query string:

```
https://yourapp.example/callback?error=access_denied&error_description=...&state=RANDOM_STATE
```

| `error`                     | Cause                                                                                                                                | Your action                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- |
| `access_denied`             | User declined consent, or admin revoked their access                                                                                 | Show "sign-in cancelled"; let user retry                                                     |
| `invalid_request`           | Missing / malformed parameter, **or** PKCE is required for this client but `code_challenge` was absent                               | Fix the request — this is a client bug                                                       |
| `invalid_scope`             | Requested scope not permitted for this client                                                                                        | Drop the offending scope; check with admin                                                   |
| `unauthorized_client`       | Client not permitted for Authorization Code Flow                                                                                     | Ask admin to enable Auth Code Flow for this client                                           |
| `unsupported_response_type` | `response_type` was not `code`                                                                                                       | Use `response_type=code`                                                                     |
| `invalid_target`            | One or more `resource=` parameters failed RFC 8707 validation (non-http(s) scheme, fragment, empty host, > 10 entries, > 1024 chars) | Fix the request — see [Resource Indicator Errors](#resource-indicator-errors-rfc-8707) below |
| `server_error`              | Transient AuthGate failure                                                                                                           | Retry with backoff                                                                           |

### Token Endpoint Errors (`/oauth/token`)

Returned as HTTP 400 JSON (except `invalid_client`, which is 401):

| `error`                  | HTTP | Common cause                                                                                                                                                                                      | Your action                                                                                                                    |
| ------------------------ | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `invalid_request`        | 400  | Missing required form field                                                                                                                                                                       | Fix the request                                                                                                                |
| `invalid_client`         | 401  | Wrong `client_id` / `client_secret`, or missing client auth                                                                                                                                       | Verify credentials; check HTTP Basic vs. body                                                                                  |
| `invalid_grant`          | 400  | Code / refresh token / device code is invalid, expired, used, or was revoked (incl. rotation reuse detection); or PKCE `code_verifier` did not match the original `code_challenge`                | Stop retrying. Restart the flow / re-authenticate the user                                                                     |
| `invalid_scope`          | 400  | Scope exceeds what the client or original grant allows                                                                                                                                            | Drop or narrow scopes                                                                                                          |
| `unauthorized_client`    | 400  | Grant type not enabled for this client                                                                                                                                                            | Ask admin to enable the grant                                                                                                  |
| `unsupported_grant_type` | 400  | `grant_type` not recognized                                                                                                                                                                       | Use one of: `authorization_code`, `refresh_token`, `urn:ietf:params:oauth:grant-type:device_code`, `client_credentials`        |
| `invalid_target`         | 400  | `resource=` is malformed, **or** (on `refresh_token` / `authorization_code` / `device_code` grants) requests a resource that is not a subset of the original grant (RFC 8707 §2.2 narrowing rule) | Fix the request — see [Resource Indicator Errors](#resource-indicator-errors-rfc-8707) below. Do not retry with the same value |
| `server_error`           | 500  | AuthGate internal error                                                                                                                                                                           | Retry with backoff; escalate if persistent                                                                                     |

### Device Flow Polling Errors

While polling `/oauth/token` with `grant_type=urn:ietf:params:oauth:grant-type:device_code`:

| `error`                 | Meaning                               | Your action                                     |
| ----------------------- | ------------------------------------- | ----------------------------------------------- |
| `authorization_pending` | User hasn't approved yet              | Keep polling at `interval`                      |
| `slow_down`             | Polling too fast                      | **Increase `interval` by ≥ 5 seconds**          |
| `access_denied`         | User rejected                         | Stop. Tell user.                                |
| `expired_token`         | `device_code` past `expires_in`       | Restart the flow from `POST /oauth/device/code` |
| `invalid_grant`         | `device_code` unknown or already used | Restart the flow                                |

See [Device Flow](./device-flow) for full details.

### Token Introspection & Validation

| Endpoint                 | Failure mode                      | Response                                                 |
| ------------------------ | --------------------------------- | -------------------------------------------------------- |
| `GET /oauth/tokeninfo`   | Missing Bearer header             | `401` `{"error": "missing_token"}`                       |
| `GET /oauth/tokeninfo`   | Invalid or expired token          | `401` `{"error": "invalid_token", ...}`                  |
| `GET /oauth/userinfo`    | Missing/invalid Bearer            | `401` + `WWW-Authenticate: Bearer error="invalid_token"` |
| `POST /oauth/introspect` | Missing/invalid client auth       | `401` + `WWW-Authenticate: Basic realm="authgate"`       |
| `POST /oauth/introspect` | Token invalid / expired / revoked | `200` `{"active": false}` (per RFC 7662 — never a 4xx)   |
| `POST /oauth/revoke`     | Any outcome                       | `200` (per RFC 7009 — no error signal)                   |

## Rate Limit Errors — HTTP 429

If you exceed the per-IP rate limit, you'll get `429 Too Many Requests`:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 30
Content-Type: application/json

{"error": "rate_limit_exceeded", "error_description": "..."}
```

**How to handle**:

- **Always honor `Retry-After`** if present.
- Back off exponentially with jitter on repeated 429s.
- If you're polling (Device Flow), your `interval` should already keep you well below the limit. If you see 429, you're polling wrong — fix the client, don't retry faster.
- For shared environments (multiple services behind one egress IP), consider getting your admin to whitelist the IP or raise the limit.

See [Tokens & Revocation §Rate Limits](./tokens#rate-limits) for the defaults.

## Special Case: Refresh Token Reuse → Family Revocation

In rotation mode, using a previously-rotated refresh token returns `invalid_grant`, and **every refresh token in the same family is revoked** server-side. This is a **terminal state** — do not retry.

```json
{
  "error": "invalid_grant",
  "error_description": "Refresh token is invalid or expired"
}
```

What caused it:

- Two tabs/processes refreshed concurrently using the same stored token
- A retry after a partial failure where you didn't persist the new token
- A stolen token was used by someone else first

**Response**: force the user to log in again. See [Tokens & Revocation §Rotation Mode](./tokens#rotation-mode-the-reuse-detection-gotcha) for prevention patterns.

## Resource Indicator Errors (RFC 8707)

`invalid_target` is returned whenever a `resource=` parameter fails validation. Two categories:

**Shape validation** (applies to every endpoint that accepts `resource`):

| Cause                                                                 | Fix                                               |
| --------------------------------------------------------------------- | ------------------------------------------------- |
| Not an absolute URI (e.g. `resource=/api`)                            | Pass the full `https://api.example.com` form      |
| Scheme is not `http` or `https` (e.g. `javascript:`, `urn:`, `data:`) | RFC 8707 requires a network-locator URI           |
| Contains a fragment (`#...`)                                          | Strip the fragment — `aud` cannot carry fragments |
| Empty host                                                            | Provide a real host name                          |
| More than 10 `resource=` values, or a single value > 1024 characters  | Reduce the list / shorten the URI                 |

**Subset rule** (RFC 8707 §2.2 — applies on the token endpoint for grants that carry a prior resource binding):

| Grant                                          | Rule                                                                                            |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `authorization_code`                           | `resource=` at `/oauth/token` must be a subset of what was sent on `/oauth/authorize`           |
| `urn:ietf:params:oauth:grant-type:device_code` | `resource=` at `/oauth/token` must be a subset of what was sent on `/oauth/device/code`         |
| `refresh_token`                                | `resource=` must be a subset of the original grant — widening is rejected, narrowing is allowed |
| `client_credentials`                           | Any `resource=` is accepted as long as it passes shape validation                               |

For `device_code`, the device code is **not consumed** on `invalid_target` — the CLI may retry with a corrected resource set. For `authorization_code`, however, the code is one-shot per RFC 6749, so the user must restart the flow.

See each flow's guide for example requests: [Authorization Code Flow](./auth-code-flow), [Device Authorization Flow](./device-flow), [Client Credentials Flow](./client-credentials).

## Error Handling Checklist

- [ ] Treat `invalid_grant` on refresh as terminal — trigger re-login, don't retry
- [ ] Treat `access_denied` as user-initiated — surface politely, don't auto-retry
- [ ] Retry `server_error` and network errors with exponential backoff
- [ ] Honor `Retry-After` on 429
- [ ] Log `error_description` server-side; **never** show it to end users
- [ ] `invalid_request` / `invalid_scope` / `unsupported_grant_type` / `unsupported_response_type` / `invalid_target` are client bugs — fix, don't retry
- [ ] Monitor `invalid_client` spikes — someone is probing your credentials or a rotation/leak happened

## Related

- [Getting Started](./getting-started)
- [Authorization Code Flow](./auth-code-flow)
- [Device Authorization Flow](./device-flow)
- [Client Credentials Flow](./client-credentials)
- [Tokens & Revocation](./tokens)
- [JWT Verification](./jwt-verification)
- [OpenID Connect](./oidc)
