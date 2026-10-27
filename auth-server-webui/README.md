# Auth Server Admin SPA

React + TypeScript + Vite 管理前端，入口固定在 `/admin-page/`。

## 配置

复制 [.env.example](.env.example) 为 `.env`，不要提交真实密钥：

```bash
VITE_ADMIN_CLIENT_ID=2
VITE_AUTH_BASE_URL=
VITE_API_BASE_URL=
VITE_ADMIN_REDIRECT_URI=
```

- `VITE_ADMIN_CLIENT_ID` 必须等于后端 seed 的 BIGINT 管理客户端 ID（默认 2）。
- `VITE_AUTH_BASE_URL`、`VITE_API_BASE_URL` 留空表示当前 origin。开发时 Vite 代理 `/oauth2`、`/login`、`/auth`、`/api` 到 `http://localhost:8080`。
- 回调默认是 `<SPA origin>/admin-page/callback`，必须精确出现在后端 `ADMIN_CLIENT_REDIRECT_URIS` 中。
- 这里**没有** `VITE_ADMIN_CLIENT_SECRET`：管理客户端是 public client，使用 S256 PKCE。

## 开发

```bash
pnpm install
pnpm dev
```

打开 `http://localhost:5173/admin-page/`。后端设置 `OAUTH_ISSUER=http://localhost:5173`，使同源校验与浏览器 origin 一致；开发代理也转发 `/logout`。需要后端、PostgreSQL、Redis 和 Turnstile 配置；首位管理员通过 [../db/seed_admin.example.sql](../db/seed_admin.example.sql) 手工创建。

开发端口固定为 5173，以便回调 URL 与后端精确匹配。若修改端口，需要同步修改后端 `ADMIN_CLIENT_REDIRECT_URIS` 和 `.env` 中的回调覆盖值。

## 构建与检查

```bash
pnpm build
pnpm lint
```

`dist/` 是独立静态产物。生产可将其嵌入 Go 二进制，不需要单独启动静态服务器：

```bash
pnpm build
cd ..
go build -tags embedspa -o auth-server .
```

默认 `go build` 不嵌入 SPA；开发仍使用 Vite 的 `5173` 服务。Docker 已使用多阶段构建自动完成前端 build 和 Go embed。

## 认证行为

- Access/Refresh Token 只保存在内存，不写 `localStorage`；刷新页面会重新走授权服务器。
- PKCE verifier/state、返回路径和创建时间保存在 `sessionStorage`，10 分钟后失效，成功后清除。
- 回调会校验 state、防止重复兑换，并在兑换前从地址栏移除 code/state。
- API 统一注入 Bearer；401 会尝试一次刷新，失败后回到登录页；403 显示权限不足。
- 页面权限与服务端保持一致：用户/管理员/会话需要 `manage:system`，客户端需要 `manage:clients`。
- 会话列表与强制撤销尚未实现（Phase 4），页面会明确标注，不会返回假成功。

## 代理说明

开发服务器代理以下路径到后端：

`/oauth2`、`/login`、`/auth`、`/api`、`/registration`、`/userinfo`、`/introspect`、`/.well-known`

`/admin-page/*` 由 Vite 自己提供，不能被代理到后端，否则 SPA 路由会失效。
