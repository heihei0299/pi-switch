# 03: `gateway health` 的 `has_models_file` 反映真实文件

Status: resolved (2026-09-11)

**What to build:** `GET /api/gateway/health` 的 `has_models_file` 改为真实检查
`gateway.ModelsPath()`（`PI_SWITCH_MODELS` 优先，否则 `~/.pi/agent/models.json`）；
文件不存在时为 `false`。

**为什么**：`gateway_handlers.go:219` 的该字段是**字面量 `true`**，从不与文件系统核对。前端把它
解成必填布尔（`webui/src/apiSchema.ts:819/832`），所以文件和脚本会把它当成"已发布"的依据。
同响应里的 `running`/`last_notify`/`message` 也是字面量，但 `mode: "logical-isolation"` 表明网关
没有常驻进程、这些字段本无对应事实；`has_models_file` 不同——它**有**一个可核对的事实。

**不改的**：`POST /api/gateway/start` 的恒定成功载荷——查证后判断是有意设计（网关是"发布
models.json"这个逻辑概念，没有可启动的进程），本批只修谎报的字段，不删这个端点。

**Seam**：真实 HTTP（`NewMgmtRouter` + `callMgmt`）。

**验收**
- [ ] `PI_SWITCH_MODELS` 指向不存在路径 → `has_models_file: false`
- [ ] 指向存在文件 → `true`
- [ ] `upstreams_total` 仍反映真实 profile 数（既有行为不回退）
- [ ] 代码里不再有该字段的字面量，并注明其余字段为何是常量

**测试**：`internal/server/gateway_health_honesty_test.go`（用 `t.Setenv("PI_SWITCH_MODELS", ...)`
隔离——`isolateConfig` 不隔离该变量，否则测试会读开发机真实的 models.json）
