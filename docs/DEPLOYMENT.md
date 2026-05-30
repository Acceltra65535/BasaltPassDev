# BasaltPass 部署说明

BasaltPass 可作为 SaaS 系统的统一认证服务独立部署。生产环境应先部署 BasaltPass，再让业务项目接入它提供的 OAuth/OIDC 端点。

## 1. 部署目标

- Backend API: `8101`
- 前端生产映射: `5104`
- 建议镜像:
  - `ghcr.io/<owner>/basaltpass-backend:<tag>`
  - `ghcr.io/<owner>/basaltpass-frontend:<tag>`

本仓库已经包含:

- `backend.Dockerfile`
- `frontend.Dockerfile`
- `docker-compose.yml`（本地开发/验证）
- `.env.example`

## 2. 镜像部署方式

建议使用 Tag 或 CI 构建生产镜像:

1. GitHub Actions 构建前后端镜像
2. 推送到 GHCR
3. 在目标环境拉取镜像
4. 注入生产 `.env` 或平台 Secret
5. 启动服务:

```bash
docker login ghcr.io -u "$GHCR_USERNAME" --password-stdin
docker compose pull
docker compose up -d --remove-orphans
```

建议镜像变量:

```env
BACKEND_IMAGE=ghcr.io/<owner>/basaltpass-backend:<tag>
FRONTEND_IMAGE=ghcr.io/<owner>/basaltpass-frontend:<tag>
```

## 3. 服务器准备

服务器目录建议固定为:

```bash
/opt/basaltpass
```

创建服务器实际使用的 `.env`，或使用 Kubernetes / 容器平台的 Secret 机制注入:

```env
JWT_SECRET=<long-random-secret>
BASALTPASS_ENV=production
BASALTPASS_SERVER_ADDRESS=:8101
BASALTPASS_CORS_ALLOW_ORIGINS=https://auth.example.com,https://admin.example.com
```

如需邮件能力，继续补齐 SMTP 或 SES 相关变量。

## 4. 生产编排

生产编排文件应至少传入:

- `${BACKEND_IMAGE}`
- `${FRONTEND_IMAGE}`
- `.env`

标准部署命令:

```bash
docker compose -f docker-compose.yml pull
docker compose -f docker-compose.yml up -d --remove-orphans
```

本仓库的 `docker-compose.yml` 偏向本地开发。公开仓库不携带环境专用的生产 compose 或集群 manifest；请根据你的服务器、Ingress、TLS、数据库和 Secret 管理方式维护生产编排。

## 5. 提供给下游项目的线上地址

建议统一暴露:

- BasaltPass 站点: `https://auth.example.com`
- Discovery: `https://auth.example.com/api/v1/.well-known/openid-configuration`
- OAuth authorize: `https://auth.example.com/api/v1/oauth/authorize`
- OAuth token: `https://auth.example.com/api/v1/oauth/token`
- OAuth userinfo: `https://auth.example.com/api/v1/oauth/userinfo`
- OAuth jwks: `https://auth.example.com/api/v1/oauth/jwks`
- OAuth introspect: `https://auth.example.com/api/v1/oauth/introspect`

## 6. 给其他项目接入时需要做的事

每个业务项目接入 BasaltPass 时，都要在 BasaltPass 内创建自己的 OAuth 客户端，必要时再创建 S2S 客户端。

常见配置项:

- `client_id`
- `client_secret`
- `redirect_uris`
- `scopes`
- `require_pkce`
- `grant_types`

## 7. 验收

- `https://auth.example.com/health` 正常
- `/.well-known/openid-configuration` 可返回
- 能在 BasaltPass 控制台创建 OAuth 客户端
- 下游项目能完成跳转登录和回调
