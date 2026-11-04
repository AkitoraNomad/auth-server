# API 概览

## OAuth2 与身份端点

| 方法与路径 | 行为 |
|---|---|
| `GET/POST /oauth2/authorize` | 授权码请求与 POST 授权确认/拒绝；仅支持 code + S256 PKCE |
| `POST /oauth2/token` | 授权码兑换、刷新、机密客户端凭据；按客户端限制 Scope 和 grant |
| `GET /.well-known/openid-configuration` | Discovery；尚无完整 OIDC / ID Token 支持 |
| `GET /.well-known/jwks.json` | 发布配置密钥的 JWK |
| `POST /introspect` | HTTP Basic，仅机密客户端可查询自身 Access Token；无效、撤销或不属自身时返回 `active:false` |
| `GET/POST /userinfo` | Bearer 用户 Token，需要 `profile`，当前仅返回 `sub`；另支持 Basic + POST 表单 `token` 的同客户端查询 |
| `GET /logout` | 返回当前会话的 CSRF Token |
| `POST /logout` | Bearer 或 Cookie + `X-CSRF-Token`；撤销用户全部现有授权及 SSO，成功返回 204 |
| `GET /api/public/admin-config` | 无需登录，仅返回实际管理客户端 `client_id` |

回调 URL 必须精确匹配，HTTP 仅允许 loopback。公共客户端不能使用 `client_credentials`；机密客户端在兑换和刷新时均需认证。

## 管理 API

管理页面统一位于 `/admin-page/`。以下 API 要求有效的管理客户端用户 Token、管理员身份；写操作另要求最近 15 分钟内通过凭据认证。

| 资源 | 路径与方法 | Scope |
|---|---|---|
| 当前身份 | `GET /api/admin/me` | 不额外要求管理 Scope |
| 用户 | `GET/POST /api/admin/users`、`PATCH/DELETE /api/admin/users/:id`、`POST /api/admin/users/:id/password` | `manage:system` |
| 客户端 | `GET/POST /api/admin/clients`、`PUT/DELETE /api/admin/clients/:id`、`PATCH /api/admin/clients/:id/state`、`POST /api/admin/clients/:id/secret` | `manage:clients` |
| 管理员 | `GET/POST /api/admin/admins`、`DELETE /api/admin/admins/:user_id` | `manage:system` |
| 会话 | `GET /api/admin/sessions`、`DELETE /api/admin/sessions/:user_id` | `manage:system` |

会话列表返回用户记录，不代表真实活跃会话；删除会话接口撤销该用户全部现有授权及 SSO。

### 列表与错误

四个列表支持 `page`（默认 1，范围 1–1000000）、`page_size`（默认 20，范围 1–100）、`q`（去除首尾空白后最多 200 字符）、`status=all|active|disabled`。查询为名称/邮箱/ID 的大小写不敏感字面子串匹配；状态表示启停，不表示在线。超过末页的有效页码自动回退，空列表返回第 1 页。

```json
{"code":0,"data":{"items":[],"total":0,"page":1,"page_size":20}}
```

DTO 使用 snake_case。参数错误返回 400、记录不存在返回 404、唯一约束或最后管理员冲突返回 409；内部数据库错误不返回 SQL 细节。客户端的 `scopes`、`redirect_uris` 和 `grant_types` 空集合返回 `[]`。

### 用户与客户端操作

- 用户 PATCH 支持 `email`、`display_name`、`is_disabled`。邮箱变更同步已有 EmailPassword 登录标识；邮箱/状态变更及密码重置递增安全版本，使旧授权码、Token 和 SSO 失效。仅改显示名称不会强制下线。
- 密码重置 body 为 `{"password":"新的密码"}`，至少 12 字符、最多 1024 字节；只支持已有 EmailPassword 登录方式的账号。
- 客户端 PUT 更新名称、回调、Scope、grant_types 和 Token 类型，需携带原 `is_public`。类型创建后固定；PUT 不修改启停状态，非空 `secret` 会被拒绝。
- 启停使用 PATCH body `{"is_disabled":true}`；secret 重置仅支持机密客户端，返回一次展示的 `{"id":3,"secret":"..."}`。列表不返回 secret。
- 保存客户端配置、改变启停状态或重置 secret 会使旧授权失效，包括 client_credentials Token；重新启用不恢复旧授权，不结束用户在其他客户端的全局 SSO。
- `internal_key` 由服务端维护，管理 API 不允许设置或修改；内置客户端禁止编辑、启停、secret 重置或删除。
- 删除或停用最后一名可用管理员会被事务锁保护，停用管理员不计入可用人数。

## 令牌与权限

Scope 为 `profile`、`avatar`、`manage:system`、`manage:clients`；`avatar` 已定义，UserInfo 尚不返回头像。授权码 Access Token 默认有效期 2 小时，Refresh Token 默认 3 天，刷新不能扩大原 Scope。

管理 Scope 除客户端白名单外，还要求内置管理客户端和有效管理员；授权码签发、兑换及刷新均校验，省略刷新 Scope 时也检查继承权限。管理 API 每次复核管理员资格与撤销状态。

移除管理员资格会立即阻止管理请求及管理权限续签，但不自动结束普通业务授权和 SSO；需要全局下线时使用用户会话撤销接口。

撤销与安全版本检查由服务端 Token 加载链路执行。外部资源服务仅离线验签时不会自动感知撤销；当前 JWT audience 为客户端 ID，Introspection 限机密客户端查询自身 Token，尚无通用资源服务授权模型。

实际路由和请求处理见 [gin_router.go](../internal/http/gin_router.go)、[管理 API](../internal/http/admin/api.go) 和 [管理操作](../internal/http/admin/management.go)。
