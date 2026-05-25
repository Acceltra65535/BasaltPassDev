---
sidebar_position: 3
---

# OAuth 2.0 & OIDC 端点

BasaltPass 实现 OAuth 2.0 Authorization Code 流程，以及主流 Web、SPA、移动端和后端客户端常用的 OpenID Connect Core 能力。当前 OIDC 能力以 `response_type=code`、PKCE、RS256 签名 ID Token、JWKS 验签、UserInfo、introspection、revocation、refresh token 和 RP-Initiated Logout 为核心。

BasaltPass 已使用 OpenID Foundation conformance suite 的 Basic OP static-client profile 做过本地验证。最近一次完整运行完成 36 个测试模块，1789 个条件成功，0 个失败，0 个 warning。部分模块仍是 conformance suite 的人工 review 类型，例如浏览器截图或跳转行为确认，这是该测试套件的正常模式。

## 支持能力

-   **授权流程**：Authorization Code（`response_type=code`）
-   **PKCE**：S256 `code_challenge` / `code_verifier`
-   **ID Token 签名**：RS256
-   **Discovery**：`/.well-known/openid-configuration`
-   **JWKS**：`/oauth/jwks`
-   **UserInfo**：`/oauth/userinfo`
-   **Refresh Token**：仅当请求 `offline_access` 且客户端允许 `refresh_token` grant 时返回
-   **客户端认证**：`client_secret_basic`、`client_secret_post`、`none`、`client_secret_jwt`、`private_key_jwt`
-   **Subject 类型**：支持 public subject 和 pairwise subject
-   **Logout**：通过 `end_session_endpoint` 支持 RP-Initiated Logout
-   **不支持的 response type**：不声明 implicit / hybrid 流程

## 发现服务 (Discovery)
配置客户端库最简单的方法是使用发现文档：
-   **URL**: `/.well-known/openid-configuration`
-   **Method**: `GET`
-   **Response**: 包含发行者(issuer)、端点和支持功能的 JSON 数据。

如果你的客户端库支持 discovery，优先使用 discovery，不要硬编码全部端点。BasaltPass 会发布 issuer、authorization endpoint、token endpoint、JWKS URI、UserInfo endpoint、revocation endpoint、introspection endpoint、end-session endpoint、支持的 scopes、支持的 claims、支持的 response types、subject types 和 token endpoint auth methods。

## 关键端点

### 授权端点 (Authorization Endpoint)
-   **Path**: `/oauth/authorize`
-   **Method**: `GET`
-   **Usage**: 将用户浏览器重定向到此处以开始登录。
-   **Params**: `client_id`, `redirect_uri`, `response_type=code`, `scope`, `state`, `nonce`, `code_challenge` (PKCE).
-   **安全要求**: `state` 为必需参数，且必须是每次请求唯一、不可预测的随机值。BasaltPass 在回调时会严格校验 `state`，不匹配将返回 `400 invalid state`。
-   **OIDC 要求**：当 `scope` 包含 `openid` 时，`nonce` 为必需参数。返回的 `id_token` 会包含同一个 nonce，客户端必须和登录前保存的值进行比较。
-   **对接影响**: 如果你的接入方已经按标准传递随机 `state` 并在回调中原样返回，一般无需额外改动。

常见可选参数：

-   `prompt=none`：尝试静默登录。如果需要用户交互，BasaltPass 会返回 `login_required` 或 `interaction_required` 等 OIDC 兼容错误。
-   `prompt=login`：强制重新认证。
-   `prompt=consent`：强制重新展示授权同意。
-   `max_age`：要求最近一次认证时间足够新。
-   `login_hint`：预填或提示账号标识。
-   `claims`：请求特定 ID Token 或 UserInfo claims。
-   `acr_values`：请求指定认证上下文等级。
-   `request`：支持最小 unsigned request object，算法为 `alg=none`。

### 令牌端点 (Token Endpoint)
-   **Path**: `/oauth/token`
-   **Method**: `POST`
-   **Usage**: 用 `authorization_code` 交换令牌，或刷新已有令牌。
-   **Auth**: Basic Auth、body 中的 client secret、公共客户端 PKCE、`client_secret_jwt` 或 `private_key_jwt`。

授权码交换示例：

```http
POST /api/v1/oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic base64(client_id:client_secret)

grant_type=authorization_code&
code={CODE}&
redirect_uri={REDIRECT_URI}&
code_verifier={CODE_VERIFIER}
```

当 `scope` 包含 `openid` 时，token response 会包含 `id_token`。客户端应使用 JWKS 端点验签，并至少校验 `iss`、`sub`、`aud`、`exp`、`iat` 和 `nonce`。

刷新令牌示例：

```http
POST /api/v1/oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic base64(client_id:client_secret)

grant_type=refresh_token&
refresh_token={REFRESH_TOKEN}
```

BasaltPass 只有在原始授权请求包含 `offline_access` 且客户端允许 `refresh_token` grant 时才返回 refresh token。如果 refresh token 对应 scope 包含 `openid`，刷新响应也会包含新的 ID Token。

### Token Introspection
-   **Path**: `/oauth/introspect`
-   **Method**: `POST`
-   **Usage**: 检查 access token 或 refresh token 是否仍然有效。
-   **Response fields**: `active`, `token_type`, `scope`, `client_id`, `sub`, `aud`, `iss`, `iat`, `nbf`, `exp`.

Introspection 适用于 opaque access token、需要即时撤销检查，或不希望在服务侧实现本地 JWT 验签的场景。

### Token Revocation
-   **Path**: `/oauth/revoke`
-   **Method**: `POST`
-   **Usage**: 撤销 access token 或 refresh token。

### One-Tap / Silent-Auth（安全加固后）
-   **路径**：
  -   `POST /oauth/one-tap/login`
  -   `GET /oauth/silent-auth?prompt=none`
-   **重要变更**：以上端点现在只返回 OAuth2 `authorization_code`，不再直接返回 `id_token`。
-   **安全校验**：
  -   client 必须已注册且处于激活状态。
  -   `redirect_uri` 必须与已注册地址精确匹配。
  -   当前用户必须属于该 client 对应的租户。
  -   用户必须已经对该应用完成过授权同意（否则返回 `interaction_required`）。
-   **对接流程**：
  -   从 One-Tap / Silent-Auth 获取 `code`（及可选 `state`）。
  -   使用标准 OAuth2 流程调用 `/oauth/token` 交换令牌。
  -   使用返回的 `access_token` 调用 `/oauth/userinfo`。

### 社交 OAuth 回调交付方式
-   **重要变更**：社交登录成功回调不再在 URL 上拼接 `?token=...`。
-   **当前行为**：
  -   后端通过 `HttpOnly` Cookie（`access_token`、`refresh_token`，`SameSite=Lax`）下发会话。
  -   前端成功页应调用 `POST /api/v1/auth/refresh` 获取 `access_token` 供 SPA 使用。

### 用户信息端点 (UserInfo Endpoint)
-   **Path**: `/oauth/userinfo`
-   **Method**: `GET` 或 `POST`
-   **Usage**: 获取已认证用户的个人信息。
-   **Header**: `Authorization: Bearer <access_token>`

UserInfo claims 按 scope 控制。`openid` 返回 subject 标识。`profile` 会在有数据时返回 `name`、`given_name`、`family_name`、`middle_name`、`nickname`、`preferred_username`、`picture`、`profile`、`website`、`gender`、`birthdate`、`locale`、`zoneinfo`、`updated_at` 等标准 profile 字段。`email` 返回 `email` 和 `email_verified`。`phone` 返回 `phone_number` 和 `phone_number_verified`。`address` 返回结构化 `address` claim。

### JWKS 端点 (JWKS Endpoint)
-   **Path**: `/oauth/jwks`
-   **Method**: `GET`
-   **Usage**: 获取公钥以在本地验证 JWT 签名。

BasaltPass 会持久化 OIDC 签名密钥，并通过 JWKS 暴露 active 和 standby key。密钥轮换时，旧 key 会保留到之前签发的 ID Token 过期后再隐藏。管理端可通过 OIDC signing-key rotation endpoint 触发轮换。

### End Session 端点
-   **Path**: `/end_session`
-   **Method**: `GET`
-   **Usage**: RP-Initiated Logout。
-   **Params**: `id_token_hint`, `post_logout_redirect_uri`, `state`.

`post_logout_redirect_uri` 必须在客户端允许列表中。校验通过后，BasaltPass 会跳转回该地址，并原样带回 `state`。

## ID Token Claims

ID Token 包含 OIDC core claims，并按 scope 与 claims 请求加入部分 profile claims：

-   必需 core claims：`iss`、`sub`、`aud`、`exp`、`iat`
-   会话与认证强度 claims：`auth_time`、`nonce`、`acr`、`amr`
-   audience 绑定：必要时包含 `azp`
-   邮箱 claims：`email`、`email_verified`
-   Profile claims：`name`、`given_name`、`family_name`、`middle_name`、`nickname`、`preferred_username`、`picture`、`locale`、`zoneinfo`

如果部署场景需要降低跨应用身份关联风险，可以为客户端或 sector 启用 `subject_type=pairwise`，让不同应用看到不同的 `sub`。

## 最小客户端接入清单

1. 使用 discovery 读取端点、issuer、支持 scopes 和 JWKS URI。
2. 登录请求使用 `response_type=code`，`scope` 包含 `openid`，并携带随机 `state`、随机 `nonce`、PKCE S256。
3. 跳转前将 `state`、`nonce`、`code_verifier` 存在浏览器 session 或服务端 session。
4. 用 `code` 和 `code_verifier` 调用 token endpoint。
5. 使用 JWKS 验证 `id_token`，并校验 `iss`、`sub`、`aud`、`exp`、`iat`、`nonce`。
6. 需要 profile 时，用 access token 调用 UserInfo。
7. 只有确实需要长期刷新时才请求 `offline_access`。
8. 需要统一登出时，使用 `end_session_endpoint`。

## Conformance 测试说明

仓库中包含本地 conformance seed 命令和脚本。`.env` 类本地配置和 conformance suite 输出文件不应提交。典型运行方式使用 static client、HTTPS reverse proxy、seed 用户和 OpenID Foundation Basic OP 测试计划。
