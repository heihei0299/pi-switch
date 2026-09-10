# spec: 信任边界与契约诚实性收敛

**来源**：`docs/architecture-review.md` §3 的 P0 项——A1（管理 API 认证缺口）、A2（CLI 假成功）、A7（服务端假成功 API 且 WebUI 已接线）、A8（WebUI 密码无生成路径）、A9（Proxy 无认证且请求体无上限）。

**问题**：这些不是结构问题，是信任边界与契约诚实性问题。前者错一次就是漏洞，后者错一次是把"什么都没做"报告成成功。

## 已核实的事实（ticket 的判据来源，非 review 转述）

1. **A1 比 review 记载更严重**：`authMiddleware` 从 **config 文件**读 `cfg.Settings.Web.Host` 判断是否 loopback，而实际绑定用的是 CLI 的 `--host`，且**从不写回 config**（全仓无 `cfg.Settings.Web.Host = ...` 写入点，只有默认空值时填 `127.0.0.1`）。daemon 模式自 spawn `--host <host> --port <port>`（`internal/daemon/daemon.go:427`）。因此 `pi-switch webui start --host 0.0.0.0` 的实际行为是：对外绑定 0.0.0.0，而认证逻辑认为自己在 loopback → 请求全部放行，**配了密码文件也无法生效**（判断在密码检查之前就返回）。
2. **A8 确认**：`resolveWebUIPassword` 只读 `PI_SWITCH_WEBUI_PASSWORD` 与 `webUIPasswordPath()`，全仓无写该文件的代码。
3. **A9 确认**：`NewProxyRouter` 只挂 `gin.Recovery()`；`proxy_handlers.go:248` 是唯一读体点（`io.ReadAll(c.Request.Body)`），无 `http.MaxBytesReader`。
4. **A7 的一个前提不成立**：review 称"导出/恢复可复用 `handleBackups` 的备份能力"，但 `handleBackups` 本身是 `c.JSON(200, []string{})`，全仓**无任何真实备份实现**，故该项无"接已有实现"的选项。
5. **可实现性分级**（决定每项是"接线"还是"501"）：
   - 已有真实实现、可接线：`package add/show/delete`（服务端 `handlePackageAdd`/`handlePackageGet`/`handlePackageDelete` 均真实）、`handleDoctor`（可真实探测）。
   - 无实现：config export/import/restore、`/api/backups`、ccs POST import。
   - 空数组是**真值**，不算假成功：ccs GET providers、`handlePackagesList`。
   - **勘误（03 实施时发现）**：`handlePresets` 不属此列。服务端 `handlePresets` 返回 openai/anthropic/google/deepseek 四条真实静态预设（旧 Node 实现 `src/commands.js` 的 `preset list` 同样返回真实目录），旧的 Go CLI 打印 `[]` 并让 `preset show <id>` 对已存在的 id 也报 not found，属**失真**而非空真值。故 CLI 侧已接线到真实目录（见 03），票 04 不得按"presets 恒空、保留 200 空数组"的前提去改。
   - 语义不明：`handleInit`。
6. **判据核心**：只有**声称发生了副作用**的响应才算假成功。GET 返回空集合没声称任何副作用，保留 200；POST 声称"已导入/已导出/已恢复"而实际无操作，必须改 501。

## 决策（已确认）

- **D1 非 loopback 且无密码的默认行为 = fail-closed（拒绝启动）**；同时提供 `--generate-password` 显式开启"生成随机密码并写入 `0600` 文件、启动时打印一次"。不默认静默生成——静默生成会让"为什么连不上"变成难查的问题，而 fail-closed 的报错本身就是文档。
- **D2 CLI 接线方式 = 抽核心函数**（handler 与 CLI 共用同一份逻辑），不让 CLI 走 HTTP 调管理 API：同进程无需认证，也不引入新依赖，符合「不为单一调用方创建抽象层」。
- **D3 `handleInit` 语义不明 → 先返回 501**，不为它编造行为。

## 验收总判据（每票都必须满足）

- **不存在"返回 200 但无副作用"的端点或命令**。
- 信任边界以**实际绑定地址**为唯一输入，不依赖 config 声明。
- 除本 spec 明确收窄的行为外零回归：`go build ./...` 通过、`go test ./... -count=1` 全绿、`gofmt -l` 无输出。

## 票与依赖

| 票 | 覆盖 | 依赖 |
|---|---|---|
| 01 | A1 + A8：认证判定改用实际绑定地址；fail-closed；密码生成路径 | 无 |
| 02 | A9：Proxy 认证与请求体上限 | 01（复用认证模式与密码来源） |
| 03 | A2：CLI 诚实性 | 无（与 01/02 并行安全） |
| 04 | A7：服务端诚实性 + WebUI 入口收口 | 无（与 03 同族，可并行） |

## 明确不做

1. 不引入 OAuth/JWT/多用户认证——仍是一组 Basic（`admin:<password>`）。
2. 不"实现"config 备份/导出/恢复与 CCS 导入——本轮只让它诚实返回 501，真实现另立 spec。
3. 不改 `retry.go` 相关行为（A4）、不动 `_ = saveConfig` 的错误传播（A10，属 P1 另一族）。
4. 不为信任边界重构引入新的 service 接口层或包边界。
