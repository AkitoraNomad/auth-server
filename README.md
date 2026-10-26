# Auth Server

Go OAuth2 授权服务器，使用 PostgreSQL、Redis，管理前端位于 [auth-server-webui/](auth-server-webui/)。

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

目标管理 SPA 使用无 secret 的公共客户端 + S256 PKCE；OIDC 的 ID Token/nonce 尚待实现。

## 配置示例

以下仅展示核心字段；启动还需配置 Turnstile，Telegram 登录需对应 bot 配置。

```yaml
port: 80
host: 0.0.0.0
redis:
  host: 0.0.0.0
  port: 6379
db: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=auth"
jwt:
  - kid: "rsa1"
    alg: "RS256"
    sec: |
      -----BEGIN RSA PRIVATE KEY-----
      ...
      -----END RSA PRIVATE KEY-----
```

## 管理 SPA：auth-server-webui

**现有工程符合简洁 React + Tailwind CSS 的技术方向，继续在此迭代，不另建 SPA。** 使用 TypeScript、Vite、React Router、pnpm 和 daisyUI；状态采用 Hooks/认证 Context，请求统一封装原生 `fetch`。可独立构建，但认证和管理仍依赖 OAuth2 端点及 `/api/admin/*`。

### 代码位置

以下路径均位于 [auth-server-webui/](auth-server-webui/)：

| 文件 | 职责 |
|---|---|
| [package.json](auth-server-webui/package.json)、[vite.config.ts](auth-server-webui/vite.config.ts) | 依赖、构建和开发代理，目前后端固定为 `http://localhost:8080` |
| [src/main.tsx](auth-server-webui/src/main.tsx)、[src/App.tsx](auth-server-webui/src/App.tsx) | React 入口、StrictMode、路由、登录回调与管理布局 |
| [src/index.css](auth-server-webui/src/index.css) | Tailwind CSS + daisyUI |
| [src/lib/auth.tsx](auth-server-webui/src/lib/auth.tsx)、[src/lib/api.ts](auth-server-webui/src/lib/api.ts) | 认证、Token 与 API 请求 |
| [src/types.ts](auth-server-webui/src/types.ts)、[src/routes/admin/](auth-server-webui/src/routes/admin/) | DTO 与管理页面 |

### 现况（2026-10-26 核查）

| 模块 | 已有实现 / 缺口 |
|---|---|
| 路由 | 当前为 `/login`、`/callback`、`/admin/*`；`/login` 与后端代理冲突，尚未迁移到目标 `/admin-page/*` |
| PKCE | 已生成 verifier/state、S256 challenge 并校验 state；仍手写 SHA-256，通过 `VITE_ADMIN_CLIENT_SECRET` 发送 Basic Auth |
| Token | 保存于 `localStorage`，存在字符串即视为已登录；无续期、回调去重和完整错误处理，退出仅清理本地状态 |
| 权限 | 已有 `auth_admin` 表和 Bearer Token 中间件，但管理客户端、特权 Scope、管理员身份检查未接通；旧管理接口仍未鉴权 |
| 管理页面 | 用户/客户端已有查询、新建、删除 UI，管理员已有查询、添加、移除 UI；编辑、启停、密码重置缺失，会话列表与撤销后端仍为占位 |
| API | 已有 `fetch`/DTO 封装；客户端 JSON 字段不匹配，缺统一 401/403、分页及完整错误反馈 |
| 交付 | 可生成独立 `dist/`；缺授权/API base URL 配置和生产托管，Docker 未打包 SPA，前端 README 仍是模板 |

验证：`pnpm build` 通过；`pnpm lint` 有 6 项错误（1 项 Fast Refresh、4 项 effect 内状态更新、1 项显式 any）。已有 Go 客户端认证/模型测试通过；浏览器登录、管理操作和生产部署尚未端到端验收。

### SPA 实现约定（目标，尚未全部实现）

- **保持简单**：沿用现有技术栈和 daisyUI，不引入重型后台框架、SSR 或复杂状态库。页面使用函数组件/Hooks，只按需提取基础组件；`lib/` 放认证/API，`routes/` 放页面，`types.ts` 放 DTO。
- **路由隔离**：统一 `/admin-page` 前缀、Vite base 和 Router basename；仅页面路由使用 SPA fallback，API 和缺失资源不得返回 HTML。未知页面显示 404，无权限显示 403。
- **公共客户端登录**：授权码 + S256 PKCE，精确注册开发/生产回调 URL，不携带 secret。密码、Telegram 登录及授权确认交给授权服务器；兑换时传 `client_id`、`code_verifier` 和原 `redirect_uri`。
- **授权请求保护**：用 Web Crypto 生成随机数和 challenge；state/verifier、时间和站内返回路径按请求暂存 `sessionStorage`，设置有效期并在成功/失败/取消后清理。校验 state、防重复兑换、清除地址栏 code/state，禁止外部返回 URL 和敏感认证日志。
- **Token 生命周期**：access/refresh token 默认仅存内存，不写 `localStorage`；页面刷新后经授权服务器会话重新授权。请求前完成认证初始化，统一 Bearer 注入、续期、401 重认证和 403 提示；权限最终由服务端校验。
- **公开配置**：提供 `.env.example`，使用 `VITE_ADMIN_CLIENT_ID`、`VITE_AUTH_BASE_URL`、`VITE_API_BASE_URL`；后两者默认当前 origin，回调为 SPA origin + `/admin-page/callback`。`VITE_*` 不得包含 secret。开发用 localhost、生产用 HTTPS；代理不覆盖 SPA 路径，跨域时明确允许来源、方法及 Authorization 头。
- **交互与数据**：简单导航、表格、表单和确认弹窗，支持窄屏、标签及键盘操作。提供加载/空数据/成功/失败状态，防重复提交，危险操作需确认；未实现功能标注不可用。使用 snake_case DTO，secret 仅创建/重置时单次展示，会话不暴露原始 Token。
- **独立交付**：保留 `pnpm dev/build/lint` 和独立 `dist/`，Go embed 仅作为可选打包方式；补齐前端 README 的配置、代理、客户端注册和部署说明。

目标路由与对应 API 使用相同权限映射；受保护页面还必须校验管理员身份和管理客户端：

| 路径（前缀 `/admin-page`） | 页面 / 权限 |
|---|---|
| `/login`、`/callback` | 登录入口、回调；回调须验证本次授权请求 |
| `/` | 导航首页；至少一个管理 Scope |
| `/users`、`/admins`、`/sessions` | 用户、管理员、会话；`manage:system` |
| `/clients` | 客户端；`manage:clients` |

## TODO

**先完成特权管理客户端迁移，再按阶段推进；Phase 1、2 均为上线前置条件。** 现有表、页面和 Token 框架不代表功能已验收。

### Phase 1：特权管理客户端迁移（优先）

- [ ] 统一 `internal_admin_console` 与 BIGINT 客户端 ID 的映射，保留现有数据；幂等初始化管理客户端及其回调、Scope、授权类型，前后端共享配置。
- [ ] 区分 public/confidential 客户端，取消 `IsPublic() = false` 的固定实现；按 SPA 约定完成无 secret 的 PKCE 登录。
- [ ] 提供受控的首位管理员初始化，为 `auth_admin.user_id` 添加唯一约束。
- [ ] 接通 `manage:system/manage:clients` 的解析、同意与签发，限制在客户端允许及用户同意范围内；管理 API 同时校验 Token、管理客户端、所需 Scope、有效用户和管理员记录，拒绝普通用户/客户端及无用户的客户端凭据 Token。
- [ ] 明确 JWT claims、验签和 Redis 撤销检查，确保撤销 Token 或移除管理员后访问失效；关闭未鉴权的 `/admin/client/*`、`/admin/accounts/*` 并清理旧模板调用。
- [ ] 完整保存 client、redirect URI、response type、state、Scope、PKCE 参数，按请求隔离 consent；在 `auth-server-webui/` 落实目标路由、回调和交互约定。
- [ ] **验收**：初始化、未登录进入后台、PKCE 成功/取消/state 错误/重复回调、错误客户端/非管理员/缺 Scope 拒绝、旧入口关闭、移除管理员、子路由刷新和 API 错误反馈。

### Phase 2：授权与登录安全

- [ ] 回调地址改为精确匹配，拒绝相似域名、协议降级和未注册路径。
- [ ] 刷新时校验客户端身份和 Token 归属；授权码、客户端凭据、刷新流程统一限制 Scope/授权类型，刷新不得扩大权限。
- [ ] Telegram 签名 ID 必须与登录账户绑定，检查 `auth_date` 有效期及异常输入。
- [ ] 授权确认增加请求绑定、CSRF 防护及拒绝流程，不直接信任 `consented`。
- [ ] 替换无盐 SHA-256 密码存储，提供现有凭据升级路径。
- [ ] **测试**：错误 PKCE、授权码重放、过期 Token、回调绕过、刷新凭据错误/跨客户端刷新、Scope 扩大、Telegram 身份错配。

### Phase 3：管理功能

- [ ] 修复客户端创建显式插入零值 ID，返回真实标识并校验输入；统一 JSON DTO，修复字段大小写与 snake_case 不一致，列表不返回 secret。
- [ ] 用户：编辑、启停、密码重置及关联 Token/Session 失效。
- [ ] 客户端：编辑、启停、多回调地址、Scope、grant_types；防止误删或破坏内置管理客户端。
- [ ] 管理员：重复添加、无效用户和最后一名管理员保护。
- [ ] 列表分页/查询、统一错误反馈；处理事务提交失败及登录标识唯一约束。
- [ ] **测试**：连续创建客户端、DTO 契约、状态变更、权限拒绝及管理员变更。

### Phase 4：会话与 Token

- [ ] 建立用户/客户端/Token/Session 索引，查询真实活跃会话和过期信息。
- [ ] 实现单会话/用户撤销和强制下线，清理 access token、refresh token 及必要的登录 Session；失败不得返回成功。
- [ ] 实现服务端退出，区分退出管理客户端与统一登录会话；前端按 `expires_in` 续期，协调并发刷新，失败后重新认证。
- [ ] 完善刷新轮换、旧 Token 失效及重放检测，与用户禁用、客户端停用和管理员变更联动。
- [ ] **测试**：撤销后访问/刷新、退出后访问/重新登录、到期和多会话下线。

### Phase 5：OAuth2 / OIDC 兼容

- [ ] Discovery 使用独立公网 issuer，适配 HTTPS/代理，仅声明已实现能力；明确是否保留 implicit 模式及其与 PKCE 的关系。
- [ ] 实现 openid/email Scope、ID Token 和 nonce 校验；UserInfo 支持 Bearer、GET/POST，按 Scope 返回 claims。
- [ ] Introspection 先设状态码再写响应；无效 Token 返回 `active:false`，正确处理认证失败，返回 Token 所属 client_id。
- [ ] 统一 JWT 配置与实际算法，修正 JWK 编码和密钥发布/轮换，区分 Access Token 与 ID Token claims。
- [ ] **测试**：Discovery、UserInfo、Introspection、JWT/JWKS 及 OIDC 客户端对接。

### Phase 6：部署与交付

- [ ] 支持独立静态托管和 Go embed 单二进制部署；Docker 增加前端构建，验证入口、回调和资源路径。
- [ ] 提供增量数据库迁移，修复 PostgreSQL 外键删表顺序，明确遗留 MySQL 支持范围。
- [ ] 补齐脱敏配置、首次初始化、PostgreSQL/Redis/Turnstile 依赖、HTTPS/代理、密钥管理、备份恢复和升级说明。
- [ ] CI 接入 Go 测试、前端 build/lint，修复现有 lint 错误；增加隔离数据库/Redis 集成测试和浏览器登录测试。
- [ ] **验收**：全新安装、保留数据升级、重启后登录/Token、生产镜像及独立静态部署中的管理与撤销流程。
