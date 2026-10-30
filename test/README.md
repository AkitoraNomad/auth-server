# 回归测试

## 离线检查与安全断言

从项目根目录运行，依赖需提前下载：

```bash
go test -race -count=1 ./...
go vet ./...
go build ./...
python3 test/securityaudit/run.py
pnpm --dir auth-server-webui lint
pnpm --dir auth-server-webui build
go build -tags embedspa -o /tmp/auth-server-embed-check .
```

安全脚本通过临时 Go overlay 将 `.go.fixture` 加入实际生产包，不复制或重写生产源码，不加载真实配置，也不访问外部服务。它同时运行普通 Go 测试和 50 项顶层安全探针，详见 [securityaudit/README.md](securityaudit/README.md)。

## PostgreSQL、Redis 与浏览器

实时集成测试位于 [integration_test.go](../internal/http/integration_test.go)，通过 `integration` build tag 显式启用。需要一个允许 `CREATE DATABASE` 的测试 PostgreSQL 账号，以及可访问的 Redis；连接信息通过环境变量传入，不读取项目的 `config.yaml` / `.env`：

```bash
export AUTH_TEST_POSTGRES='postgres://USER:PASSWORD@HOST:5432/postgres?sslmode=disable'
export AUTH_TEST_REDIS='HOST:6379'
go test -tags integration -race -count=1 -v ./internal/http -run TestLive
```

每次测试创建随机命名的独立 PostgreSQL 数据库，以及带相同随机前缀的 Redis 键。Redis 使用命令 hook 给键添加前缀，命令、事务和 Lua 仍在真实服务器执行。结束后删除该数据库及该前缀下的键，不执行 `FLUSHDB`，不修改已有数据库。测试失败也会执行清理；进程被强制终止时可能留下以 `auth_regression_` 开头的测试数据，需要确认所属运行后清理。

浏览器回归需要 Python Playwright 和 Chromium：

```bash
python3 -m venv /tmp/auth-server-browser-env
/tmp/auth-server-browser-env/bin/pip install -r test/browser/requirements.txt
/tmp/auth-server-browser-env/bin/python -m playwright install chromium
export AUTH_TEST_BROWSER_PYTHON=/tmp/auth-server-browser-env/bin/python
# 可选：使用已安装的 Chromium；未指定时优先使用系统 chromium，否则使用 Playwright 下载的版本。
# export AUTH_TEST_CHROMIUM=/usr/bin/chromium
go test -tags integration -race -count=1 -v ./internal/http -run TestLive
```

先构建前端。测试启动临时本地 HTTP 服务，使用生产路由、模板和构建后的 SPA。浏览器脚本见 [browser/regression.py](browser/regression.py)。测试账号只存在于临时数据库。Turnstile widget 和 Siteverify 使用测试替身，Telegram/外部字体请求被阻断；这不验证真实第三方服务、HTTPS 反代或 Redis Cluster。

覆盖 PostgreSQL 数组/约束和增量迁移、管理权限、刷新轮换与重放、并发刷新、真实 Redis Lua/TTL、用户级撤销和退出、最后管理员事务锁，以及浏览器 PKCE 登录、401 刷新重试、退出失败恢复、取消授权、列表、403/404、深链刷新和窄屏/键盘交互。未设置浏览器 Python 时只跳过浏览器子测试；未设置数据库/Redis 时跳过实时测试。将跳过视作通过是不正确的。

[安全 CI](../.github/workflows/security.yml) 的 integration job 使用独立 PostgreSQL/Redis services，并安装浏览器运行此套回归，不连接开发者的测试服务器。
