# 02: Proxy 认证与请求体上限

**What to build:** 让 Proxy 面与非 loopback 绑定时的信任边界和 01 保持一致，并给请求体加上限。端到端行为：非 loopback 启动 Proxy 时无凭据请求被拒；超大请求体返回 413；loopback 本地开发行为不变。

**实现要点**（spec D1 的延伸）：

- `NewProxyRouter` 在非 loopback 时挂**同一套** Basic 认证（复用 01 的认证模式与密码来源，不新建第二套机制）
- proxy 入口加 `http.MaxBytesReader`：上限可配，缺省 32MB
- `internal/server/proxy_handlers.go:248` 是唯一读体点（`io.ReadAll(c.Request.Body)`），改造面应局限于此
- 若命中上限，返回 413 且不落库、不计费

**Blocked by:** 01: 认证判定改用实际绑定地址，非 loopback 默认拒绝启动

**Status:** ready-for-agent

- [ ] 非 loopback + 无密码时 Proxy 不可用（启动失败或请求被拒）；有密码时无凭据请求返回 401
- [ ] 请求体超过上限返回 413，且不产生请求落库/计费记录
- [ ] 上限可配置，缺省 32MB；loopback 场景行为与改动前一致
- [ ] 认证机制与 01 共用同一份实现（不出现第二套认证代码或第二处密码读取）
- [ ] 测试：非 loopback + 无密码 → proxy 拒绝；超大 body → 413；loopback 正常路径不回归
- [ ] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
