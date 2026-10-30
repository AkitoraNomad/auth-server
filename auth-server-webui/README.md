# Auth Server Admin SPA

React + TypeScript + Vite 管理前端，入口固定在 `/admin-page/`。

## 配置

参考 [.env.example](.env.example) 创建 `.env`；以下均为公开配置，不得包含 secret：

```bash
VITE_ADMIN_CLIENT_ID=2
VITE_AUTH_BASE_URL=
VITE_API_BASE_URL=
VITE_ADMIN_REDIRECT_URI=
```

- `VITE_ADMIN_CLIENT_ID` 必须等于后端 seed 的 BIGINT 管理客户端 ID（默认 2）。
- `VITE_AUTH_BASE_URL`、`VITE_API_BASE_URL` 留空表示当前 origin。开发时通过 Vite 代理访问后端。
- 回调默认是 `<SPA origin>/admin-page/callback`，必须精确出现在后端 `ADMIN_CLIENT_REDIRECT_URIS` 中。
- 这里**没有** `VITE_ADMIN_CLIENT_SECRET`：管理客户端是 public client，使用 S256 PKCE。
- `VITE_*` 在开发/构建时读取；修改后需重启 Vite 或重新构建。当前后端没有 CORS 允许来源配置，跨域填写 Auth/API URL 不能直接完成独立部署。

## 开发

```bash
pnpm install --frozen-lockfile
pnpm dev
```

需要 Node.js 22.12+ 和 pnpm 11.3.0。打开 `http://localhost:5173/admin-page/`。后端设置 `OAUTH_ISSUER=http://localhost:5173`，使同源校验与浏览器 origin 一致；使用 `127.0.0.1` 时也应同步修改 issuer。需要后端、PostgreSQL、Redis 和 Turnstile 配置；完整初始化及首位管理员步骤见 [根目录 README](../README.md#本地运行)。

开发端口固定为 5173，以便回调 URL 与后端精确匹配。若修改端口，需要同步修改后端 `OAUTH_ISSUER`、`ADMIN_CLIENT_REDIRECT_URIS` 和前端显式设置的回调覆盖值。

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

默认 `go build` 不嵌入 SPA；开发仍使用 Vite 的 `5173` 服务。Docker 已使用多阶段构建自动完成前端 build 和 Go embed。Go 只嵌入管理 SPA，运行时仍需配置、登录模板及 Bulma CSS；部署文件要求见 [根目录 README](../README.md#构建与部署)。

## 认证行为

- Access/Refresh Token 只保存在内存，不写 `localStorage`；刷新页面会重新走授权服务器。
- 当前不持久化 consent，重新授权时仍需确认 Scope。
- PKCE verifier/state、返回路径和创建时间保存在 `sessionStorage`，10 分钟后失效，兑换成功或失败后清除。
- 回调会校验 state、防止重复兑换，并在兑换前从地址栏移除 code/state。
- API 统一注入 Bearer；401 会尝试一次刷新，失败后回到登录页；403 显示权限不足。
- 页面权限与服务端保持一致：用户/管理员/会话需要 `manage:system`，客户端需要 `manage:clients`。
- 退出调用服务端 `POST /logout`，撤销该用户所有现有授权和 SSO 会话；前端协调刷新与退出，仅在服务端成功后清理本地 Token。进行中禁用重复操作，失败保留当前页面并显示错误，成功后跳转登录页。
- 管理员写操作要求最近 15 分钟内通过凭据登录，刷新 Token 不延长窗口；超时后需退出并重新登录。
- 会话页返回用户记录，支持撤销所选用户所有现有授权和 SSO 会话；尚无真实活跃会话列表或单会话管理。
- 用户、客户端、管理员已有列表/新建/删除；编辑、启停、密码/secret 重置尚未实现。客户端可在创建时设置多个回调、Scope 和 grant_types。
- 客户端表单已移除 implicit；支持 authorization_code、refresh_token，或机密客户端的 client_credentials。

## 代理说明

开发服务器默认代理以下路径到 `http://localhost:8080`，可在启动 Vite 时通过 `BACKEND_URL` 环境变量修改目标：

`/assets`、`/oauth2`、`/login`、`/logout`、`/auth`、`/api`、`/registration`、`/userinfo`、`/introspect`、`/.well-known`

`/admin-page/*` 由 Vite 自己提供，不能被代理到后端，否则 SPA 路由会失效。

浏览器自动化回归及真实服务测试方式见 [测试总览](../test/README.md)。
