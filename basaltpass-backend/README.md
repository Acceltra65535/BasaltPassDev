# BasaltPass Backend

基于 Go Fiber 的多租户身份与权限后端服务。

## 技术栈

- 框架: Go Fiber
- ORM: GORM
- 数据库: MySQL / SQLite
- 认证: JWT

## 当前开发约定

- 默认开发端口: `8101`
- 默认开发配置来源: 仓库根目录 `.env`
- 默认本地 Docker 数据库: MySQL（compose 内服务名 `mysql`）

## 配置加载

后端支持从配置文件与环境变量加载设置。

- 配置文件: `basaltpass-backend/config/config.yaml`
- 环境变量: 以 `BASALTPASS_` 前缀覆盖
- `.env` 默认查找位置: 项目根目录 `./.env`
- 可通过 `BASALTPASS_ENV_FILE` 指定自定义路径

常见变量示例:

```env
JWT_SECRET=change-me
OIDC_KEY_ENCRYPTION_SECRET=change-me-to-a-stable-32-byte-or-64-hex-secret
BASALTPASS_VERIFICATION_PEPPER=change-me-to-a-stable-random-secret
BASALTPASS_SERVER_ADDRESS=:8101
BASALTPASS_DATABASE_DRIVER=mysql
BASALTPASS_DATABASE_DSN=basaltpass:basaltpass@tcp(mysql:3306)/basaltpass?charset=utf8mb4&parseTime=True&loc=Local
```

`OIDC_KEY_ENCRYPTION_SECRET` 用于加密持久化的 OIDC ID Token 签名私钥。生产环境应固定配置并妥善备份；未设置时会从 `JWT_SECRET` 派生。
`BASALTPASS_VERIFICATION_PEPPER` 用于验证码和注册风控哈希，生产环境应固定配置并妥善保存。

支持的主要字段:

- `server.address`
- `database.driver`
- `database.dsn`
- `database.path`
- `cors.allow_origins`

## 本地运行

### Docker Compose

在仓库根目录运行:

```bash
cd BasaltPass
docker compose up -d --build
```

启动后:

- Backend: `http://localhost:8101`
- Frontend: `http://localhost:5104`
- MySQL: `localhost:3307`

### 直接运行后端

```bash
cd BasaltPass/basaltpass-backend
go run ./cmd/basaltpass
```

如果从后端目录直接运行，程序仍会自动查找仓库根目录 `.env`。

## 数据库

- 本地 Docker 开发默认使用 MySQL
- `docker-compose.yml` 中的数据库数据通过 Docker volume 持久化
- 生产环境建议使用独立云数据库，并通过 `BASALTPASS_DATABASE_DSN` 注入连接串

## API

- 健康检查: `GET /health`
- OIDC Discovery: `GET /api/v1/.well-known/openid-configuration`
- OAuth Authorization: `GET /api/v1/oauth/authorize`
- Token Endpoint: `POST /api/v1/oauth/token`
- UserInfo: `GET|POST /api/v1/oauth/userinfo`
- JWKS: `GET /api/v1/oauth/jwks`
- Introspection: `POST /api/v1/oauth/introspect`
- Revocation: `POST /api/v1/oauth/revoke`
- RP-Initiated Logout: `GET /api/v1/end_session`
- S2S 接口说明: `../basaltpass-docs/docs/reference/s2s-api.md`

## OIDC 运行时约定

当前后端实现面向 OIDC Authorization Code profile：

- `response_type` 仅声明并支持 `code`，不暴露 implicit/hybrid flow。
- 当 `scope` 包含 `openid` 时，授权请求必须携带 `nonce`。
- 公共客户端应使用 PKCE S256；confidential client 可继续使用 client secret。
- token endpoint 在 `openid` scope 下返回 RS256 签名的 `id_token`。
- 客户端应通过 `/api/v1/oauth/jwks` 获取公钥并校验 `iss`、`sub`、`aud`、`exp`、`iat`、`nonce`。
- refresh token 仅在请求 `offline_access` 且客户端允许 `refresh_token` grant 时签发。
- refresh token introspection、access token introspection 和 revocation 均由 OAuth 端点处理。
- signing key 存入数据库并使用 `OIDC_KEY_ENCRYPTION_SECRET` 加密私钥；生产环境必须稳定配置该密钥。

管理端可触发 OIDC signing key rotation。轮换期间新 key 会先出现在 JWKS 中，旧 key 会保留到已签发 ID Token 过期后再隐藏，避免客户端缓存 JWKS 时出现验签失败。

更完整的客户端接入说明见 `../basaltpass-docs/docs/integration/oauth2-oidc.md`。

## Conformance 测试

本仓库包含本地 OpenID Foundation conformance suite 辅助文件：

- `cmd/conformance-seed`: 为本地 SQLite conformance 数据库写入 static client、用户和 profile 数据。
- `scripts/seed_conformance_sqlite.py`: 轻量 seed 脚本，适合快速重置本地测试数据。
- `conformance.config.yaml`: 本地 conformance 后端配置示例。

`conformance.env`、suite 日志、临时证书、测试结果 zip 和本地数据库属于运行产物，不应提交。

## 系统设置

系统设置默认存储在:

- `basaltpass-backend/config/settings.yaml`

可通过环境变量覆盖:

- `BASALTPASS_SETTINGS_FILE`

## 测试

```bash
cd BasaltPass/basaltpass-backend
go test ./...
```
