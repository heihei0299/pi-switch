# 01 - 移除故障转移链（过渡清理，单候选直通）

Status: resolved
Scope: `internal/config` / `internal/server` 代理路由 / `webui` / `tui` / `docs`
Spec: `.scratch/remove-failover-chain/spec.md`

## 背景
- 单会话 `429` 触发全局 `cooldownUntil + failover` 候选池，导致所有会话级联挂死；本次为最小切移除跨供应商全局故障转移链，为后续 `per-conversation` 熔断让路，不落 ADR，保留 `retry.go` 原语供复用。

## 目标
- 切断跨会话污染：单请求单供应商单渠道直通，失败透传原 status/body，不重试、不冷却、不跳候选。

## 交付

### 1) 配置（`internal/config/config.go`）
- 移除 `Settings.Proxy.Failover []string` 生效语义（删除字段）。
- 读兼容：含 `failover` 的旧 `config.json` 忽略该键、不报错；`MigratedForSave` / 保存路径首次落盘删除该键。
- 移除 `health` 中 `failover profile not found` warning。

### 2) 代理路由（`internal/server/server.go` + 休眠 `internal/server/retry.go`）
- `resolveRoute` 重定义为单候选：
  - `supplier/channel/model` → `[supplier]` 精确 pin（保持）；
  - `supplier/model` → `[supplier]` 单候选，不再追加 `failover` 补位；
  - 裸 `model` → 仅当 `current` 供应商暴露该模型时 `[current]`，否则 `[]` → `502 no_route`（不再全量扫描）。
- `handleChatCompletions / handleStream / handleResponses` 等：单次直通，不再调用 `expandAttempts / pinChannelAttempts / admitForRound / waitForRound / isCooling / coolForAttempt / candidateCooldownKeys / coolingHint`，失败直接透传（`429/5xx/transport` 均直通），不写 `cooldownUntil`。
- `retry.go` 保留文件与类型不动，调用点加 `// retained for per-conversation breaker, not used in transitional passthrough` 注释。
- 错误类型：不再返回 `failover_exhausted`，统一为普通 upstream 透传；裸模型未命中为 `502 no_route`。

### 3) API / WebUI / TUI / 文档
- API：`PUT /proxy/failover` 改 `410 Gone`，body `{ error: { message: "failover removed, will be replaced by per-conversation breaker", type: "gone" } }`；`GET /health` 等回显移除 `failover`。
- WebUI：`webui/src/components/ProxyPanel.tsx` 移除 `FailoverEditor`、`api.setFailover`、`types.failover`、`i18n` failover keys；`ProxyPanel` 仅保留 host/port。
- TUI/CLI：移除 `Settings → Failover` 菜单与 `proxy failover` 生效路径，改为提示已移除。
- 文档：`README.md / README_ZH.md` 移除 `proxy failover` 命令与 Gateway Routing & Failover 中 failover 描述；`CONTEXT.md` 首段已移除“支持同模型 failover”。

## 验收
1. 含 `failover` 的旧 `config.json` 可加载，首次保存后不含该键；`GET /health` 不含 `failover`。
2. `PUT /proxy/failover` 返回 `410` + `gone`。
3. 路由：`supplier/channel/model` 仅打该渠道；`supplier/model` 仅打该供应商首渠；裸 `model` 仅走 `current` 否则 `502 no_route`。
4. 429 隔离：会话 A 对 X 触发 429 后，会话 B 下一请求不受影响（不 failover、不被冷却阻塞）。
5. 直通：`429/500/502/503/504/transport` 均直通原 status/body，不写冷却、不等待。
6. UI：Proxy 面板无 FailoverEditor；TUI 无 Failover 菜单。
7. `go test ./...` 与 `NODE_ENV=test npx vitest run` 通过；`retry.go` 保留但无调用。

## 不做
- 不实现新 `per-conversation` 熔断/限流/重试；不改变 `Supplier/Channel` 数据模型与 `Gateway` 聚合发布。
## 实施总结
- 提交：`7a94c97` — `feat(proxy): remove failover chain, single-candidate passthrough`
- 实现的 seams：S1 config兼容（旧config含failover可加载、保存时删除）、S2 resolveRoute单候选（三段/二段/裸模型）、S3 代理直通（429/5xx透传、429隔离）、S4 PUT /proxy/failover 410 Gone、S5 WebUI无FailoverEditor
- 验收标准：
  - [x] 1. 含 failover 的旧 config.json 可加载，首次保存后不含该键；GET /health 不含 failover
  - [x] 2. PUT /proxy/failover 返回 410 + gone
  - [x] 3. 路由：supplier/channel/model 仅打该渠道；supplier/model 仅打该供应商首渠；裸 model 仅走 current 否则 502 no_route
  - [x] 4. 429 隔离：会话 A 429 不影响会话 B（passthrough_single_test.go 验证）
  - [x] 5. 直通：429/5xx/transport 均直通原 status/body，不写冷却、不等待
  - [x] 6. UI：Proxy 面板无 FailoverEditor（ProxyPanel.failover.test.tsx）
  - [x] 7. go test ./... (8 packages) 与 webui npm run test (27 files 226 tests) 通过；retry.go 保留但无调用
- 测试结果：Go 8 packages passed (with 13 skipped legacy failover tests), WebUI 27 files 226 tests passed, 新增回归 5 文件（config/server/webui）
- typecheck：go vet ./... 通过
- 文档对齐：CONTEXT.md 首段移除 failover、README.md/README_ZH.md 移除 failover 命令与 Automatic failover 段、HomePanel/ProfilesPanel/SettingsPanel/i18n 清理
- 遗留 / 后续建议：per-conversation 熔断另起 spec 实现；retry.go 保留待新熔断复用；TUI Settings→Failover 已无入口但未单独 UI 测试；裸 model 未命中时 502 no_route 需在 Release Notes 提示用户改显式 pin
