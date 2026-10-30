# 简化整理与回归结果（2026-10-30）

## 改动范围

- 配置、数据库、Redis、JWT 和模板改为显式初始化；初始化错误向启动入口返回，不再依赖导入顺序执行外部 I/O。
- 删除 Token 响应阶段的重复查询与最后登录时间更新，改在凭据登录成功后更新。
- 管理员写操作复用同一请求已验证的认证时间，保留撤销检查和 15 分钟凭据窗口。
- PostgreSQL 数组编解码复用已有 pgx 依赖，保留 nil 写入空数组的约定；补空元素、引号、反斜杠、逗号及无效输入回归。
- 合并重复 URL 检查、mapstructure 解码、前端列表加载与导航图标，删除确认未使用的工具和配置字段。
- 前端 API 从认证状态读取 Token；退出等待服务端完成，失败显示错误并允许重试；明确退出完成状态，避免自动重新登录。
- 按用户要求保留第七项：HS512 加载能力、Redis Cluster 构造入口及 ScopedClientInfo 接口均未删除或收窄。

## 验证结果

| 检查 | 结果 |
|---|---|
| `go test -race -count=1 ./...` | 通过 |
| `python3 test/securityaudit/run.py` | 84 项顶层测试通过，其中 50 项隔离安全探针；无 race |
| `go vet ./...`、`go build ./...` | 通过 |
| 前端 `pnpm lint`、`pnpm build` | 通过 |
| `go build -tags embedspa` | 通过 |
| PostgreSQL/Redis 实时回归（`-race`） | 通过，使用授权测试服务器和独立测试数据 |
| Chromium 浏览器回归 | 15 项检查通过，无未捕获页面错误 |
| 文档链接、Shell/Python 语法、CI YAML、`git diff --check` | 通过 |

实时回归覆盖数组读写、唯一约束、管理权限、刷新轮换/重放/并发、真实 Redis Lua 与 TTL、撤销与退出、最后管理员的并发事务锁。浏览器覆盖 PKCE 登录与回调、四个列表页、401 刷新后使用新 Token 重试、退出等待/失败/重试、缺 Scope、深链刷新、窄屏/键盘、404、非法回调和拒绝授权。

浏览器回归发现并修复：

1. 授权确认页的 `no-referrer` 使 Chromium 原生表单提交携带 `Origin: null`，被现有同源检查拒绝。仅该页改用 `same-origin` Referrer 策略；空 Origin 和跨域拒绝规则保持。
2. 退出成功后清空认证状态与路由跳转之间，自动登录守卫可能抢先发起新授权。显式退出完成状态阻止了这次重新登录。

测试创建的 PostgreSQL 数据库与带随机前缀的 Redis 键均已清理，不使用已有账号，不执行 FLUSHDB。回归脚本和运行方式见 [测试总览](README.md)。CI 已增加独立 PostgreSQL/Redis services 和浏览器回归，远程 CI 本次未实际运行。

## 验证边界

Turnstile widget/Siteverify 使用测试替身，Telegram 和外部字体请求被阻断。未执行真实第三方验证、TLS 反代、Redis Cluster、网络故障注入、Docker 镜像构建、线上部署或完整跨浏览器视觉验收。普通 Go 测试不包含 build-tagged 实时集成测试；复现本报告需同时运行测试总览中的两类测试。
