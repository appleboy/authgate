# OpenID Connect (ID Tokens & UserInfo)

AuthGate supports **OpenID Connect 1.0** on top of the Authorization Code Flow. When you include `openid` in your requested `scope`, AuthGate issues an **ID token** alongside the access token and makes `/oauth/userinfo` available.

> **Device Flow does not currently issue ID tokens.** For OIDC you need the [Authorization Code Flow](./auth-code-flow).

## ID Token vs. Access Token

| Question                                 | ID Token                                       | Access Token                                                                                                                                   |
| ---------------------------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Who is it _about_?                       | The end user (identity)                        | An authorization to call an API                                                                                                                |
| Who is it _for_?                         | **Your client application** (`aud=client_id`)  | The resource server(s) the token is bound to (`aud`=per-request `resource` or `JWT_AUDIENCE`)                                                  |
| Sent to APIs as `Authorization: Bearer`? | **No** — never                                 | Yes                                                                                                                                            |
| Validate `aud`?                          | **Yes — must equal your `client_id`**          | **Yes — must equal the resource server's identifier** (see [JWT Verification §Audience Binding](./jwt-verification#audience-binding-rfc-8707)) |
| Validate `nonce`?                        | Yes — must match what you sent                 | N/A                                                                                                                                            |
| Contains PII?                            | Yes (email, name, picture, depending on scope) | No                                                                                                                                             |

**Rule of thumb**: only your own client app should ever parse the ID token. Pass it to another service and you're leaking the user's identity to a party that isn't the audience.

## Request an ID Token

In the Authorization Code Flow, include `openid` in `scope` and a `nonce`:

```
GET /oauth/authorize
  ?client_id=YOUR_CLIENT_ID
  &redirect_uri=https://yourapp.example/callback
  &response_type=code
  &scope=openid profile email
  &state=RANDOM_STATE
  &nonce=RANDOM_NONCE
  &code_challenge=CODE_CHALLENGE
  &code_challenge_method=S256
```

After the code exchange at `/oauth/token`, the response includes an `id_token`:

```json
{
  "access_token": "eyJhbG...",
  "refresh_token": "def502...",
  "id_token": "eyJhbG...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "openid profile email"
}
```

## ID Token Claims

**Header:**

```json
{
  "alg": "RS256",
  "kid": "abc123...",
  "typ": "JWT"
}
```

**Payload** (shape depends on the granted scopes):

| Claim                | Always | When added                                       | Meaning                                                         |
| -------------------- | ------ | ------------------------------------------------ | --------------------------------------------------------------- |
| `iss`                | ✓      |                                                  | Issuer URL — must equal your AuthGate `BASE_URL`                |
| `sub`                | ✓      |                                                  | Stable user identifier (UUID)                                   |
| `aud`                | ✓      |                                                  | Your `client_id` — **must match** for the token to be valid     |
| `exp`                | ✓      |                                                  | Expiration (Unix time)                                          |
| `iat`                | ✓      |                                                  | Issued-at (Unix time)                                           |
| `auth_time`          | ✓      |                                                  | When the user authenticated (Unix time)                         |
| `jti`                | ✓      |                                                  | Unique token ID                                                 |
| `nonce`              | —      | If you sent `nonce` in the authorization request | Must equal the value you sent — prevents replay                 |
| `at_hash`            | —      | When an access token is co-issued                | First half of SHA-256(access_token), base64url-encoded          |
| `name`               | —      | `scope` includes `profile`                       | Full display name                                               |
| `preferred_username` | —      | `scope` includes `profile`                       | Username for display (e.g. `alice`)                             |
| `picture`            | —      | `scope` includes `profile` and user has avatar   | Avatar URL                                                      |
| `updated_at`         | —      | `scope` includes `profile`                       | Profile last-updated (Unix time)                                |
| `email`              | —      | `scope` includes `email`                         | Primary email                                                   |
| `email_verified`     | —      | `scope` includes `email`                         | `true` if the email has been verified (e.g. via OAuth provider) |

## Verifying the ID Token

Use the same JWKS mechanics as access tokens ([JWT Verification](./jwt-verification)) but with **tighter rules**:

1. **Signature** — verify against the JWKS key matching the `kid` header.
2. **`iss`** — must equal your AuthGate `BASE_URL`.
3. **`aud`** — must equal your `client_id`. If `aud` is an array, it must contain your `client_id` and no untrusted values.
4. **`exp`** — must be in the future (small clock-skew tolerance, e.g. 30s).
5. **`iat`** — should be reasonably recent.
6. **`nonce`** — must equal the `nonce` you sent in the authorization request.
7. **`auth_time`** — if you requested `max_age`, enforce it.
8. **`at_hash`** _(optional, recommended)_ — verify it matches the access token you also received.

### Go (golang-jwt + keyfunc)

```go
import (
    "strings"
    "github.com/MicahParks/keyfunc/v3"
    "github.com/golang-jwt/jwt/v5"
)

jwksURL := "https://your-authgate/.well-known/jwks.json"
k, _ := keyfunc.NewDefault([]string{jwksURL})

token, err := jwt.Parse(idTokenString, k.Keyfunc,
    jwt.WithIssuer("https://your-authgate"),
    jwt.WithAudience(clientID),               // enforces aud
    jwt.WithExpirationRequired(),
    jwt.WithValidMethods([]string{"RS256", "ES256"}),
)
if err != nil {
    return fmt.Errorf("invalid id_token: %w", err)
}

claims := token.Claims.(jwt.MapClaims)
nonce, ok := claims["nonce"].(string)
if !ok || nonce != expectedNonce {
    return fmt.Errorf("nonce mismatch")
}
```

### Python (PyJWT)

```python
import jwt
from jwt import PyJWKClient

jwks_client = PyJWKClient(f"{AUTHGATE_URL}/.well-known/jwks.json")
signing_key = jwks_client.get_signing_key_from_jwt(id_token)

claims = jwt.decode(
    id_token,
    signing_key.key,
    algorithms=["RS256", "ES256"],
    issuer=AUTHGATE_URL,
    audience=CLIENT_ID,              # enforces aud
    options={"require": ["exp", "iss", "sub", "aud"]},
)

if claims.get("nonce") != expected_nonce:
    raise ValueError("nonce mismatch")
```

### Node.js (jose)

```javascript
import { createRemoteJWKSet, jwtVerify } from "jose";

const JWKS = createRemoteJWKSet(
  new URL(`${AUTHGATE_URL}/.well-known/jwks.json`),
);

const { payload } = await jwtVerify(idToken, JWKS, {
  issuer: AUTHGATE_URL,
  audience: CLIENT_ID, // enforces aud
  algorithms: ["RS256", "ES256"],
});

if (payload.nonce !== expectedNonce) throw new Error("nonce mismatch");
```

## UserInfo Endpoint

For scope-gated user claims at request time, call `/oauth/userinfo` with the **access token** (not the ID token):

```bash
curl -H "Authorization: Bearer ACCESS_TOKEN" https://your-authgate/oauth/userinfo
```

**Response** (shape depends on granted scopes):

```json
{
  "sub": "user-uuid",
  "iss": "https://your-authgate",
  "name": "Alice Example",
  "preferred_username": "alice",
  "picture": "https://...",
  "updated_at": 1700000000,
  "email": "alice@example.com",
  "email_verified": true
}
```

- Always includes `sub` and `iss`
- `profile` scope gates `name`, `preferred_username`, `picture`, `updated_at`
- `email` scope gates `email`, `email_verified`

On an invalid/expired token, UserInfo returns `401 Unauthorized` with `WWW-Authenticate: Bearer error="invalid_token"`.

**When to hit UserInfo vs. read ID token claims?** The ID token is a one-shot identity proof valid at login. For up-to-date profile data (e.g., the user changed their avatar), hit UserInfo with the current access token.

## Discovery

Your OIDC library should auto-configure from:

```
https://your-authgate/.well-known/openid-configuration
```

See [Getting Started](./getting-started#start-here-oidc-discovery) for the full document shape.

> AuthGate also publishes a parallel [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) document at `/.well-known/oauth-authorization-server` for non-OIDC OAuth 2.1 / MCP clients. The two documents are kept in sync for fields they share — pick whichever your library expects.

## Common Pitfalls

- **Sending the ID token as a Bearer to an API.** Don't. Use the access token.
- **Skipping `aud` validation.** Without it, an ID token issued for _another_ client could be accepted as yours.
- **Skipping `nonce` validation.** Always send and validate `nonce`. The spec marks it OPTIONAL for the auth-code flow, but omitting it forfeits replay protection and is strongly discouraged.
- **Parsing the ID token without verifying the signature.** Never — the JWT is _not_ authenticated until you verify it.
- **Validating `aud=client_id` on _access_ tokens.** Access tokens carry `aud=<resource server identifier>` (the RFC 8707 `resource` value or `JWT_AUDIENCE`), never your `client_id`. Use `aud=client_id` only when validating ID tokens.

## Related

- [Getting Started](./getting-started)
- [Authorization Code Flow](./auth-code-flow)
- [JWT Verification](./jwt-verification)
- [Tokens & Revocation](./tokens)
- [Errors](./errors)
