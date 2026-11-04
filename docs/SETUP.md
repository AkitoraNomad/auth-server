# 配置与运行

## 环境

- Go 1.26+、PostgreSQL、Redis、`psql`。
- Node.js 22.12+、pnpm 11.3.0。
- Turnstile site key / secret；Telegram 登录另需 bot 配置。

## 配置

在项目根目录创建 `config.yaml`。以下配置用于 Vite 同源代理开发：

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
jwt: []
admin:
  redirect-uris:
    - http://localhost:5173/admin-page/callback
    - http://localhost:8080/admin-page/callback
  scopes: [manage:system, manage:clients]
  grant-types: [authorization_code, refresh_token]
```

设置 `TURNSTILE_SITE_KEY`、`TURNSTILE_SECRET_KEY` 后启动。可选 Telegram 配置为 `telegram.bot-name` 和 `telegram.bot-token`。

服务读取当前工作目录的 `config.yaml`，不会自动加载 `.env`。[根目录环境变量示例](../.env.example) 可用于逐项 export、VS Code `envFile` 或 Docker `--env-file`。字符串及数组元素支持 `${VAR}` / `${VAR:default}`；占位符替换整个字段，不支持字符串拼接。整数和布尔值直接写在 YAML 中。

`jwt: []` 仅适合临时开发，会在每次启动时生成新 RSA 密钥，使旧 JWT 和签名会话 Cookie 失效。稳定部署使用持久密钥：

```yaml
jwt:
  - kid: rsa1
    alg: RS256
    sec: ${JWT_PRIVATE_KEY}
```

`JWT_PRIVATE_KEY` 为完整 PKCS#1 PEM 私钥内容（`BEGIN RSA PRIVATE KEY`），不是文件路径。JWT Access Token 使用首个 RSA 密钥签名；多实例需保持密钥配置一致。

| 直接覆盖 YAML 的环境变量 | 作用 |
|---|---|
| `OAUTH_ISSUER` | 公网 origin，必须与浏览器访问的协议、主机和端口一致，无路径前缀 |
| `ADMIN_CLIENT_DISPLAY_NAME` | 管理客户端显示名称 |
| `ADMIN_CLIENT_REDIRECT_URIS` | 逗号分隔的精确回调 URL |
| `ADMIN_CLIENT_SCOPES` | 必须包含 `manage:system` 和 `manage:clients` |
| `ADMIN_CLIENT_GRANT_TYPES` | 必须包含 `authorization_code`，另允许 `refresh_token` |

字段以 [internal/config.go](../internal/config.go) 为准，[config-schema.json](../config-schema.json) 用于编辑参考，启动时不执行 JSON Schema 校验。

## 数据库与首位管理员

数据库结构见 [db/postgres.sql](../db/postgres.sql)，使用 `auth` schema。仅在空数据库运行初始化脚本，**该脚本包含 DROP TABLE，会删除已有认证数据**：

```bash
export DB='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=auth'
psql "$DB" -v ON_ERROR_STOP=1 -f db/postgres.sql
```

服务按唯一 `internal_key=auth-server-admin-spa` 注册管理客户端，ID 由数据库分配；普通客户端的 `internal_key` 为 NULL。首位管理员需使用 [SQL seed](../db/seed_admin.example.sql) 创建：

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

使用至少 12 个字符的密码。哈希工具从标准输入读取密码，输出 Argon2id PHC 哈希；SQL seed 不会改写已有 EmailPassword 密码。自助注册仅支持 Telegram，邮箱密码账号由管理员或 SQL seed 创建。当前运行代码使用 PostgreSQL，`db/mysql.sql` 为遗留脚本。

## 本地运行

从项目根目录启动后端：

```bash
OAUTH_ISSUER=http://localhost:5173 go run .
```

另开终端启动 SPA：

```bash
pnpm --dir auth-server-webui install --frozen-lockfile
pnpm --dir auth-server-webui dev
```

访问 `http://localhost:5173/admin-page/`。使用 `127.0.0.1` 或其他端口时，同步修改 issuer 和注册回调。Vite 代理目标默认 `http://localhost:8080`，可通过 `BACKEND_URL` 覆盖。前端配置与代理路径见 [SPA 文档](../auth-server-webui/README.md)。

## 部署

### Go 托管 SPA

```bash
pnpm --dir auth-server-webui install --frozen-lockfile
pnpm --dir auth-server-webui build
go build -tags embedspa -o auth-server .
OAUTH_ISSUER=http://localhost:8080 ./auth-server
```

`embedspa` 嵌入 `auth-server-webui/dist/`，构建前须先生成前端产物；普通 Go 构建不包含 SPA。运行目录仍需 `config.yaml`、`template/` 和 `assets/bulma.min.css`。根路径图片使用 `placeholder.jpg`，缺失时返回文字占位。

管理静态资源缓存一年并标记 immutable；首页与深链使用 `no-store`，缺失 JS/CSS 返回 404。独立静态产物可由同源反向代理提供；后端当前不提供跨域 CORS 配置。

### Docker

[Dockerfile](../Dockerfile) 自动构建前端与 Go 服务，以 UID/GID `10001:10001` 运行。配置需挂载至 `/app/config.yaml`，并对该用户可读：

```bash
docker build -t auth-server:local .
docker run --rm -p 127.0.0.1:8080:8080 \
  --env-file .env \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  auth-server:local
```

容器配置的 `host` 使用 `0.0.0.0`；PostgreSQL/Redis 必须是容器可访问的服务地址。`.env` 需包含 YAML 引用的全部变量，密钥不要写入镜像或前端 `VITE_*` 配置。

### 运行约束

- Go 通过 HTTP 监听，HTTPS 由反向代理终止；issuer 与精确回调 URL 使用实际公网地址。
- `trusted-proxies` 仅填写实际代理 IP/CIDR；默认不信任转发头。
- Redis 支持 `username`、`password` 和 `tls: true`，保存令牌、会话及撤销元数据，需配置持久化。
- 备份 PostgreSQL 数据、配置和签名密钥；恢复时保持相关安全状态一致。
- 管理 SPA 的 Access/Refresh Token 只存内存；页面刷新后重新授权，授权确认不持久化。
- 管理写操作要求最近 15 分钟内通过凭据登录；超时后退出并重新登录，普通刷新不延长窗口。
