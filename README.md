# Auth Server

Go OAuth2 授权服务器，使用 Gin、PostgreSQL 和 Redis；管理前端位于 [auth-server-webui/](auth-server-webui/)，采用 React、TypeScript、Vite、Tailwind CSS 和 daisyUI。

## 项目现状（2026-10-30 核对）

以下区分代码已实现的能力与尚待完成的功能；测试范围见文末。

| 模块 | 当前实现与边界 |
|---|---|
| OAuth2 | 支持授权码 + S256 PKCE、刷新、机密客户端凭据；所有授权码流程强制 PKCE，已关闭 implicit，不支持密码 grant |
| 登录与注册 | 支持邮箱密码和 Telegram 登录，自助注册仅支持 Telegram；邮箱密码账号由管理员或 SQL seed 创建；登录/注册需要 Turnstile，Telegram 需额外配置 bot |
| 管理客户端 | 启动时幂等创建/更新固定 BIGINT ID 的公共客户端，默认 ID=2、JWT Token、无 secret；不自动创建管理员 |
| 管理权限 | 每次请求校验 JWT、Redis 撤销状态、管理客户端、真实用户、`auth_admin` 和路由 Scope；管理员写操作要求最近 15 分钟内通过凭据登录 |
| 管理页面 | 用户、客户端、管理员支持列表/新建/删除；客户端创建支持多个回调、Scope、grant_types，secret 仅创建时返回；编辑、启停、密码/secret 重置未实现 |
| 会话与退出 | 已实现服务端全局退出、用户级强制下线、刷新轮换及重放撤销；会话页列出的是用户记录，尚无真实活跃会话索引或单会话管理 |
| 元数据与令牌查询 | 已提供 Discovery、JWKS、Introspection、Bearer UserInfo；UserInfo 需要 `profile`，当前仅返回 `sub` |
| OIDC | 尚未实现 `openid`/`email` Scope、ID Token 和 nonce；提供 Discovery 不代表已完成 OIDC |
| 部署 | 支持 Vite 开发代理、Go 嵌入管理 SPA、Docker 多阶段构建；尚未提供跨域 CORS 配置或自动迁移执行器 |
| 数据库 | 当前运行代码仅使用 PostgreSQL；[db/mysql.sql](db/mysql.sql) 是遗留脚本，不代表当前支持 MySQL |

## 授权流程

```mermaid
sequenceDiagram
    autonumber
    participant B as Browser
    participant C as Client
    participant A as Auth Server
    participant R as Resource Server
    B->>C: 请求服务
    C-->>B: 跳转授权端点
    B->>A: client_id, response_type=code, redirect_uri, scope, state, PKCE challenge
    A-->>B: 登录与授权确认
    B->>A: 确认授权
    A-->>B: 校验回调地址，返回 code 和 state
    B->>C: 回调 code, state
    C->>A: code, redirect_uri, code_verifier, 客户端认证（按类型）
    A-->>C: access_token, refresh_token, scope
    C->>R: 携带 access_token 请求资源
    R-->>C: 资源响应
    C-->>B: 服务响应
```

回调 URL 必须与注册值精确匹配；生产使用 HTTPS，本机 HTTP 仅允许 loopback。公共客户端不能使用 `client_credentials`；机密客户端在兑换及刷新时均需认证。

## 本地运行

### 1. 环境与配置

需要 Go 1.26.0 或更高版本、PostgreSQL、Redis，以及可用的 Turnstile 配置。前端需要 Node.js 22.12+（Docker/CI 使用 Node 22）和 pnpm 11.3.0；数据库初始化示例使用 `psql`。

服务只从**当前工作目录**读取 `config.yaml`，该文件被 Git 忽略，不随仓库分发。先在项目根目录新建配置；下面示例用于本地 Vite 代理开发：

```yaml
host: 127.0.0.1
port: 8080
issuer: http://localhost:5173
trusted-proxies: []
db: ${DB:postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=auth}
redis:
  host: ${REDIS_HOST:127.0.0.1}
  port: "${REDIS_PORT:6379}"
cloudflare:
  turnstile:
    key: ${TURNSTILE_SITE_KEY}
    secret: ${TURNSTILE_SECRET_KEY}
# 仅本地开发：空列表会在每次启动时生成临时 RSA 密钥。
jwt: []
admin:
  client-id: 2
  redirect-uris:
    - "http://localhost:5173/admin-page/callback"
    - "http://127.0.0.1:5173/admin-page/callback"
    - "http://localhost:8080/admin-page/callback"
    - "http://127.0.0.1:8080/admin-page/callback"
  scopes: ["manage:system", "manage:clients"]
  grant-types: ["authorization_code", "refresh_token"]
```

将 `TURNSTILE_SITE_KEY`、`TURNSTILE_SECRET_KEY` 设为所用站点的有效配置。本地临时 JWT 密钥在重启后会变化，原 JWT 和签名会话 Cookie 将无法继续验证。需要稳定登录状态时配置持久化 RSA 密钥：

```yaml
jwt:
  - kid: rsa1
    alg: RS256
    sec: ${JWT_PRIVATE_KEY}
```

`JWT_PRIVATE_KEY` 是完整的 PKCS#1 PEM 私钥内容（`BEGIN RSA PRIVATE KEY`），不是文件路径。生产需妥善保存该密钥。JWT Access Token 使用首个 RSA 密钥签名，管理端需要 RS256。密码使用随机 salt 的 Argon2id PHC 哈希。可选 Telegram 登录使用 `telegram.bot-name` 和 `telegram.bot-token`；未配置 token 时服务端拒绝 Telegram 登录。

字段结构见 [config-schema.json](config-schema.json)，实际加载逻辑见 [internal/config.go](internal/config.go)。Schema 供配置参考，启动时不会执行 JSON Schema 校验。支持字符串及字符串数组中的 `${VAR}` / `${VAR:default}`，整数/布尔字段应直接使用 YAML 值。

服务**不会自动读取 `.env`**。根目录 [.env.example](.env.example) 是环境变量参考，可逐项 `export`，或交给 VS Code `envFile` / Docker `--env-file` 加载。`DB`、`REDIS_*`、Turnstile 等变量需在 YAML 中使用对应占位符；以下变量由代码直接覆盖配置：

| 变量 | 默认值 / 作用 |
|---|---|
| `OAUTH_ISSUER` | 覆盖公网 issuer；未指定时使用 YAML 的 `issuer`，再回退到监听地址 |
| `ADMIN_CLIENT_ID` | `2`；必须与前端 `VITE_ADMIN_CLIENT_ID` 一致 |
| `ADMIN_CLIENT_DISPLAY_NAME` | `Auth Server Admin` |
| `ADMIN_CLIENT_REDIRECT_URIS` | 逗号分隔的完整回调 URL；默认包含 localhost / 127.0.0.1 的 5173、8080 回调 |
| `ADMIN_CLIENT_SCOPES` | 默认 `manage:system,manage:clients`；必须同时包含两者 |
| `ADMIN_CLIENT_GRANT_TYPES` | 默认 `authorization_code,refresh_token`；必须含授权码，只允许这两类 |

`issuer` 必须是浏览器使用的公网 origin（协议、主机和端口一致，无路径前缀）。Vite 代理开发使用 `http://localhost:5173`；Go 独立托管使用 `http://localhost:8080`。使用 `127.0.0.1` 访问时也要同步修改 issuer。错误的 origin 会导致浏览器写请求被拒绝。

### 2. 初始化或升级 PostgreSQL

下面的 `DB` 应指向目标 PostgreSQL，并包含 `search_path=auth`；当前表及管理客户端序列固定使用 `auth` schema。

```bash
export DB='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=auth'
```

**仅全新数据库**使用基础 schema；该脚本包含 `DROP TABLE`，会删除现有认证数据：

```bash
psql "$DB" -v ON_ERROR_STOP=1 -f db/postgres.sql
```

已有数据库先备份，再按顺序应用增量迁移，保留原有数据：

```bash
psql "$DB" -v ON_ERROR_STOP=1 -f db/migrations/001_phase1_admin_spa.sql
psql "$DB" -v ON_ERROR_STOP=1 -f db/migrations/002_identity_constraints.sql
```

001 增加公共客户端、回调列表、grant_types 及管理员唯一约束；002 增加大小写不敏感邮箱和 `(provider_type, login_key)` 唯一索引，并移除已有客户端的 implicit grant。重复邮箱、外部身份或管理员记录会导致对应迁移失败并回滚，需先人工去重。全新 schema 已包含上述变更，无需再执行迁移。

升级后，没有安全元数据的旧授权码、Access/Refresh Token 和 SSO Session 会被拒绝，需要重新登录。密码只支持 Argon2id；旧 SHA-256 密码记录需重置。

### 3. 手工创建首位管理员

服务不自动创建管理员。使用 [db/seed_admin.example.sql](db/seed_admin.example.sql) 创建 `auth_user`、EmailPassword provider 和 `auth_admin` 记录：

```bash
go mod download
read -r -s PASSWORD
HASH=$(printf '%s' "$PASSWORD" | go run ./cmd/hash-password)
unset PASSWORD
psql "$DB" \
  -v admin_email='admin@example.com' \
  -v admin_display_name='Admin' \
  -v admin_password_hash="$HASH" \
  -f db/seed_admin.example.sql
unset HASH
```

使用至少 12 个字符的密码，与管理 API 的新建用户规则保持一致；哈希工具本身只校验非空及长度上限。工具从标准输入读取密码，输出随机 salt 的 Argon2id PHC 哈希。SQL seed 对已有 EmailPassword 记录不会改写密码，不能用作密码重置工具。

### 4. 启动后端和 SPA

在已配置 Turnstile 环境变量的终端中，从项目根目录运行：

```bash
export OAUTH_ISSUER=http://localhost:5173
export ADMIN_CLIENT_ID=2
go run .
```

服务启动时会创建或覆盖管理客户端的配置，**该 ID 必须没有被其他应用占用**。其回调、Scope、授权类型、公共客户端属性、空 secret 和 JWT 类型由服务端接管。

另开终端启动前端；首次配置时参考 [auth-server-webui/.env.example](auth-server-webui/.env.example) 创建前端 `.env`：

```bash
cd auth-server-webui
pnpm install --frozen-lockfile
pnpm dev
```

打开 `http://localhost:5173/admin-page/`，完成密码或 Telegram 登录与 Scope 同意。开发端口固定 5173；代理目标默认 `http://localhost:8080`，可通过启动 Vite 时的 `BACKEND_URL` 环境变量修改。

代理覆盖 `/assets`、`/oauth2`、`/login`、`/logout`、`/auth`、`/api`、`/registration`、`/userinfo`、`/introspect`、`/.well-known`；`/admin-page/*` 由 Vite 提供。

## 管理 SPA 与 API

详细前端操作见 [auth-server-webui/README.md](auth-server-webui/README.md)。沿用现有 React/Hooks、认证 Context、原生 `fetch` 和 daisyUI；前端 DTO 使用 snake_case。

| 代码位置 | 职责 |
|---|---|
| [src/App.tsx](auth-server-webui/src/App.tsx)、[src/main.tsx](auth-server-webui/src/main.tsx) | `/admin-page` basename、页面、权限门、回调与路由 |
| [src/lib/auth.tsx](auth-server-webui/src/lib/auth.tsx) | PKCE、state 校验、内存 Token、刷新与服务端退出 |
| [src/lib/api.ts](auth-server-webui/src/lib/api.ts)、[src/lib/config.ts](auth-server-webui/src/lib/config.ts) | Bearer 请求、401 单次刷新重试、公开配置、安全返回路径 |
| [src/routes/admin/](auth-server-webui/src/routes/admin/)、[src/types.ts](auth-server-webui/src/types.ts) | 管理页面与 DTO |
| [internal/http/gin_router.go](internal/http/gin_router.go)、[internal/http/admin/api.go](internal/http/admin/api.go) | 实际路由、管理员认证、Scope、近期认证及管理处理 |

- Access/Refresh Token 仅存内存，不写 `localStorage`；刷新页面后重新授权。当前不持久化 consent，因此仍需重新确认 Scope。
- verifier/state、创建时间与站内返回路径暂存 `sessionStorage`，10 分钟失效；回调校验 state、避免重复兑换，并清除地址栏 code/state。
- 退出会调用 `POST /logout`，使该用户所有现有授权和 SSO 会话失效，仅在服务端成功后清理本地 Token。退出进行中禁用重复操作，失败时保留当前页面并显示错误，成功后跳转登录页。普通刷新不会延长管理员写操作的 15 分钟凭据认证窗口，超时需退出并重新登录。
- 缺少页面 Scope 显示 403，未知页面显示 404；最终权限由服务端校验。删除最后一名管理员或其用户记录会被事务保护拒绝。

| SPA 路径（前缀 `/admin-page`） | JSON API / 权限 |
|---|---|
| `/login`、`/callback` | 登录入口、PKCE 回调；无对应管理写 API |
| `/` | `GET /api/admin/me`；首页需要至少一个管理 Scope |
| `/users` | `GET/POST /api/admin/users`、`DELETE /api/admin/users/:id`；`manage:system` |
| `/clients` | `GET/POST /api/admin/clients`、`DELETE /api/admin/clients/:id`；`manage:clients` |
| `/admins` | `GET/POST /api/admin/admins`、`DELETE /api/admin/admins/:user_id`；`manage:system` |
| `/sessions` | `GET /api/admin/sessions` 返回用户记录、`DELETE /api/admin/sessions/:user_id` 撤销该用户所有现有授权及 SSO；`manage:system` |

以上管理 API 均要求管理客户端的有效用户 Token 和管理员身份，写操作另校验近期认证。旧 `/admin/*` 路由已移除。

## OAuth2 及元数据端点

| 方法与路径 | 当前行为 |
|---|---|
| `GET/POST /oauth2/authorize` | 授权码请求与 POST 授权确认/拒绝；仅支持 `response_type=code`、S256 PKCE |
| `POST /oauth2/token` | 授权码兑换、刷新、机密客户端凭据；按客户端限制 Scope 和 grant |
| `GET /.well-known/openid-configuration` | 使用配置的 issuer；声明 code、S256、三种已实现 grant；尚无 ID Token |
| `GET /.well-known/jwks.json` | 发布配置密钥的 JWK；RSA 的 `n`/`e` 使用无填充 base64url |
| `POST /introspect` | HTTP Basic 认证，仅机密客户端可查询自身 Access Token；无效、撤销、不属自身的 Token 返回 `active:false` |
| `GET/POST /userinfo` | 支持 Bearer 用户 Access Token，要求 `profile`，当前只返回 `sub`；也保留 Basic + POST 表单 `token` 的同客户端查询方式 |
| `GET /logout` | 返回当前会话的 CSRF Token，不执行退出 |
| `POST /logout` | Bearer 认证，或 Cookie + `X-CSRF-Token`；撤销用户所有现有授权及 SSO，成功返回 204 |

当前 Scope 为 `profile`、`avatar`、`manage:system`、`manage:clients`。`avatar` 已定义，但 UserInfo 尚不返回头像或其他资料 claims。授权码 Access Token 默认有效期 2 小时，Refresh Token 默认 3 天；刷新带轮换和重放检测，不能扩大原 Scope。

## 构建与部署

### Go 托管管理 SPA

从项目根目录构建：

```bash
pnpm --dir auth-server-webui install --frozen-lockfile
pnpm --dir auth-server-webui build
go build -tags embedspa -o auth-server .
OAUTH_ISSUER=http://localhost:8080 ./auth-server
```

打开 `http://localhost:8080/admin-page/`。`-tags embedspa` 将 `auth-server-webui/dist/` 嵌入二进制；未先构建前端时，embed 构建会失败。默认 `go build ./...` 不嵌入 SPA，`/admin-page` 需由 Vite 或反向代理提供。

**嵌入的只有管理 SPA。** 运行目录仍需 `config.yaml`、`template/` 和 `assets/bulma.min.css`；根路径图片使用 `placeholder.jpg`，缺失时返回文字占位。复制二进制部署时需同时提供这些运行文件，无需额外启动前端静态服务器。

管理静态资源 `/admin-page/assets/*` 缓存一年并标记 immutable；首页和深链使用 `no-store`。缺失 JS/CSS 返回 404，不回退成 HTML。

`VITE_*` 是前端开发/构建时的公开配置，不得包含 secret；修改后需重启 Vite 或重新构建前端及嵌入二进制。独立静态产物可交给同源反向代理托管；当前后端没有 CORS 允许来源配置，直接换成跨域 Auth/API 地址并不能完成部署。

### Docker 与反向代理

[Dockerfile](Dockerfile) 使用 Node 构建前端、Go 嵌入 SPA，并以 UID/GID `10001:10001` 运行。配置不会复制进镜像，需挂载到 `/app/config.yaml`；文件和密钥需对该用户可读。

```bash
docker build -t auth-server:local .
# 先准备容器配置和 .env（包含 YAML 引用的全部变量）。
docker run --rm -p 127.0.0.1:8080:8080 \
  --env-file .env \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  auth-server:local
```

容器中的 `host` 应设为 `0.0.0.0`；PostgreSQL/Redis 地址需使用容器可访问的服务地址，不能沿用指向容器自身的 localhost。生产还需：

- HTTPS 反代、正确的 `OAUTH_ISSUER` 和精确的 HTTPS 管理回调；Go 当前通过 HTTP 监听，TLS 由反代终止。
- `trusted-proxies` 仅列实际代理的 IP/CIDR，登录限速才会采用其 `X-Forwarded-For`；默认忽略转发头。
- Redis 可配置 `username`、`password`、`tls: true`；它保存 Token、会话、刷新族和用户安全版本等状态，应保留撤销元数据并配置持久化。
- PostgreSQL 数据、配置和持久密钥的备份恢复；数据库迁移需在启动前手工执行。

## 验证

2026-10-30 本次整理后执行并通过；完整结果见 [回归报告](test/regression-report-2026-10-30.md)：

```bash
go test -race ./...
go vet ./...
go build ./...
python3 test/securityaudit/run.py
pnpm --dir auth-server-webui lint
pnpm --dir auth-server-webui build
go build -tags embedspa -o /tmp/auth-server-embed-check .
```

隔离安全回归通过 Go overlay 直接测试实际生产包，包含 50 项顶层安全测试，并同时运行普通 Go 测试；使用原样的 `go.mod/go.sum`，替换数据库驱动、Redis 命令及外部请求，不接触现有用户和令牌。覆盖客户端认证、PKCE、授权码重放、刷新轮换/重放/跨客户端保护、Scope、JWT、Telegram、CSRF/Origin、SID、退出、撤销并发、最后管理员保护及近期认证。详见 [test/securityaudit/README.md](test/securityaudit/README.md)。普通 `go test ./...` 不包含这些 `.go.fixture` 测试，必须单独运行脚本。

本次还在授权的测试 PostgreSQL/Redis 上运行了独立数据库和键命名空间的实时回归，验证约束、迁移、事务锁、刷新并发、真实 Lua/TTL 与撤销，并完成 Chromium 浏览器回归。运行方式和验证边界见 [test/README.md](test/README.md)。Turnstile 使用测试替身；真实第三方服务、HTTPS 反代、Redis Cluster、Docker 镜像构建及线上部署未验收。

[安全 CI](.github/workflows/security.yml) 已配置 Go race/vet、隔离回归、govulncheck，以及前端 lint/build/audit，以及独立 PostgreSQL/Redis 与浏览器回归；[镜像 CI](.github/workflows/docker-image.yml) 依赖安全任务通过后构建并推送镜像。工作流已存在不代表本次已运行远程 CI。

## 后续工作

以下保留原 Phase 编号，列出各阶段剩余工作；已实现项见项目现状与端点表。

- **Phase 1 / 管理 SPA 验收**：基础浏览器回归已覆盖 PKCE、取消授权、深链、403/404、窄屏和键盘交互；仍需跨浏览器及完整视觉验收。
- **Phase 2 / 授权与登录安全**：已有隔离及 PostgreSQL/Redis 实时回归；补真实 Turnstile、TLS/代理和网络故障验证。
- **Phase 3 / 管理功能**：用户编辑、启停、密码重置；客户端编辑、启停、secret 重置；列表分页/查询和错误反馈完善。多回调、Scope、grant_types 已支持创建时配置。
- **Phase 4 / 会话管理**：真实活跃会话索引、到期信息、按客户端/单会话撤销；将未来的用户禁用、客户端停用接入现有撤销机制。用户级强制下线、全局退出、刷新轮换及重放检测已实现。
- **Phase 5 / OIDC**：`openid`/`email`、ID Token、nonce、按 Scope 返回资料 claims；完善密钥轮换管理和客户端兼容性验证。Discovery、Bearer UserInfo、Introspection、RSA JWK 编码已有实现。
- **Phase 6 / 部署交付**：自动迁移管理、跨域静态托管/CORS、备份恢复操作说明，以及全新安装、保留数据升级、生产镜像和重启恢复验收。
