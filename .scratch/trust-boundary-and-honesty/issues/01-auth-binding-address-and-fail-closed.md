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

**Status:** resolved (2026-09-11)

- [x] `authMiddleware` 不再从 config 读取 host；认证模式由实际绑定地址决定（唯一输入源）
- [x] `--host 0.0.0.0` 且无密码时启动**失败**（含 daemon 路径），错误信息说明如何提供密码或改用 loopback
- [x] `--generate-password` 生成密码文件（权限 0600）并仅打印一次；随后非 loopback 请求无凭据返回 401
- [x] `isLoopback` 不再把 `0.0.0.0`/`::` 判为 loopback（空 host 亦不判为 loopback，见实施记录）
- [x] `handleWebUIInfo` 的 `authRequired` 与中间件实际行为一致
- [x] 测试：非 loopback + 无密码 → `/api/config` 401 或启动失败；`0.0.0.0` 不被判 loopback；「绑定地址与认证判定一致」的回归测试（含 `--host` 传参路径）
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [x] 不做 01 之外的行为收窄（A9 归 02，假成功归 03/04）

## 实施记录

### 交付

- `internal/server/server.go`：删除缺陷载体 `authMiddleware()`（它运行时读 `cfg.Settings.Web.Host`）；新增 `MgmtAuthOptions`、`ValidateBindAuth`、`ResolveAuthOptions`、`GenerateAndStorePassword`、`NewMgmtRouterWithAuth`、`storedWebUIPassword`、`basicAuthMiddleware`、`effectiveBindHost`；`isLoopback` 收敛；`splitHostPort`、`resolveWebUIPassword` 作为死代码删除
- `cmd/pi-switch/main.go`：四条启动路径收口为 `startWebUI` / `startProxy` / `startWebUIMode`；新增 `--generate-password` 解析；help 文本更新
- `internal/server/settings_handlers.go`：`handleWebUIInfo` 改为读取本请求实际生效的认证模式
- 测试：`internal/server/auth_binding_test.go`（13 个用例）、`cmd/pi-switch/main_test.go`（新增，该包此前无测试文件）
- `NewMgmtRouter()` 保留为 loopback 零参入口，因此 **39 个既有测试文件零改动**

### 三个实现中发现并修掉的真缺陷（超出 issue 原文）

1. **`--host ""` 是 fail-open**（Spec reviewer 判为 BLOCKING）：`isLoopback("")` 沿用旧逻辑返回 `true`，而 `r.Run(":43110")` 实测绑定 `[::]`（所有接口）——于是"没写 host"被读成"本地"，管理面全网可达且免认证。已改为**空 host 非 loopback**，`ValidateBindAuth` 拒绝、`NewMgmtRouterWithAuth` 装守卫、`effectiveBindHost` 让 `authRequired` 与之一致。反转验证：恢复旧逻辑后 `host=` 与 `host=___` 两个子用例 + 守卫用例立即失败。
2. **环境变量会让宣告的密码失效**：原实现把 `PI_SWITCH_WEBUI_PASSWORD` 也算"已配置"，于是 `--generate-password` 不生成、而 daemon 子进程按文件读到**另一个**密码，父进程打印的凭据根本不能用。收敛为确定性优先级 **generate > 密码文件 > 环境变量**，并加 `TestResolveAuthOptions_AnnouncedPasswordIsTheEnforcedOne` 端到端断言（宣告的密码 200、陈旧环境变量 401、子进程解析到同一秘密）。
3. **`ValidateBindAuth` 接受纯空白密码**：只查 `Password != ""`，`"   "` 会通过守卫。已改为 `TrimSpace` 后判空。

### Review 结果（Step ③，两个独立单轴 reviewer 并行）

- **Standards-only**：NO BLOCKING，4 follow-up + 11 advisory。已处理其中属于本票自身责任的项：错误信息提到已不读的 env var（**已恢复 env 支持**，A8 完成判据本就要求它）、`NewProxyRouterWithAuth` 无人调用且注释做出无法兑现的承诺（**已删除**，Proxy 认证归 02）、`splitHostPort` 死代码（已删）、新测试读开发者真实 config（**已加 `isolateConfig` 隔离**）、B8 恒真断言与 B2/B6 打不到缺陷（**已重写并反转验证**）、`authStateKey` 声明未用（gin 的 `Set`/`Get` 只收 string key，改为常量 `authStateCtxKey`）。
- **Spec-only**：AC1–AC5、AC8 SATISFIED；AC6 PARTIAL（缺 `--host` flag 路径测试）→ 已补 `cmd/pi-switch/main_test.go`；AC7 由本代理执行（见下）。BLOCKING #1（空 host fail-open）已按上节修复。同时把 `ValidateBindAuth` 文档里"guards the proxy"的失实表述改正。
- 两个 reviewer 均观察到审查期间 `server.go` 被并发编辑（我的一次批量编辑曾短暂产生三份重复类型声明）。**结论：本票的 editing 与 review 未做到严格串行，证据快照需以最终 diff 为准** —— 已在最终提交前重跑全部验证。

### 验证证据

- `go build ./...` 通过；`go vet ./internal/server/ ./cmd/...` 无输出；`gofmt -l internal/ cmd/` 无输出；`go test ./... -count=1` **16 个包全绿**
- 真实运行（临时 HOME/端口隔离，结束已清理，未污染真实 `~/.pi-switch`）：
  - `webui start --host 0.0.0.0` → 退出 1 + `refusing to start on non-loopback host 0.0.0.0 without a password: set PI_SWITCH_WEBUI_PASSWORD, create the password file, or pass --generate-password`
  - `webui start --host ""` → 退出 1（修复前会绑定所有接口并放行）
  - `--generate-password` → 密码文件 `-rw-------`（0600）+ 恰打印一次 + 正常监听
  - 端到端 curl（绑 0.0.0.0）：无凭据 `/api/config` **401**、`/healthz` **200**、正确凭据 **200**、错误凭据 **401**、`/api/webui/info` → `authRequired:true`
  - **daemon 路径**：无密码拒绝退出 1；`--generate-password` 起子进程；子进程强制认证（401/200）；`webui stop` 正常
  - `net.Listen(":0")` → `[::]:43769`，证明空 host 即通配绑定

### 未纳入本票的后续项

- `ValidateBindAuth` 不再声称守护 Proxy；**Proxy 认证与 body 上限仍归 02**（`startProxy` 目前不过守卫，这是已知且有意的缺口）
- 文档失实（`WEBUI_GUIDE.md`、`README.md`/`README_ZH.md` 的 flag 列表、`webui/src/i18n.tsx`、`SettingsPanel.tsx` 关于"saved setting 决定是否需要认证"的说法）**归 04 的前后端同批收口**——本票已把配置面板的承诺改准确（决定权在 `--host`），但 UI 文案需随 04 一起改
- `GenerateAndStorePassword` 保持导出：它是 spec/A8 完成判据点名的接口
- 预存密码文件不会被重新 chmod 为 0600（只在创建路径保证），记为 advisory

**已知风险**：本票会改 `NewMgmtRouter` 签名与 `cmd` 启动路径，属信任边界修复不可避免的接口变更；测试文件中依赖旧签名的调用需同步更新。
