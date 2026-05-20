# MCP Integration Guide

AuthGate implements the OAuth 2.1 surface required by the
[Model Context Protocol (MCP) authorization spec](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization),
so it can act as a drop-in authorization server for any MCP deployment.

This guide covers what an MCP server (the resource server) advertises to
clients, what AuthGate provides on each side of the trust boundary, and how
to wire the two together.

## Trust boundary

| Component       | Owner           | Responsibility                                                                                                         |
| --------------- | --------------- | ---------------------------------------------------------------------------------------------------------------------- |
| MCP client      | The application | Discovers the AS, performs PKCE, sends `resource=<MCP-URL>`                                                            |
| MCP server (RS) | Your deployment | Publishes [RFC 9728 Protected Resource Metadata][rfc9728] pointing at AuthGate; verifies token signature, `iss`, `aud` |
| AuthGate (AS)   | This service    | Issues access/refresh tokens with audience bound to the MCP resource                                                   |

AuthGate does **not** publish RFC 9728 Protected Resource Metadata; that
belongs to each MCP server. The PRM document is what tells clients which
AuthGate URL to use.

[rfc9728]: https://datatracker.ietf.org/doc/html/rfc9728

## What to advertise on your MCP server

The MCP server's PRM document (`/.well-known/oauth-protected-resource`) must
advertise the AuthGate base URL as its authorization server. Example:

```json
{
  "resource": "https://mcp.example.com",
  "authorization_servers": ["https://auth.example.com"],
  "bearer_methods_supported": ["header"],
  "scopes_supported": ["read", "write"]
}
```

When an MCP client receives a 401 with `WWW-Authenticate: Bearer
resource_metadata="..."`, it fetches the PRM, follows
`authorization_servers[0]`, and asks AuthGate for metadata.

## AuthGate AS metadata

MCP clients try `/.well-known/oauth-authorization-server` (RFC 8414) first,
then fall back to OIDC discovery. AuthGate publishes both:

| URL                                       | Use                                                  |
| ----------------------------------------- | ---------------------------------------------------- |
| `/.well-known/oauth-authorization-server` | OAuth 2.0 AS metadata — curated, no OIDC-only fields |
| `/.well-known/openid-configuration`       | OIDC Provider metadata — unchanged                   |
| `/.well-known/jwks.json`                  | Public keys for `RS256`/`ES256` verification         |

The OAuth metadata response includes:

- `issuer`, `authorization_endpoint`, `token_endpoint`
- `introspection_endpoint`, `revocation_endpoint`
- `registration_endpoint` — only when `ENABLE_DYNAMIC_CLIENT_REGISTRATION=true`
- `grant_types_supported` — `authorization_code`, `device_code`,
  `refresh_token`, `client_credentials`
- `code_challenge_methods_supported` — `["S256"]` (PKCE `plain` is rejected)
- `token_endpoint_auth_methods_supported`,
  `introspection_endpoint_auth_methods_supported`,
  `revocation_endpoint_auth_methods_supported`

Browser-based MCP clients need cross-origin access to these endpoints. The
`/.well-known/*` group respects `CORS_ENABLED` / `CORS_ALLOWED_ORIGINS`
exactly like `/oauth/*`.

## PKCE requirement

MCP requires `code_challenge_method=S256`. AuthGate's behaviour aligns:

- Public clients (no client secret) **must** present an `S256` code challenge.
- `plain` is rejected (returns `invalid_request`).
- Confidential clients may also opt into PKCE; set `PKCE_REQUIRED=true` to
  force it across all clients.

## Dynamic Client Registration (RFC 7591)

MCP recommends DCR so clients can self-register without admin intervention.
AuthGate exposes `POST /oauth/register` when
`ENABLE_DYNAMIC_CLIENT_REGISTRATION=true`. An MCP client posts:

```http
POST /oauth/register HTTP/1.1
Content-Type: application/json

{
  "client_name": "Acme MCP CLI",
  "redirect_uris": ["http://127.0.0.1:1729/callback"],
  "grant_types": ["authorization_code"],
  "token_endpoint_auth_method": "none"
}
```

The response contains `client_id` and (for confidential clients) a one-time
`client_secret`. Restrict DCR with `DYNAMIC_CLIENT_REGISTRATION_TOKEN` to
require a pre-shared bearer token for registration.

## Audience binding via Resource Indicators (RFC 8707)

MCP clients send `resource=<MCP-URL>` on both `/authorize` and `/token`. The
issued JWT's `aud` claim is **bound to the requested resource**. AuthGate:

- Validates each `resource` value with the RFC 8707 §2.1 baseline rules
  (absolute URI, no fragment) **plus stricter operational requirements**:
  the scheme MUST be `http` or `https`, and the URI MUST have a non-empty
  host. Other absolute-URI shapes accepted by §2.1 — `urn:`-style
  identifiers, schemeless authorities, `https:foo` without a `//host` —
  are rejected with `error=invalid_target`. MCP servers must therefore use
  an HTTP(S) URL (typically the same value as the PRM document's
  `resource` field) as their resource identifier; `urn:` resource IDs are
  not supported even though they are technically valid under §2.1.
- Replaces the static `JWT_AUDIENCE` config for that token. When the caller
  does not send `resource`, the existing `JWT_AUDIENCE` is used as before.
- Persists the bound resource on the authorization code and on access/refresh
  token rows.
- Enforces RFC 8707 §2.2 on refresh: the caller may narrow the audience but
  never widen it. Widening returns 400 `invalid_target`.
- On `authorization_code` token exchange, validates that any token-time
  `resource` is a subset of what was bound at `/authorize`.
- **Enforces a per-client `AllowedResources` allowlist on every grant**
  (`client_credentials`, `authorization_code`, `device_code`, `refresh_token`):
  a client-supplied `resource` is accepted only when it is an **exact-string
  match** of an allowlist entry, otherwise the request fails with 400 / redirect
  `error=invalid_target`. The allowlist is **deny-all by default** — an empty
  allowlist rejects any client-supplied `resource`. Sending no `resource` is
  unaffected (the static `JWT_AUDIENCE` fallback still applies, since that value
  is operator-controlled, not client-controlled). Admins manage the allowlist in
  the client create/edit screen. See the [allowlist section](#per-client-resource-allowlist-rfc-8707)
  below, including the **breaking-change** note.

**Trust model:** the `aud` claim is server-attested for the user-delegated
grants (`authorization_code`, `device_code`, `refresh_token`) — the user
explicitly authorized that resource at consent time, and the access token's
`aud` matches what they approved. For `device_code` specifically, AuthGate
routes resource-bound device codes through an explicit confirmation page
that displays the requesting client and the requested resource(s) BEFORE
`AuthorizeDeviceCode` is called, regardless of whether the user arrived via
`verification_uri_complete` or typed the user code into the
`verification_uri` form. The MCP server must verify that `aud` matches its
own resource identifier before accepting the token — token replay against a
different MCP server with the same `iss`/signature must fail. Standard
verification still applies:

- Check the JWT signature against JWKS.
- `iss` matches AuthGate's configured `BASE_URL`.
- `exp` is in the future.
- **`type` claim equals `"access"`.** AuthGate also issues refresh tokens
  signed with the same key, but they carry `type: "refresh"` and never the
  per-request RFC 8707 resource as `aud` (refresh JWTs are signed with nil
  audience override and fall back to the static `JWT_AUDIENCE` config). A
  resource server that checks only signature/`iss`/`exp`/`aud` would silently
  accept a refresh token as a valid access token whenever `JWT_AUDIENCE` is
  configured to its own resource identifier. Reject any JWT whose `type` is
  not `"access"`, and configure `JWT_AUDIENCE` either unset or to an AS-only
  value (never a resource-server identifier).
- For tokens obtained via `client_credentials`, `sub` starts with `client:`
  (machine identity) — treat these distinctly from user-delegated tokens
  if your policy differs for them.

For full RS-side verification details — including JWKS fetching, `aud` shape
handling (string vs array), refresh-token-as-access-token confusion
mitigation, and code samples in Go / Python / Node — see the
[JWT Verification Guide](JWT_VERIFICATION.md), in particular the
[Audience Binding (RFC 8707)](JWT_VERIFICATION.md#audience-binding-rfc-8707)
section.

### Per-client resource allowlist (RFC 8707)

Each OAuth client carries an `AllowedResources` allowlist. A client may obtain a
token whose `aud` was derived from a client-supplied `resource` value **only**
when that value is an **exact-string match** of an allowlist entry. This applies
uniformly to all four grants — including the `client_credentials` (M2M) path,
which has no human consent gate. AuthGate is therefore the authority that
attests a given client may target a given resource, as RFC 8707 intends.

- **Deny-all by default.** An empty `AllowedResources` rejects _any_ non-empty
  client-supplied `resource` with `invalid_target`. The client still receives
  the operator-configured `JWT_AUDIENCE` fallback `aud` when it sends no
  `resource` parameter (that value is operator-controlled, not client-controlled).
- **Exact match, no normalization.** `https://api.x.com` ≠ `https://api.x.com/`
  and case is significant — enter exactly what clients will request. Entries are
  validated at client-save time against the same RFC 8707 §2.1 syntax rules as
  request-time `resource` values (absolute `http`/`https` URI, non-empty host, no
  fragment); malformed entries are rejected.
- **Managed by admins** in the client create/edit screen (tag picker).

> **Breaking change.** A client that currently passes `resource` will start
> receiving `invalid_target` until an admin populates its `AllowedResources`.
> This is an intentional security fix (the resource-indicator feature shipped
> recently in #187/#190/#192). Before upgrading, inventory the `resource` values
> your clients send and pre-populate each client's allowlist. Clients that never
> send `resource` are unaffected.

A resource server SHOULD still verify `aud == its-own-id` (defense-in-depth and
RFC 8707 RS-side hygiene), but it can now also rely on AuthGate having refused to
mint an out-of-allowlist `aud` in the first place. For user-delegated grants
(`authorization_code`, `device_code`) the user's consent screen additionally
displays the requested resource, so the binding is both user-attested and
allowlist-attested.

## curl walkthrough

```bash
# 1. Fetch AS metadata (the MCP-required endpoint).
curl -s http://localhost:8080/.well-known/oauth-authorization-server | jq '
  {issuer, authorization_endpoint, token_endpoint,
   introspection_endpoint, registration_endpoint,
   code_challenge_methods_supported}'
# Expect: code_challenge_methods_supported = ["S256"];
# registration_endpoint present when ENABLE_DYNAMIC_CLIENT_REGISTRATION=true.

# 2. Confirm CORS preflight on the metadata endpoint. A real browser preflight
#    is an OPTIONS request carrying Access-Control-Request-Method.
curl -i -X OPTIONS \
  -H "Origin: https://allowed.example.com" \
  -H "Access-Control-Request-Method: GET" \
  http://localhost:8080/.well-known/oauth-authorization-server \
  | grep -i access-control-allow-origin
# Expect: Access-Control-Allow-Origin: https://allowed.example.com

# 3. Run the authorization-code flow with a resource indicator.
#    (Perform interactive consent in a browser, then exchange the code.)
curl -s -X POST http://localhost:8080/oauth/token \
  -d grant_type=authorization_code -d "code=$CODE" -d "redirect_uri=$RURI" \
  -d "client_id=$CID" -d "code_verifier=$CV" \
  -d "resource=https://mcp.example.com"
# Decode the access_token's payload; "aud" must equal "https://mcp.example.com".

# 4. Refresh requesting a resource outside the original grant — must fail.
curl -X POST http://localhost:8080/oauth/token \
  -d grant_type=refresh_token -d "refresh_token=$RT" -d "client_id=$CID" \
  -d "resource=https://forbidden.example.com"
# Expect: 400 {"error":"invalid_target",...}
```

## Configuration checklist

For an MCP-ready deployment:

- `BASE_URL=https://auth.example.com` (your AuthGate's public URL)
- `JWT_SIGNING_ALGORITHM=RS256` or `ES256` (asymmetric keys exposed via JWKS)
- `CORS_ENABLED=true` and `CORS_ALLOWED_ORIGINS=<browser MCP client origins>`
- `ENABLE_DYNAMIC_CLIENT_REGISTRATION=true` if you want self-service MCP clients
- `ENABLE_REFRESH_TOKENS=true` (long-running MCP sessions)
- `PKCE_REQUIRED=true` recommended; AuthGate already requires `S256` for public
  clients and rejects `plain`.
- `STRICT_REDIRECT_URIS=true` recommended. OAuth 2.1 §1.5 requires every redirect
  URI to be loopback or HTTPS. With this flag on, AuthGate rejects a redirect URI
  that uses plain `http://` to a non-loopback host (`http://127.0.0.1...`,
  `http://localhost...`, and any `https://...` are still accepted). Enforced when
  a client is created or updated — including via Dynamic Client Registration —
  so pre-existing clients keep working until they are next edited. Default is
  `false` for backward compatibility.

No new configuration keys are required to support MCP — Resource Indicators
are always-on and backward-compatible: callers that don't send `resource`
keep getting `aud` from `JWT_AUDIENCE`.

> ⚠️ **`JWT_AUDIENCE` operational constraint.** Refresh JWTs are signed with
> the static `JWT_AUDIENCE` (not the per-request `resource`) so they can be
> presented back to `/oauth/token`. **`JWT_AUDIENCE` MUST therefore be either
> unset or set to an AS-only identifier — never a resource server's `aud`.**
> Otherwise a refresh token whose `aud` happens to match an MCP server's
> expected audience could be accepted as if it were an access token by any RS
> that only verifies signature/`iss`/`exp`/`aud`. See
> [docs/CONFIGURATION.md](CONFIGURATION.md#environment-variables) (JWT Audience
> Claim section) for the full rationale.

## See also

- [JWT Verification Guide](JWT_VERIFICATION.md) — RS-side verification, JWKS,
  `aud`/`type` claim validation, code samples
- [Configuration Guide](CONFIGURATION.md) — `JWT_AUDIENCE`, CORS, OAuth client
  setup
- [Authorization Code Flow Guide](AUTHORIZATION_CODE_FLOW.md) — `/oauth/authorize`
  parameters including `resource`
- [Client Credentials Flow Guide](CLIENT_CREDENTIALS_FLOW.md) — M2M tokens and
  the per-client resource allowlist
