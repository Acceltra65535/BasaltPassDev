---
sidebar_position: 1
---

# 仓库结构

BasaltPass 是一个包含后端和前端代码的 monorepo (单仓库)。

## 目录布局

-   `basaltpass-backend/`: Go (Fiber + GORM) 应用。
-   `basaltpass-frontend/`: React (Vite) 应用。
-   `basaltpass-docs/`: 本文档站点。
-   `scripts/`: 稳定的全栈开发入口脚本。
-   `tools/`: 本地开发、测试数据和一次性辅助脚本。

## 后端结构

-   `cmd/basaltpass/`: 入口点。
-   `cmd/basaltpass-import/`: Basalt bundle 导入 CLI。
-   `tools/`: 需要引用 internal 包的后端本地开发工具。
-   `internal/api/v1/`: 路由定义。
-   `internal/handler/`: HTTP 请求处理器。
-   `internal/middleware/`: 认证、租户上下文、速率限制。
-   `internal/model/`: 数据库模型。
-   `internal/service/`: 业务逻辑 (Auth、User、Tenant 等)。

## 前端结构

-   `apps/`: 应用入口点 (admin、tenant、user)。
-   `src/`: 共享组件、hooks 和工具函数。

## 控制台作用域

系统通过 JWT 中的 `scp` (scope) 声明来区分访问权限：

-   `user`: 访问用户控制台 API。
-   `tenant`: 访问租户控制台 API。
-   `admin`: 访问平台管理 API。
