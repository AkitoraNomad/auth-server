# Auth Server

Go OAuth2 授权服务器，使用 PostgreSQL、Redis，管理前端位于 [auth-server-webui/](auth-server-webui/)。

## 安全修复与升级（2026-10-27）

严格复审的修复、回归结果和未验证边界见 [修复复测报告](docs/security-remediation-2026-10-27.md)。本次升级会拒绝没有安全元数据的旧授权码、Access/Refresh Token 和 SSO Session，需要重新登录。

- 现有数据库需应用 [002_identity_constraints.sql](db/migrations/002_identity_constraints.sql)；先处理重复邮箱、重复外部身份，迁移遇到重复记录会回滚。不要对已有数据运行会删表的 `db/postgres.sql`。
- 退出管理端会撤销该用户所有现有授权和 SSO 会话。管理员写操作要求最近 15 分钟内通过凭据登录；普通刷新不会延长这个窗口。
- 生产只允许 HTTPS 回调；本机 HTTP 例外限 loopback。Introspection 仅允许机密客户端查询自身令牌；UserInfo 支持带 `profile` scope 的 Bearer Token。
- `issuer` 必须是浏览器使用的公网 origin。Vite 代理开发时设置 `OAUTH_ISSUER=http://localhost:5173`；Go 独立托管本机页面时用 `http://localhost:8080`。
- 反代部署通过 `trusted-proxies` 明确列出实际代理 IP/CIDR，登录限速才会使用这些代理提供的 `X-Forwarded-For`。默认忽略转发头，不能配置任意来源为可信代理。
- Docker 运行用户为 UID/GID `10001:10001`；挂载配置、密钥应对该用户可读。Redis 可配置 `username`、`password` 和 `tls: true`，需保留令牌和撤销元数据。

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

管理 SPA 使用无 secret 的公共客户端 + S256 PKCE。管理客户端固定为 BIGINT `client_id=2`（默认值，可用 `ADMIN_CLIENT_ID` 修改），由服务端启动时幂等接管。OIDC 的 ID Token/nonce 尚待 Phase 5 实现。

## 配置示例

以下仅展示核心字段；完整字段见 [config-schema.json](config-schema.json)。启动还需要 Turnstile，Telegram 登录需对应 bot 配置。

```yaml
host: 127.0.0.1
port: 8080
issuer: http://localhost:8080
redis:
  host: 127.0.0.1
  port: "6379"
db: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=auth"
jwt:
  - kid: "rsa1"
    alg: "RS256"
    sec: |
      -----BEGIN RSA PRIVATE KEY-----
      ...
      -----END RSA PRIVATE KEY-----
admin:
  client-id: 2
  redirect-uris:
    - "http://localhost:5173/admin-page/callback"
    - "http://127.0.0.1:5173/admin-page/callback"
    - "http://localhost:8080/admin-page/callback"
    - "http://127.0.0.1:8080/admin-page/callback"
  scopes:
    - "manage:system"
    - "manage:clients"
  grant-types:
    - "authorization_code"
    - "refresh_token"
```

对应的运行时覆盖变量见根目录 [.env.example](.env.example)（可直接 export，或放在 VS Code `launch.json` 的 `envFile` 中）：

```bash
ADMIN_CLIENT_ID=2
ADMIN_CLIENT_REDIRECT_URIS=http://localhost:5173/admin-page/callback
ADMIN_CLIENT_SCOPES=manage:system,manage:clients
ADMIN_CLIENT_GRANT_TYPES=authorization_code,refresh_token
OAUTH_ISSUER=http://localhost:8080
```

`issuer` 必须与实际公网访问地址一致；反向代理/HTTPS 部署时不要依赖默认值。

## 管理 SPA：auth-server-webui

**现有工程符合简洁 React + Tailwind CSS 的技术方向，继续在此迭代，不另建 SPA。** 使用 TypeScript、Vite、React Router、pnpm 和 daisyUI；状态采用 Hooks/认证 Context，请求统一封装原生 `fetch`。可独立构建，但认证和管理仍依赖 OAuth2 端点及 `/api/admin/*`。

### 代码位置

以下路径均位于 [auth-server-webui/](auth-server-webui/)：

| 文件 | 职责 |
|---|---|
| [package.json](auth-server-webui/package.json)、[vite.config.ts](auth-server-webui/vite.config.ts) | 依赖、构建和开发代理；开发端口固定 5173，后端默认 `http://localhost:8080` |
| [src/main.tsx](auth-server-webui/src/main.tsx)、[src/App.tsx](auth-server-webui/src/App.tsx) | React 入口、StrictMode、`/admin-page` basename、路由、回调与权限门 |
| [src/index.css](auth-server-webui/src/index.css) | Tailwind CSS + daisyUI |
| [src/lib/auth.tsx](auth-server-webui/src/lib/auth.tsx)、[src/lib/auth-context.ts](auth-server-webui/src/lib/auth-context.ts)、[src/lib/api.ts](auth-server-webui/src/lib/api.ts) | PKCE 登录、内存 Token、刷新、401 重认证、API 请求 |
| [src/lib/config.ts](auth-server-webui/src/lib/config.ts) | `VITE_*` 公共配置、API/Auth base URL、回调 URL、安全返回路径 |
| [src/types.ts](auth-server-webui/src/types.ts)、[src/routes/admin/](auth-server-webui/src/routes/admin/) | snake_case DTO 与管理页面 |

### 现况（Phase 1 实现后）

| 模块 | 状态 |
|---|---|
| 路由 | 已迁移到 `/admin-page/*`，BrowserRouter basename 与 Vite base 一致；未知页面 404，缺 Scope 403 |
| PKCE | Web Crypto 生成 verifier/state/challenge；token 兑换只发送 `client_id`、`code_verifier`、`redirect_uri`，不再携带 secret |
| Token | 仅存内存，不写 localStorage；支持续期、401 单次重试、刷新页面后重新走授权；退出仅清理本地状态（服务端退出见 Phase 4） |
| 权限 | 管理 API 校验 JWT 签名/claims、Redis 撤销、管理客户端、真实用户和 `auth_admin`；`manage:system`/`manage:clients` 按路由校验；旧 `/admin/*` 已删除 |
| 管理页面 | 用户/客户端/管理员已有查询、新建、删除；编辑、启停、密码重置仍为 Phase 3；会话页明确标注 Phase 4，不再返回假成功 |
| API | 客户端 DTO 已改为 snake_case；客户端 secret 列表不返回，创建时单次展示；统一处理 401/403 与 API 错误 |
| 交付 | 可生成独立 `dist/`；`-tags embedspa` 将 SPA 嵌入 Go 单二进制；Docker 多阶段构建已包含前端；独立静态托管/CORS 仍为可选部署方式 |

验证：`go build ./...`、`go vet ./...`、`go test ./...` 通过；`pnpm build`、`pnpm lint` 通过。已在测试 PostgreSQL/Redis 上使用 Turnstile 测试密钥完成 160 项 HTTP/代理回归，0 项失败；覆盖 OAuth/PKCE 成功流、token/refresh 轮换、JWT 管理 API、普通用户/公共客户端、非管理员/缺 Scope/客户端凭据拒绝、授权码重放、错误 PKCE、错误 state/redirect、旧路由 404、管理员移除即时失效、Redis 撤销、会话/consent 数组清理、服务重启幂等和 Vite 路由/代理。3 项已知问题：stale 登录页可切换 session 用户、跨客户端 refresh 未被拒绝、最后一名管理员可被移除；UserInfo Bearer/ID Token 属 Phase 5。真实浏览器自动化/视觉验收仍待补。

### SPA 实现约定（Phase 1 已落实）

- **保持简单**：沿用现有技术栈和 daisyUI，不引入重型后台框架、SSR 或复杂状态库。页面使用函数组件/Hooks；`lib/` 放认证/API，`routes/` 放页面，`types.ts` 放 DTO。
- **路由隔离**：统一 `/admin-page` 前缀、Vite base 和 Router basename；仅页面路由使用 SPA fallback，API 和缺失资源不得返回 HTML。未知页面显示 404，无权限显示 403。
- **公共客户端登录**：授权码 + S256 PKCE，精确注册开发/生产回调 URL，不携带 secret。密码、Telegram 登录及授权确认交给授权服务器；兑换时传 `client_id`、`code_verifier` 和原 `redirect_uri`。
- **授权请求保护**：用 Web Crypto 生成随机数和 challenge；state/verifier、时间和站内返回路径按请求暂存 `sessionStorage`（10 分钟 TTL），成功/失败/取消后清理。校验 state、防重复兑换、清除地址栏 code/state，禁止外部返回 URL 和敏感认证日志。
- **Token 生命周期**：access/refresh token 仅存内存，不写 `localStorage`；页面刷新后经授权服务器会话重新授权。请求前完成认证初始化，统一 Bearer 注入、续期、401 重认证和 403 提示；权限最终由服务端校验。
- **公开配置**：提供 [.env.example](auth-server-webui/.env.example)，使用 `VITE_ADMIN_CLIENT_ID`、`VITE_AUTH_BASE_URL`、`VITE_API_BASE_URL`；后两者默认当前 origin，回调默认为 SPA origin + `/admin-page/callback`。`VITE_*` 不得包含 secret。开发用 localhost、生产用 HTTPS；代理不覆盖 SPA 路径，跨域时明确允许来源、方法及 Authorization 头。
- **交互与数据**：简单导航、表格、表单和确认弹窗，支持窄屏、标签及键盘操作。提供加载/空数据/成功/失败状态，防重复提交，危险操作需确认；未实现功能标注不可用。使用 snake_case DTO，secret 仅创建/重置时单次展示，会话不暴露原始 Token。
- **独立交付**：保留 `pnpm dev/build/lint` 和独立 `dist/`；生产可用 `go build -tags embedspa` 把 `dist/` 嵌入单二进制，不需要单独启动静态服务器。

目标路由与对应 API 使用相同权限映射；受保护页面还必须校验管理员身份和管理客户端：

| 路径（前缀 `/admin-page`） | 页面 / 权限 |
|---|---|
| `/login`、`/callback` | 登录入口、回调；回调须验证本次授权请求 |
| `/` | 导航首页；至少一个管理 Scope |
| `/users`、`/admins`、`/sessions` | 用户、管理员、会话；`manage:system` |
| `/clients` | 客户端；`manage:clients` |

## Phase 1 本地验收

### 1. 数据库

全新数据库：

```bash
psql "$DB" -f db/postgres.sql
```

已有数据库（保留数据）：

```bash
psql "$DB" -f db/migrations/001_phase1_admin_spa.sql
psql "$DB" -f db/migrations/002_identity_constraints.sql
```

迁移会为 `auth_client` 增加 `is_public`、`redirect_uris`、`grant_types`，并为 `auth_admin.user_id` 增加唯一约束。`002_identity_constraints.sql` 还会为邮箱（大小写不敏感）和 `(provider_type, login_key)` 增加唯一索引，并移除隐式 implicit grant。若已有重复邮箱、重复外部身份或重复管理员记录，迁移会失败，需要先备份并人工去重。

### 2. 启动后端

需要 PostgreSQL、Redis、Turnstile 配置。服务启动时会幂等创建/更新 `client_id=2` 的管理客户端，**该 ID 必须没有被其他应用占用**：

```bash
export ADMIN_CLIENT_ID=2
export ADMIN_CLIENT_REDIRECT_URIS=http://localhost:5173/admin-page/callback,http://127.0.0.1:5173/admin-page/callback,http://localhost:8080/admin-page/callback,http://127.0.0.1:8080/admin-page/callback
export OAUTH_ISSUER=http://localhost:5173 # Vite 代理开发；Go 独立托管时改为 8080
go run .
```

### 3. 手工创建首位管理员

Phase 1 按约定不自动创建管理员。使用 [db/seed_admin.example.sql](db/seed_admin.example.sql)：

```bash
read -r -s PASSWORD
HASH=$(printf '%s' "$PASSWORD" | go run ./cmd/hash-password)
psql "$DB" \
  -v admin_email='admin@example.com' \
  -v admin_display_name='Admin' \
  -v admin_password_hash="$HASH" \
  -f db/seed_admin.example.sql
```

脚本会创建 `auth_user`、`auth_user_provider`（EmailPassword）和 `auth_admin` 记录。密码使用带随机 salt 的 Argon2id PHC 哈希；`cmd/hash-password` 从标准输入读取密码并输出哈希，不会把密码写入 shell 参数或数据库明文。 由于项目尚未投产，本次不做 SHA-256 兼容；已有 SHA-256 密码记录的账号需要重置密码。

### 4. 启动 SPA

```bash
cd auth-server-webui
cp .env.example .env
pnpm install
pnpm dev
```

打开 `http://localhost:5173/admin-page/`。未登录时 SPA 会自动跳转 `/oauth2/authorize`，完成密码登录和 Scope 同意后回到 `/admin-page/callback`。开发代理会把 `/oauth2`、`/login`、`/auth`、`/api` 转发到 `http://localhost:8080`。

Token 仅存内存；刷新页面会重新经过授权服务器。由于 Phase 1 不持久化 consent，刷新后需要重新确认 Scope（这是当前约定）。

### 5. 单二进制生产构建（嵌入 SPA）

生产环境不需要单独启动静态服务器。先构建前端，再用 `embedspa` 标签构建 Go：

```bash
pnpm --dir auth-server-webui build
go build -tags embedspa -o auth-server .
./auth-server
```

打开 `http://localhost:8080/admin-page/`。此时 `/admin-page/*` 的 SPA shell、深链和带哈希的静态资源都由 Go 二进制提供。

- 默认 `go build ./...` 仍可正常编译，不嵌入 SPA；Vite dev 继续使用独立的 `5173` 服务。
- `-tags embedspa` 会嵌入 `auth-server-webui/dist`；如果未先构建前端，Go 构建会直接失败，避免发布空壳 SPA。
- Dockerfile 已改为 Node 构建 + Go 嵌入的多阶段构建。
- 生产域名和回调需通过 `ADMIN_CLIENT_REDIRECT_URIS` / `VITE_ADMIN_REDIRECT_URI` 配置；默认已包含本机 `8080` 回调。
- 资源缓存：`/admin-page/assets/*` 使用一年 immutable，`index.html` 与 SPA 深链使用 `no-store`，缺失的 `.js/.css` 返回 404，不会错误回退成 HTML。

### 6. 建议验收项

- 未登录进入 `/admin-page/users`，完成 PKCE 登录后回到原路径。
- 只勾选一个管理 Scope，确认另一个页面的导航隐藏、直接访问返回 403。
- 错误 state、重复 callback、取消授权、非管理员登录应失败且不留下可用 Token。
- 移除 `auth_admin` 记录后，原 Access Token 的下一次管理请求应返回 403。
- 旧 `/admin/client/*`、`/admin/accounts/*` 应返回 404。
- `GET /api/admin/clients` 不得包含 secret。

## TODO

**先行目标：Phase 1 特权管理客户端迁移已完成代码实现，并已在测试 PostgreSQL/Redis/Turnstile 环境完成后端与代理链路验收；真实浏览器自动化验收仍待补，Phase 2+ 未完成。**

### Phase 1：特权管理客户端迁移（优先）

- [x] 固定 BIGINT 管理客户端 ID=2 的幂等 seed，接管并更新其回调、Scope、授权类型；前后端通过同名环境变量/default 对齐。
- [x] 区分 public/confidential 客户端，取消 `IsPublic() = false` 的固定实现；SPA 使用无 secret 的 PKCE 登录。
- [x] 提供手工首位管理员 SQL，为 `auth_admin.user_id` 添加唯一约束。
- [x] 接通 `manage:system/manage:clients` 的解析、同意与签发；管理 API 校验 JWT、管理客户端、Scope、真实用户和 `auth_admin`，拒绝客户端凭据 Token。
- [x] 自定义 JWT claims（`sub/client_id/scope/jti/iss/aud/iat/exp`），验签后检查 Redis 撤销；移除管理员后即时失效；关闭旧 `/admin/*` 入口。
- [x] 完整保存 client、redirect URI、response type、state、Scope、PKCE 参数，按请求隔离 consent；SPA 已迁移到 `/admin-page`。
- [x] **后端与代理端到端验收**：管理客户端初始化、PKCE 成功、token/refresh、state 错误、授权码重放、错误 PKCE、非管理员/缺 Scope/客户端凭据拒绝、旧入口 404、移除管理员即时失效、Redis 撤销、Vite `/admin-page` 深链与代理均已验证。
- [ ] **浏览器 UI 自动化验收**：真实浏览器完成取消授权、子路由刷新、403/404 展示、窄屏/键盘交互及完整视觉回归。

### Phase 2：授权与登录安全

- [x] 刷新时校验客户端身份和 Token 归属；授权码、客户端凭据、刷新流程统一限制 Scope/授权类型。
- [x] `POST /login` 已登录守卫：session 已有用户时拒绝/复用登录，禁止 stale 登录页切换账号。
- [x] Telegram 签名 ID 必须与登录账户绑定，检查 `auth_date` 有效期及异常输入。
- [x] 授权确认增加请求绑定、CSRF 防护及拒绝流程，不直接信任 `consented`。
- [x] 替换无盐 SHA-256 密码存储：新密码使用 Argon2id（随机 salt、PHC 编码）；由于项目未投产，不做旧哈希兼容。
- [ ] **测试**：错误 PKCE、授权码重放、过期 Token、回调绕过、刷新凭据错误/跨客户端刷新、Scope 扩大、Telegram 身份错配。

> Phase 1 已对管理客户端和新客户端执行精确 redirect URI 匹配；相似域名/未注册路径会在授权入口拒绝。

### Phase 3：管理功能

- [x] 修复客户端创建显式插入零值 ID，返回真实标识；统一 JSON DTO，修复字段大小写与 snake_case 不一致，列表不返回 secret。
- [ ] 用户：编辑、启停、密码重置及关联 Token/Session 失效。
- [ ] 客户端：编辑、启停、多回调地址、Scope、grant_types；防止误删或破坏内置管理客户端（当前仅禁止删除 ID=2）。
- [x] 管理员：唯一约束及外键校验；删除管理员和删除用户在事务锁内保护最后一名管理员。
- [ ] 列表分页/查询、统一错误反馈；处理事务提交失败及登录标识唯一约束。
- [ ] **测试**：连续创建客户端、DTO 契约、状态变更、权限拒绝及管理员变更。

### Phase 4：会话与 Token

- [ ] 建立用户/客户端/Token/Session 索引，查询真实活跃会话和过期信息。
- [x] 实现用户级强制下线：通过安全版本号使现有 Token、授权码及 SSO Session 失效；单会话管理仍待实现。
- [x] 实现服务端全局退出及管理端接入；前端协调刷新与退出。按客户端或单会话退出尚未实现。
- [ ] 完善刷新轮换、旧 Token 失效及重放检测，与用户禁用、客户端停用和管理员变更联动。
- [ ] **测试**：撤销后访问/刷新、退出后访问/重新登录、到期和多会话下线。

### Phase 5：OAuth2 / OIDC 兼容

- [ ] Discovery 使用独立公网 issuer，适配 HTTPS/代理，仅声明已实现能力；明确是否保留 implicit 模式及其与 PKCE 的关系。
- [ ] 实现 openid/email Scope、ID Token 和 nonce 校验；UserInfo 支持 Bearer、GET/POST，按 Scope 返回 claims。
- [ ] Introspection 先设状态码再写响应；无效 Token 返回 `active:false`，正确处理认证失败，返回 Token 所属 client_id。
- [ ] 统一 JWT 配置与实际算法，修正 JWK 编码和密钥发布/轮换，区分 Access Token 与 ID Token claims。
- [ ] **测试**：Discovery、UserInfo、Introspection、JWT/JWKS 及 OIDC 客户端对接。

### Phase 6：部署与交付

- [x] 支持 Go embed 单二进制部署；Dockerfile 增加前端构建；嵌入二进制的 SPA 路由、深链、资源和回调路径已验证（Docker 镜像构建待有 Docker 的环境验证）。
- [ ] 支持独立静态托管与 CORS 配置，供非 Go 托管场景使用。
- [ ] 提供增量数据库迁移管理，修复 PostgreSQL 外键删表顺序（基础 schema 已修正），明确遗留 MySQL 支持范围。
- [ ] 补齐脱敏配置、首次初始化、PostgreSQL/Redis/Turnstile 依赖、HTTPS/代理、密钥管理、备份恢复和升级说明。
- [ ] CI 接入 Go 测试、前端 build/lint，修复现有 lint 错误；增加隔离数据库/Redis 集成测试和浏览器登录测试。
- [ ] **验收**：全新安装、保留数据升级、重启后登录/Token、生产镜像及独立静态部署中的管理与撤销流程。
