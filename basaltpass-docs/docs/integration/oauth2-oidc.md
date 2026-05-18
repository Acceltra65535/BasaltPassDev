---
sidebar_position: 3
---

# OAuth 2.0 & OIDC Endpoints

BasaltPass implements OAuth 2.0 Authorization Code flow and the OpenID Connect
Core profile used by common web, SPA, mobile, and backend clients. The current
OIDC surface is centered on `response_type=code`, PKCE, RS256 signed ID Tokens,
JWKS-based verification, UserInfo, introspection, revocation, refresh tokens,
and RP-initiated logout.

BasaltPass was validated against the OpenID Foundation conformance suite using
the Basic OP static-client profile. The latest local run completed 36 modules
with 1789 successful conditions and no failures or warnings. Some conformance
modules still require human review for browser screenshots or redirect behavior,
which is normal for the suite.

## Supported Profile

-   **Authorization flow**: Authorization Code (`response_type=code`)
-   **PKCE**: S256 `code_challenge` / `code_verifier`
-   **ID Token signing**: RS256
-   **Discovery**: `/.well-known/openid-configuration`
-   **JWKS**: `/oauth/jwks`
-   **UserInfo**: `/oauth/userinfo`
-   **Refresh tokens**: only when `offline_access` is requested and the client allows `refresh_token`
-   **Client authentication**: `client_secret_basic`, `client_secret_post`, `none`, `client_secret_jwt`, `private_key_jwt`
-   **Subject identifiers**: public subject and pairwise subject support
-   **Logout**: RP-initiated logout through `end_session_endpoint`
-   **Unsupported response types**: implicit and hybrid flows are not advertised

## Discovery
The easiest way to configure your client library is using the Discovery Document:
-   **URL**: `/.well-known/openid-configuration`
-   **Method**: `GET`
-   **Response**: JSON containing issuer, endpoints, and supported capabilities.

If your client library supports discovery, prefer discovery over hard-coding
endpoint URLs. BasaltPass publishes the issuer, authorization endpoint, token
endpoint, JWKS URI, UserInfo endpoint, revocation endpoint, introspection
endpoint, end-session endpoint, supported scopes, supported claims, supported
response types, subject types, and token endpoint authentication methods.

## Key Endpoints

### Authorization Endpoint
-   **Path**: `/oauth/authorize`
-   **Method**: `GET`
-   **Usage**: Redirect the user's browser here to start login.
-   **Params**: `client_id`, `redirect_uri`, `response_type=code`, `scope`, `state`, `nonce`, `code_challenge` (PKCE).
-   **Security requirement**: `state` is mandatory and must be an unguessable per-request value. BasaltPass validates `state` on callback and rejects mismatches with `400 invalid state`.
-   **OIDC requirement**: when `scope` contains `openid`, `nonce` is required. The returned `id_token` includes the same nonce and clients must compare it with the value stored before redirect.
-   **Integration impact**: If your client already sends a unique `state` and returns it unchanged in the callback, no changes are required.

Common optional parameters:

-   `prompt=none`: attempts silent authentication. If user interaction is required, BasaltPass returns an OIDC-compatible error such as `login_required` or `interaction_required`.
-   `prompt=login`: forces a fresh authentication step.
-   `prompt=consent`: forces the consent screen even when prior consent exists.
-   `max_age`: requires the previous authentication time to be recent enough.
-   `login_hint`: pre-fills or hints the account identifier.
-   `claims`: requests specific ID Token or UserInfo claims.
-   `acr_values`: requests a preferred authentication context class.
-   `request`: supports minimal unsigned request objects using `alg=none`.

### Token Endpoint
-   **Path**: `/oauth/token`
-   **Method**: `POST`
-   **Usage**: Exchange `authorization_code` for tokens, or refresh an existing token.
-   **Auth**: Basic Auth, client secret in body, public-client PKCE, `client_secret_jwt`, or `private_key_jwt`.

Authorization code request:

```http
POST /api/v1/oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic base64(client_id:client_secret)

grant_type=authorization_code&
code={CODE}&
redirect_uri={REDIRECT_URI}&
code_verifier={CODE_VERIFIER}
```

When `scope` contains `openid`, the token response includes an `id_token`.
Clients should validate the ID Token with the JWKS endpoint and verify at least
`iss`, `sub`, `aud`, `exp`, `iat`, and `nonce`.

Refresh request:

```http
POST /api/v1/oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic base64(client_id:client_secret)

grant_type=refresh_token&
refresh_token={REFRESH_TOKEN}
```

BasaltPass only returns a refresh token when the original authorization request
included `offline_access` and the client allows the `refresh_token` grant. If
the refresh token scope includes `openid`, a refreshed response also includes a
new ID Token.

### Token Introspection
-   **Path**: `/oauth/introspect`
-   **Method**: `POST`
-   **Usage**: Check whether an access token or refresh token is still active.
-   **Response fields**: `active`, `token_type`, `scope`, `client_id`, `sub`, `aud`, `iss`, `iat`, `nbf`, `exp`.

Introspection is useful for opaque access tokens, immediate revocation checks,
or services that do not want to implement local JWT verification.

### Token Revocation
-   **Path**: `/oauth/revoke`
-   **Method**: `POST`
-   **Usage**: Revoke an access token or refresh token.

### One-Tap / Silent Auth (Hardened)
-   **Paths**:
  -   `POST /oauth/one-tap/login`
  -   `GET /oauth/silent-auth?prompt=none`
-   **Important change**: These endpoints now issue an OAuth2 `authorization_code` only. They do **not** return `id_token` directly.
-   **Security checks**:
  -   Client must be registered and active.
  -   `redirect_uri` must exactly match registered URIs.
  -   User must belong to the client tenant.
  -   User must have prior consent/authorization for the app (otherwise `interaction_required`).
-   **Integration flow**:
  -   Receive `code` (+ optional `state`) from One-Tap/Silent-Auth.
  -   Exchange `code` at `/oauth/token` using standard OAuth2 flow.
  -   Use returned `access_token` for `/oauth/userinfo`.

### Social OAuth Callback Delivery
-   **Important change**: Social login callback no longer appends `?token=...` to the success URL.
-   **Current behavior**:
  -   Backend sets `HttpOnly` cookies (`access_token`, `refresh_token`) with `SameSite=Lax`.
  -   Frontend success page should call `POST /api/v1/auth/refresh` to obtain `access_token` for SPA storage/runtime use.

### UserInfo Endpoint
-   **Path**: `/oauth/userinfo`
-   **Method**: `GET` or `POST`
-   **Usage**: Get profile information for the authenticated user.
-   **Header**: `Authorization: Bearer <access_token>`

UserInfo claims are scope-controlled. `openid` returns the subject identifier.
`profile` adds standard profile fields such as `name`, `given_name`,
`family_name`, `middle_name`, `nickname`, `preferred_username`, `picture`,
`profile`, `website`, `gender`, `birthdate`, `locale`, `zoneinfo`, and
`updated_at` when available. `email` adds `email` and `email_verified`.
`phone` adds `phone_number` and `phone_number_verified`. `address` adds the
structured `address` claim.

### JWKS Endpoint
-   **Path**: `/oauth/jwks`
-   **Method**: `GET`
-   **Usage**: Get public keys to verify JWT signatures locally.

BasaltPass persists OIDC signing keys and exposes active and standby keys
through JWKS. Key rotation keeps old keys available long enough for previously
issued ID Tokens to expire. Administrative rotation is available through the
OIDC signing-key rotation endpoint.

### End Session Endpoint
-   **Path**: `/end_session`
-   **Method**: `GET`
-   **Usage**: RP-initiated logout.
-   **Params**: `id_token_hint`, `post_logout_redirect_uri`, `state`.

`post_logout_redirect_uri` must be registered in the client allow-list. When it
is accepted, BasaltPass redirects back to it and preserves `state`.

## ID Token Claims

ID Tokens include OIDC core claims and selected profile claims based on scopes
and requested claims:

-   Required core claims: `iss`, `sub`, `aud`, `exp`, `iat`
-   Session and assurance claims: `auth_time`, `nonce`, `acr`, `amr`
-   Audience binding: `azp` when needed
-   Email claims: `email`, `email_verified`
-   Profile claims: `name`, `given_name`, `family_name`, `middle_name`,
    `nickname`, `preferred_username`, `picture`, `locale`, `zoneinfo`

For privacy-sensitive deployments, use `subject_type=pairwise` for clients or
sectors that should not share a globally correlatable `sub`.

## Minimal Client Checklist

1. Use discovery to read endpoints, issuer, supported scopes, and JWKS URI.
2. Start login with `response_type=code`, `scope` containing `openid`, a random `state`, a random `nonce`, and PKCE S256.
3. Store `state`, `nonce`, and `code_verifier` in the browser session or server session before redirecting.
4. Exchange `code` with `code_verifier`.
5. Validate `id_token` with JWKS and verify `iss`, `sub`, `aud`, `exp`, `iat`, and `nonce`.
6. Call UserInfo with the returned access token when the application needs profile data.
7. Request `offline_access` only when the application needs long-lived refresh.
8. Use `end_session_endpoint` for coordinated logout when RP-initiated logout is required.

## Conformance Test Notes

The repository contains local conformance helpers under the backend scripts and
conformance seed command. Local `.env` files and suite output files are not meant
to be committed. A typical conformance run uses a static client, HTTPS reverse
proxy, seeded user, and the OpenID Foundation Basic OP test plan.
