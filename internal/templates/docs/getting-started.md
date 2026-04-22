# Getting Started with AuthGate

This guide is for developers integrating an application **with** an existing AuthGate deployment. For operator/deployment docs (running the server, env vars, key generation), see the server README.

AuthGate is an OAuth 2.0 + OpenID Connect authorization server. It issues tokens that your app can use to authenticate users and call protected APIs.

## Pick a Flow

| Your application                                  | Recommended flow              |
| ------------------------------------------------- | ----------------------------- |
| Server-rendered web app (has a backend)           | Authorization Code + PKCE (confidential client) |
| Single-page app (React / Vue / Svelte / etc.)     | Authorization Code + PKCE (public client)       |
| Mobile or desktop app                             | Authorization Code + PKCE (public client)       |
| CLI tool, IoT device, or headless shell           | Device Authorization Grant    |
| Backend service calling another service (no user) | Client Credentials            |

Unsure? Use **Authorization Code + PKCE** for anything with a user and **Client Credentials** for service-to-service.

## Before You Integrate

Ask your AuthGate administrator for:

1. **Base URL** — e.g. `https://your-authgate`. Everything else you need is reachable from `BASE_URL/.well-known/openid-configuration` (see below).
2. **`client_id`** — identifies your application.
3. **`client_secret`** — only for *confidential* clients (server-side web apps, client-credentials services). Public clients (SPAs, mobile, CLIs) do not get a secret.
4. **Allowed redirect URIs** — for Authorization Code Flow. AuthGate does **exact-string matching**: `https://yourapp.example/cb` and `https://yourapp.example/cb/` are not the same URI.
5. **Allowed scopes** — which of `openid`, `profile`, `email`, `offline_access` this client may request. (Your admin may also have registered custom API scopes — ask which ones apply.)
6. **Enabled grant types** — which of Device Flow / Auth Code Flow / Client Credentials are turned on for this client.

## Start Here: OIDC Discovery

Instead of hardcoding endpoint URLs, fetch the OIDC Discovery document:

```bash
curl https://your-authgate/.well-known/openid-configuration
```

```json
{
  "issuer": "https://your-authgate",
  "authorization_endpoint": "https://your-authgate/oauth/authorize",
  "token_endpoint": "https://your-authgate/oauth/token",
  "userinfo_endpoint": "https://your-authgate/oauth/userinfo",
  "revocation_endpoint": "https://your-authgate/oauth/revoke",
  "jwks_uri": "https://your-authgate/.well-known/jwks.json",
  "response_types_supported": ["code"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "scopes_supported": ["openid", "profile", "email", "read", "write"],
  "token_endpoint_auth_methods_supported": ["client_secret_basic", "client_secret_post", "none"],
  "grant_types_supported": [
    "authorization_code",
    "urn:ietf:params:oauth:grant-type:device_code",
    "refresh_token",
    "client_credentials"
  ],
  "claims_supported": ["sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "at_hash", "name", "preferred_username", "email", "email_verified", "picture", "updated_at"],
  "code_challenge_methods_supported": ["S256"]
}
```

Most mature OAuth/OIDC libraries can consume this document directly and wire up the flow for you.

**A few gotchas with this document:**

- `jwks_uri` and `id_token_signing_alg_values_supported` are **only present when AuthGate is configured for RS256/ES256** (asymmetric signing). On HS256 deployments they're omitted.
- `/oauth/introspect` and `/oauth/device/code` are supported but **not advertised** in Discovery — use the paths shown in this guide directly.
- `offline_access` is accepted when requested even though it's not currently listed in `scopes_supported`.

## Supported Scopes

| Scope            | Purpose                                                                             |
| ---------------- | ----------------------------------------------------------------------------------- |
| `openid`         | Required to receive an **ID token** and use `/oauth/userinfo`                       |
| `profile`        | Unlocks `name`, `preferred_username`, `picture`, `updated_at` on UserInfo/ID token  |
| `email`          | Unlocks `email`, `email_verified` on UserInfo/ID token                              |
| `offline_access` | Signals that you want a refresh token (OIDC Core §11)                               |

Notes:

- `openid` and `offline_access` are **not valid** in the Client Credentials flow (rejected).
- A client can only request scopes the administrator registered for it.
- Scopes are sent as a **space-separated** string (`scope=openid profile email`).

## Tokens at a Glance

After a successful flow, AuthGate issues:

- **Access token** — JWT; short-lived; include as `Authorization: Bearer <token>` on API calls.
- **Refresh token** — opaque; longer-lived; trade for a new access token at `/oauth/token`.
- **ID token** — JWT about the user (only when `scope` contains `openid`). See [OpenID Connect](./oidc).

Access token lifetime varies per client (`short` ≈ 15m, `standard` ≈ 10h, `long` ≈ 24h). Always honor the `expires_in` field of the token response — **never hardcode a duration**.

Rate limits, revocation, introspection, refresh rotation: see [Tokens & Revocation](./tokens).

## A Minimal Integration Checklist

- [ ] Confirm `BASE_URL`, `client_id`, (`client_secret`?), redirect URIs, and scopes with your admin.
- [ ] Fetch `/.well-known/openid-configuration` at startup; cache it.
- [ ] Pick a flow and wire it up (see the per-flow docs below).
- [ ] Verify tokens at your resource servers using JWKS ([JWT Verification](./jwt-verification)).
- [ ] Handle the common OAuth errors ([Errors](./errors)).
- [ ] Implement sign-out: call `/oauth/revoke` with the refresh token ([Tokens & Revocation](./tokens)).
- [ ] If the client is public and long-lived, use PKCE (`S256` — the only method AuthGate accepts).

## Next Steps

- [Authorization Code Flow + PKCE](./auth-code-flow) — Web, SPA, and mobile apps
- [Device Authorization Flow](./device-flow) — CLI and headless clients
- [Client Credentials Flow](./client-credentials) — Service-to-service
- [OpenID Connect](./oidc) — ID tokens and UserInfo
- [JWT Verification](./jwt-verification) — Verify access tokens at resource servers
- [Tokens & Revocation](./tokens) — Refresh, revoke, introspect
- [Errors](./errors) — OAuth error codes and how to handle them
