# 隔离安全回归

准备当前 `go.mod/go.sum` 锁定依赖后运行：

```bash
go mod download
python3 test/securityaudit/run.py
```

如宿主 Go 缓存只读，可指定已有的可写缓存：

```bash
GOMODCACHE=/path/to/writable/modcache GOCACHE=/path/to/writable/buildcache \
  python3 test/securityaudit/run.py
```

脚本强制离线运行，**使用项目原样的 go.mod/go.sum，不替换或降级依赖**，并启用 `-race`。缓存不全或工具链过旧时会失败；不能将这些失败解释为漏洞复测通过。

脚本将相关生产源码（包括完整 OAuth2 server/manager、TokenStore、会话选项、登录、注册、退出、客户端策略、Introspection/UserInfo、管理员处理和配置替换函数）原样复制到临时模块。仅替换应用初始化和外部服务：不读取真实 config.yaml/.env，不启动 HTTP 监听，不接触现有 PostgreSQL、Redis、用户或令牌。Turnstile 正向登录测试使用内存 HTTP transport，真实验证函数仍参与执行。

SQL 驱动提供人工身份和事务检查；管理员并发测试模拟表锁，验证事务边界、最后管理员保护及 commit 错误传播，不能替代 PostgreSQL 实际锁验证。Redis 命令 hook 支持测试所需的字符串、集合、过期、SETNX、事务管道及 Lua 对应的安全操作；Lua 是语义适配，**没有执行真实 Redis Lua 引擎**。因此真实 Redis TTL/集群/网络故障、PostgreSQL 约束与锁、TLS/反代及浏览器行为仍需集成验证。

当前共 48 项顶层回归，覆盖原严格复审的安全断言，以及正常 public/confidential 登录和刷新、带 scope 的 HTTP 重放、跨客户端撤销干扰、用户版本丢失、撤销与签发并发、提交故障、SID 轮换、CSRF/Origin、限速、防转发头伪造、Cookie、安全头、管理员事务和近期认证。修复后应全部通过；不能把断言改成接受不安全行为。`.go.fixture` 由脚本构建执行，已单独纳入 CI，常规 `go test ./...` 不包含这些探针。

修复及本次验证范围见 [修复复测报告](../../docs/security-remediation-2026-10-27.md)。历史发现和失败证据保留在 [严格复审](../../docs/security-review-strict-2026-10-27.md) 及其原始结果中。
