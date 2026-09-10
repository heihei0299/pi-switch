# 01: 认证判定改用实际绑定地址，非 loopback 默认拒绝启动

**What to build:** 让管理面的认证决策只依赖**实际绑定地址**，不再依赖 config 文件里的声明；非 loopback 且无密码时默认拒绝启动；提供显式的密码生成路径。端到端行为：`pi-switch webui start --host 0.0.0.0` 在无密码时**启动失败并说明原因**，或（带 `--generate-password`）生成密码并要求认证；loopback 启动行为不变。

**要修的真实缺陷**：`authMiddleware` 读 `cfg.Settings.Web.Host`（config 文件），而绑定用 CLI 的 `--host` 且从不写回 config。故 `--host 0.0.0.0` 时绑定对外、判定却是 loopback → 全部放行，**连配了密码文件也无效**。

**实现要点**（spec D1）：

- 认证模式在 `cmd` 层由 `parseHostPort` 的实际返回值算好后**传入** `NewMgmtRouter`，中间件不得再运行时读 config 判断 host
- `isLoopback` 收敛为只认 `127.0.0.1`、`localhost`、`::1`、`[::1]`；**移除 `0.0.0.0` 与 `::`**（绑定所有接口是 loopback 的反面）
- 非 loopback + 无密码 → 拒绝启动（fail-closed）；`--generate-password` 时生成随机密码写入 `webUIPasswordPath()`（权限 0600）并在启动打印一次
- 校验必须覆盖 **daemon 路径**：`daemon.Start` 之前在同一处校验，禁止绕过
- `handleWebUIInfo` 的 `authRequired` 改用同一输入源，避免继续谎报

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] `authMiddleware` 不再从 config 读取 host；认证模式由实际绑定地址决定（唯一输入源）
- [ ] `--host 0.0.0.0` 且无密码时启动**失败**（含 daemon 路径），错误信息说明如何提供密码或改用 loopback
- [ ] `--generate-password` 生成密码文件（权限 0600）并仅打印一次；随后非 loopback 请求无凭据返回 401
- [ ] `isLoopback` 不再把 `0.0.0.0`/`::` 判为 loopback
- [ ] `handleWebUIInfo` 的 `authRequired` 与中间件实际行为一致
- [ ] 测试：非 loopback + 无密码 → `/api/config` 401 或启动失败；`0.0.0.0` 不被判 loopback；「绑定地址与认证判定一致」的回归测试（含 `--host` 传参路径）
- [ ] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [ ] 不做 01 之外的行为收窄（A9 归 02，假成功归 03/04）

**已知风险**：本票会改 `NewMgmtRouter` 签名与 `cmd` 启动路径，属信任边界修复不可避免的接口变更；测试文件中依赖旧签名的调用需同步更新。
