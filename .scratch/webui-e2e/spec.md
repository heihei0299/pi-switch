# WebUI 真实端到端测试方案

Status: ready-for-agent

## 1. 目标与范围

验证用户通过真实浏览器使用嵌入式 WebUI 时，以下完整链路均可用：

1. 添加供应商。
2. 保存上游配置。
3. 从上游获取模型。
4. 在供应商侧编辑模型并暴露模型。
5. 网关侧获取暴露模型列表。
6. 网关侧编辑模型 JSON/元数据。
7. 网关侧勾选模型并发布到 Pi。
8. 发布后刷新页面和读取 `~/.pi/agent/models.json`，确认不是旧数据。
9. 包管理从真实 Pi 插件/扩展/技能目录导入。
10. 包管理列表、启用、禁用、卸载等生命周期。
11. 代理真实转发后，Stats 页面显示请求。
12. 桌面/移动端、中文/英文 locale、网络/API 错误均有可观察结果。

本方案的“真实”定义：

- WebUI 必须由新 Go 二进制的 `embed.FS` 提供，不使用 Vite mock server 代替后端。
- 浏览器必须实际访问容器中的 WebUI URL，并点击真实控件。
- 允许在隔离环境中 mock 外部上游 provider、models catalog 和 Pi 插件源；不使用真实 API key，不把外部网络可用性当成产品通过条件。
- 每个写操作必须同时验证：浏览器请求、HTTP 响应、页面刷新后的投影、文件或 SQLite 的权威事实。

## 2. 环境拓扑

```text
测试宿主机
  ├─ npm run build                    # 生成 WebUI + Go 二进制
  ├─ Chromium/Playwright              # 真实浏览器
  └─ incus exec pi-switch-web         # 实际 LXC 实例（当前实例名）
       ├─ 新二进制 /tmp/pi-switch-functional
       ├─ 隔离 HOME=/tmp/e2e/home
       ├─ 隔离 config.json / requests.db / requests.log
       ├─ 真实 Go WebUI :<web-port>
       ├─ 真实 Go Proxy :<proxy-port>
       └─ mock upstream :<upstream-port>
```

执行前固定记录：

```text
container = pi-switch-web
binary version
binary build-info
container IP
web-port / proxy-port / upstream-port
isolated HOME/config/DB paths
```

严禁复用真实 `/root/.pi-switch/config.json`、`/root/.pi-switch/requests.db`、`/root/.pi/agent/models.json`。测试结束必须停止临时 daemon 并删除隔离目录。

### 2.1 本机 agent-browser 拓扑（Local real-browser gate）

除 Real LXC gate 外，允许在本机直接运行真实浏览器 gate。该模式不使用 Incus，应用和 mock upstream 均在本机隔离 HOME 中运行：

```text
本机 /tmp/pi-switch-local-e2e
  ├─ 新 Go 二进制
  ├─ 隔离 HOME/config/requests.db/requests.log
  ├─ 隔离 ~/.pi/agent/models.json
  ├─ 真实 Go WebUI 127.0.0.1:<web-port>
  ├─ 真实 Go Proxy 127.0.0.1:<proxy-port>
  └─ mock upstream 127.0.0.1:<upstream-port>

agent-browser → http://127.0.0.1:<web-port>
```

本机浏览器必须使用 `agent-browser` CLI 驱动真实 Chromium，不使用 Playwright mock route 替代页面 API。浏览器操作、浏览器请求、HTTP 响应、刷新后的页面和隔离文件/SQLite 仍按同一条证据链断言。

执行前准备 agent-browser 自有 session 和可写 runtime 目录：

```bash
agent-browser skills get core --full
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/agent-browser-runtime}"
mkdir -p "$XDG_RUNTIME_DIR" && chmod 700 "$XDG_RUNTIME_DIR"
export AGENT_BROWSER_SESSION="$(agent-browser session id --scope worktree --prefix pi-switch-webui)"
export AGENT_BROWSER_ALLOWED_DOMAINS=127.0.0.1
agent-browser open http://127.0.0.1:<web-port>
agent-browser wait --load networkidle
agent-browser snapshot -i
```

说明：

- `agent-browser` 每次 `snapshot -i` 产生的新 `@ref` 只对当前页面状态有效；点击、导航、表单提交或 React 重渲染后必须重新 snapshot，不能复用旧 ref。
- 优先使用 `find role`、`find text`、`find placeholder`、`find label` 和当前 snapshot 的 `@ref`；不要用固定坐标或猜测 DOM 层级。
- `agent-browser network requests` 只能用于观察/断言真实请求，禁止用 `network route` 拦截 `/api/*`。
- 英文 locale 在启动新 session 前设置 `LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8`；中文 locale 使用默认系统语言或 `zh-CN` 浏览器环境。

推荐本机启动命令：

```bash
npm --prefix webui run build
GOOS=linux GOARCH=amd64 bash scripts/build-go.sh .scratch/webui-e2e/pi-switch-functional
bash .scratch/webui-e2e/local-services.sh       # 必须由持久后台 job 持有
```

`local-services.sh` 必须为 WebUI、proxy、mock upstream 设置隔离环境变量：`HOME`、`PI_SWITCH_CONFIG`、`PI_SWITCH_CONFIG_DIR`、`PI_SWITCH_DB`、`PI_SWITCH_MODELS`、`PI_AGENT_SETTINGS`。一次性 shell 退出可能回收 daemon 子进程，因此测试服务应由受控后台 job 持有，结束时统一停止并清理。

## 3. 固定测试夹具

### 3.1 初始 config

隔离 `config.json` 至少包含：

- `current: "mock"`
- `profiles.mock`
- `mock/main` channel
- 初始模型：`model-a`、`model-b`
- 初始 `exposedModels: ["model-a"]`
- `conversationSource: "off"`
- WebUI/proxy 使用随机端口

初始状态用于验证新增 `model-b` 不会被旧 projection 或旧 gateway 文件掩盖。

### 3.2 Mock upstream

mock upstream 只承担外部 provider seam，不 mock pi-switch 自己的 `/api/*`：

| 路径 | 行为 |
|---|---|
| `GET /v1/models` | 返回 `model-a`、`model-b`、必要时返回 `model-c` |
| `POST /v1/chat/completions` | 返回确定性的 JSON completion 和 usage |
| `POST /v1/chat/completions` + `stream:true` | 返回确定性的 SSE chunks、usage、`[DONE]` |
| `401/403/500` fixture | 用于测试 Test Connection、错误提示和重试边界 |

mock 必须记录收到的请求数、路径、Authorization 是否存在、请求 model，供测试断言转发确实发生。

### 3.3 Gateway current fixture

将 `PI_SWITCH_MODELS` 指向隔离的 `models.json`，初始内容为：

```json
{"providers":{}}
```

发布后必须独立检查：

- 文件存在且是合法 JSON。
- `providers` 中包含预期固定 gateway provider。
- 新模型 ID、显示名、模型 extra 与 UI 最后一次保存一致。
- 再次 `GET /api/models/gateway/preview` 的 `pending_count` 为 0。
- 浏览器刷新后仍显示新模型，而不是只在当前 React state 中显示。

### 3.4 Pi plugin fixture

包管理测试必须使用目标 Pi 版本的真实插件目录/manifest 约定，夹具不得只手工写 `settings.json#packages` 字符串数组。准备以下至少一种真实样本：

- extension/plugin 文件。
- skill 目录及 `SKILL.md` 或目标 Pi manifest。
- prompt/theme（若目标版本支持）。
- package identity、版本、启用状态和能力字段。

夹具必须覆盖：一个有效包、一个缺 manifest 包、一个禁用包、一个重复 identity 包。具体目录名和 manifest 字段以目标 Pi 版本为准，并在测试记录中固定。

## 4. 测试生命周期

### 4.1 准备

1. `npm run build`，记录二进制版本和 build identity。
2. `incus file push` 新 `linux/amd64` 二进制到容器临时路径。
3. 创建隔离 HOME、config、DB、gateway models file、catalog fixture 和 Pi plugin fixture。
4. 随机分配 upstream/WebUI/proxy 端口，确认端口为空。
5. 启动 mock upstream。
6. 用新二进制启动真实 WebUI 和 proxy daemon。
7. 用 `curl` 仅做 readiness check：`/healthz`、`/api/state`、`/api/buildInfo`。
8. Chromium 打开真实 WebUI URL，等待首屏 API 完成。

### 4.2 每个用例

每个用例都必须：

1. 记录操作前 config/gateway/DB 快照摘要。
2. 通过浏览器完成 UI 操作。
3. 等待对应真实 API response，不使用固定 sleep 代替。
4. 断言响应状态和 JSON contract。
5. 刷新页面或重新进入面板。
6. 检查权威文件/DB 和页面投影。
7. 失败时保存 console、pageerror、requestfailed、API body、screenshot 和脱敏快照。

### 4.3 清理

按逆序执行：

1. 停止 proxy daemon。
2. 停止 WebUI daemon。
3. 确认临时端口无 listener、PID 文件已清理。
4. 停止 mock upstream。
5. 关闭本次命名的 `agent-browser` session。
6. 删除隔离 HOME 和临时文件。
7. 确认真实容器 `/root/.pi-switch`、`/root/.pi` 没有变化；本机模式则确认真实 `$HOME/.pi-switch`、`$HOME/.pi` 没有变化。

### 4.4 本机 agent-browser 操作循环

每个真实浏览器用例遵循以下固定循环：

```bash
# 页面变化后重新获取可操作 ref
agent-browser snapshot -i
agent-browser find role button click --name '👤 供应商' --exact
agent-browser wait --load networkidle

# 写操作前清空请求记录，写操作后只观察真实请求
agent-browser network requests --clear
agent-browser find role button click --name '保存' --exact
agent-browser wait --load networkidle
agent-browser network requests --filter '/api/'

# 刷新页面，再检查投影、错误和证据
agent-browser reload
agent-browser wait --load networkidle
agent-browser errors
agent-browser console
agent-browser screenshot artifacts/<run-id>/<case-id>/browser.png
```

推荐断言顺序：

1. 用 `snapshot -i` 确认真实控件和当前 ref。
2. 通过 `find`/`click`/`fill`/`check` 完成用户操作。
3. 用 `network requests` 确认浏览器实际发出的 method、URL 和 HTTP status；不能只看 toast。
4. `reload` 或切换面板，确认不是 React 当前 state 假象。
5. 用 `agent-browser eval` 做 viewport/DOM 可观察性检查；用 `curl` 或真实页面 `fetch` 做 readiness/API contract 检查，但不能代替 UI 操作。
6. 将 console、page error、failed request、截图和脱敏后的 API/权威快照写入本次 run 目录。

## 5. 核心用例清单

### E2E-WEB-001 首屏和真实嵌入资源

**操作：**

1. 浏览器打开真实 WebUI URL。
2. 等待 `/api/state`、`/api/proxy/status` 和页面资源完成。
3. 检查根页面、静态 JS/CSS 和导航。

**断言：**

- HTTP 200。
- 页面显示当前 profile 和导航。
- `/api/buildInfo.webui.embedded=true`，asset count 大于 0。
- console/pageerror/requestfailed 均为空。

### E2E-WEB-002 添加供应商

**操作：**

1. 点击「添加供应商」。
2. 填写名称 `ui-provider`、mock Base URL、API key fixture。
3. 保存。
4. 刷新页面并重新进入供应商页。

**断言：**

- `POST /api/profiles` 返回 200 且 contract 合法。
- config 中出现 `ui-provider`，名称、API 类型、Base URL 未丢失。
- 页面重新加载后仍显示 `ui-provider`。
- API key 不出现在测试日志、截图或错误 toast 中。

### E2E-WEB-003 保存上游/channel

**操作：**

1. 编辑 `ui-provider`。
2. 添加名为 `main` 的上游。
3. 填写 mock Base URL、API 类型、Responses mode、headers。
4. 保存并刷新。

**断言：**

- `PUT /api/profiles/:name` 返回 200。
- config 中 `upstreams[0].name` 为 `main`。
- channel 的 API、headers、responses mode 和上游 URL 均保持。
- 非法 channel 名称、重复 channel 名称在 UI 侧阻止或返回可读 400。

### E2E-WEB-004 从上游获取模型

**操作：**

1. mock upstream 的 `/v1/models` 返回 `model-a`、`model-b`、`model-c`。
2. 进入「供应商 → 模型」。
3. 点击「从 provider 获取」。
4. 等待 `POST /api/profiles/:name/fetch-models`。

**断言：**

- mock 确实收到 `GET /v1/models`。
- WebUI 显示三个模型。
- 其他 channel 的模型池不被覆盖。
- 新拉取模型默认不自动加入 exposed set，除非产品契约明确要求。
- 返回的模型 metadata 与 provider/catalog 结果一致，不凭空制造不在源数据中的字段。
- refresh 后模型仍存在。

### E2E-WEB-005 供应商侧暴露模型

**操作：**

1. 初始只暴露 `model-a`。
2. 勾选 `model-b`。
3. 点击模型窗口「保存」。
4. 关闭并重新打开模型窗口，刷新供应商页。

**断言：**

- checkbox 状态改变后，`PUT /profiles/:name/models` 和 `PUT /profiles/:name/expose?channel=main` 都必须发送。
- 两个响应均为成功且 `backup:null` 等可选 nullable 字段不会造成 decoder 失败。
- config 中 `exposedModels` 包含 `model-b`。
- 刷新后 `model-b` 仍为暴露状态。
- 保存失败时不能只更新 UI state 后静默回退。

**当前已知红测：** `06-webui-model-exposure-save-reverts.md` 已证明 models PUT 200 后 expose PUT 未发送，根因是 `backup:null` decoder 不兼容。

### E2E-WEB-006 网关获取模型

**操作：**

1. 在供应商侧完成 `model-b` 暴露。
2. 进入「网关」。
3. 等待 `GET /api/models/gateway/preview`。

**断言：**

- gateway group 包含 `supplier/channel/model-b`。
- 页面显示该模型的 published/pending 状态与后端 preview 一致。
- 页面刷新、切换导航再回来后仍显示 `model-b`。
- `current`、`proposed`、`diff`、`groups`、`removed` 五类 projection 不互相矛盾。

### E2E-WEB-007 网关侧填写/编辑模型

**操作：**

1. 在 Gateway 的 JSON/editor 区域修改 `model-b` 的显示名或一个保留 extra 字段。
2. 点击格式化。
3. 检查 structured preview。
4. 点击「应用到 Pi」。

**断言：**

- 合法 JSON 编辑会更新 canonical draft 和 structured projection。
- 非法 JSON 显示错误行，不能覆盖原 proposal。
- 保存后 `models.json` 中新显示名/extra 存在。
- 刷新 Gateway 页面后编辑仍存在。
- 手工 extra 字段保留，但不能导致无变化时 pending 永远不归零。

### E2E-WEB-008 网关勾选并发布模型

**操作：**

1. 在候选分组勾选新暴露的 `model-b`。
2. 等待 selection preview。
3. 点击「应用到 Pi/保存」。
4. 刷新 Gateway 页面。
5. 重新请求 preview，并通过 proxy `/v1/models` 检查可见模型。

**断言：**

- selection preview 的 `proposed` 包含 `model-b`。
- publish/apply 返回 200。
- `~/.pi/agent/models.json` 包含 `model-b`，不是旧模型列表。
- Gateway 页面刷新后仍显示 `model-b`。
- `pending_count=0`，除非期间确实又产生新编辑。
- proxy `/v1/models` 返回新模型。

**与 E2E-WEB-005 的关系：** 如果该用例只显示旧模型，应先区分供应商 expose 是否已持久化；直接发布一个已暴露模型的 Gateway 链路必须单独记录，不能把两个根因混写。

### E2E-WEB-009 供应商测试连接

**操作：**

1. 对可访问 mock upstream 点击「测试连接」。
2. 对返回 401、500、超时的 mock upstream 分别测试。

**断言：**

- 正确 upstream 返回 success true 和真实量级 response time。
- 401/403 显示 API key/权限错误。
- 网络错误/超时显示可读错误并有超时上限。
- 测试连接不写入请求统计、不修改 config。

### E2E-WEB-010 包管理导入真实 Pi 插件

**操作：**

1. 在隔离 HOME 中放入目标 Pi 版本的真实 plugin/extension/skill/prompt/theme fixture。
2. 打开「包管理」。
3. 点击「从 Pi Agent 导入」。
4. 等待 `POST /api/packages/import`。
5. 刷新页面并查看 package 详情。

**断言：**

- 导入数量与实际 fixture 数量一致。
- 每个 package 的 identity、版本、启用状态和能力标记正确。
- 重复导入幂等，不产生重复行。
- 缺失 settings 文件、空 package 列表、发现真实目录但无 manifest 三种情况有不同且可读的结果。
- 导入真实 plugin 后，package list 能展示，而不是只插入一个字符串 spec。
- 页面不能把“没有扫描到配置”伪装成成功导入 0 个包。

**当前已知红测/能力缺口：** `08-package-import-pi-plugins.md` 已记录当前实现只读取 `~/.pi/agent/settings.json#packages`，不扫描实际 Pi plugin/skill 目录；修复前必须先固定目标 Pi 版本的目录契约。

### E2E-WEB-011 包管理生命周期

**操作：**

1. 通过 UI 添加一个 local/package fixture。
2. 刷新并确认出现在列表。
3. 切换启用/禁用。
4. 卸载。
5. 刷新并检查状态。

**断言：**

- add/install 不是只写成功 toast；实际包内容/metadata 或明确的未安装状态可验证。
- enable/disable 与 DB、页面状态一致。
- uninstall 后从 installed 列表消失，但历史 identity 是否保留必须符合契约。
- 网络/无效 spec 错误不会留下半条 package 记录。

### E2E-WEB-012 真实代理请求与 Stats

**操作：**

1. 通过真实 proxy 向 mock upstream 发起 non-stream 和 stream 请求。
2. 请求完成后立即进入 Stats。
3. 检查请求明细、provider/model 聚合、token 和 cost。
4. 等待 1 秒后再次刷新 Stats。

**断言：**

- proxy 和 mock upstream 均收到请求。
- SQLite、legacy log 和 Stats 页面都能看到请求。
- stream/non-stream usage 一致。
- 刚完成请求不能因为当前毫秒窗口边界被漏掉。

**当前已知红测：** `07-stats-current-window-upper-bound.md` 已证明秒精度 `ts` 配合毫秒右开窗口会漏掉当前秒请求。

### E2E-WEB-013 响应式和 locale

**viewport：**

- 375×812
- 390×844
- 768×900
- 1024×900
- 1440×1000

**locale：**

- `zh-CN`
- `en-US`

**断言：**

- 导航、供应商、模型 modal、网关、包管理、Stats、Settings 均可到达。
- 移动端 drawer 打开后可操作，关闭后不遮挡内容。
- `scrollWidth <= clientWidth + 1`。
- 不依赖单一语言的文本定位；优先 role、label、稳定 test id。
- 每个 viewport 失败保存截图。

### E2E-WEB-014 API/网络错误恢复

**操作：**

1. 让 stats API 返回 malformed JSON。
2. 让 gateway preview 返回 500。
3. 让 package import 返回错误。
4. 恢复 API 后重新进入面板。

**断言：**

- 当前面板显示错误，不导致整个 App 白屏。
- 上一次成功 snapshot 按契约保留或明确清空。
- 恢复后可重新加载成功。
- console 不出现未处理 promise rejection。

## 6. 浏览器自动化约束

推荐使用 Playwright，但测试入口必须支持真实 URL：

```bash
BASE_URL=http://<container-ip>:<web-port> \
  npx playwright test e2e/real-webui.spec.ts
```

真实 suite 不得 `page.route("**/api/**")` 伪造 pi-switch API。允许 route mock 的范围仅限：

- 外部上游 provider URL。
- 外部 models catalog URL。
- 明确的 Pi plugin fixture source（若该 source 不是本地文件）。

建议给关键控件增加稳定属性：

```text
[data-testid="add-profile"]
[data-testid="fetch-models"]
[data-testid="save-models"]
[data-testid="expose-model-model-b"]
[data-testid="gateway-preview"]
[data-testid="gateway-apply"]
[data-testid="package-import"]
```

若暂时不能加 test id，使用 role/label/aria-label，不使用第几个 checkbox、中文/英文裸文本或 DOM 层级猜测。

## 7. 失败证据格式

每个失败用例保存：

```text
artifacts/<run-id>/<case-id>/
  browser.png
  console.json
  page-errors.json
  requests.json
  responses.json       # 脱敏
  state-before.json
  state-after.json
  config-after.json    # 脱敏
  models-after.json
  stats-after.json
  db-summary.json
  daemon-status.txt
  upstream.log
```

凭据、API key、Authorization header 必须脱敏。正常通过时只保留摘要，不提交 PNG/HAR/日志到 Git。

## 8. 分层执行门槛

### Pull Request gate

- WebUI typecheck。
- WebUI unit tests。
- Playwright mock-upstream responsive suite。
- Go vet 和隔离 DB Go tests。
- build identity/embedded asset smoke。

### Real LXC gate

- `incus exec pi-switch-web` 真实 Go WebUI。
- 真实 Chromium URL。
- E2E-WEB-001～014 中适用用例。
- 至少包含供应商→模型→暴露→网关→发布的完整链路。
- 至少包含一个真实 Pi plugin fixture 的导入链路。

### Local real-browser gate

- 本机隔离 HOME 中启动真实 Go WebUI、proxy 和 mock upstream。
- `agent-browser` 驱动真实 Chromium 打开 `http://127.0.0.1:<web-port>`。
- 不使用 `/api/*` route mock；浏览器 network log 必须能看到实际请求和响应。
- 至少覆盖 E2E-WEB-001、002、004、005、006、007、008、010、011、012、013。
- 本机 gate 也必须检查刷新后的页面、隔离 `models.json`/SQLite/requests.log，并执行清理。
- 若 LXC 或本机缺少 Chromium，不得将 API smoke 误标记为 Local real-browser gate PASS。

### External provider/nightly gate

只有具备专用测试凭据、脱敏策略和可回滚环境时，才执行真实外部 provider；不得把个人 API key 或不可重复的外部网络状态作为 PR gate。

## 9. 通过标准

一次 run 只有在以下条件全部满足时标记 PASS：

- 所有 P0 核心链路通过：添加供应商、获取模型、暴露模型、Gateway preview/edit/publish、package import。
- UI 操作、实际 API、权威文件/DB、刷新后的页面四者一致。
- 无未处理 console/page error。
- 无未解释的 5xx 或 failed request。
- daemon、临时端口、mock upstream 和隔离目录已清理。
- 失败项必须有可复现的最小现场和明确根因；不能用“接口返回 200”替代真实用户结果。

当前实现不应标记全绿，至少保留以下红项直到修复并重跑：

- 模型 expose 保存的 `backup:null` decoder 问题。
- Stats 当前秒时间窗口边界问题。
- 实际 Pi plugin/extension/skill 导入能力缺口。

## 10. 本次 Local real-browser 实测记录

本次运行使用本机 `agent-browser` CLI 驱动真实 Chrome for Testing，不使用 Incus，也没有对 pi-switch 的 `/api/*` 做 route mock。

固定记录：

```text
agent-browser CLI = 0.36.0
Chrome for Testing = 151.0.7922.34
WebUI = http://127.0.0.1:46882
Proxy = http://127.0.0.1:46883
Mock upstream = http://127.0.0.1:46881
Build version = 20260908.0.2
Build target = linux/amd64
WebUI embedded = true
```

执行过的真实浏览器链路：

- 首屏和 `buildInfo`：通过，嵌入资源 `assetCount=2`。
- 添加 `ui-provider`：通过，刷新后仍存在。
- mock `/v1/models` 获取 `model-c`：通过，浏览器页面和 mock 请求日志均可观察。
- 供应商模型暴露：红。点击保存后 network log 只有 `PUT /api/profiles/mock/models 200`，没有 `PUT /api/profiles/mock/expose?channel=main`；这是 `backup:null` decoder 回归。
- 为隔离验证 Gateway，下游使用一次明确记录的真实 API exposure recovery；该 recovery 不计入供应商 UI expose 通过。
- Gateway 候选列表：页面可看到 `model-b`；Gateway JSON 编辑和 apply 请求返回 200，但刷新后 `pending_count=2`，手工编辑值与 canonical proposal 不一致，不能标记通过。
- Proxy non-stream 和 stream：通过；stream 收到 `[DONE]`，usage 可观察。
- Stats：普通页面可见真实请求；按请求 `ts` 精确查询得到 `exact=0`、放宽 1 秒得到 `widened=2`，复现当前秒边界红项。
- 包管理：真实浏览器完成 settings package 导入、添加、禁用、卸载；本次仍未满足真实 Pi extension/skill/theme 目录 fixture gate。
- locale/responsive：`zh-CN` 移动端 `390x844` 的 `scrollWidth=390`；另开 `en-US` session 验证 Profiles 页面；最终 `agent-browser errors` 和 `console` 均为空。

本次 run 结束时已关闭命名 browser session，停止 WebUI/proxy/mock，释放测试端口，删除隔离目录，并核对真实 `$HOME/.pi-switch`、`$HOME/.pi` 未变化。因此本次结果为 **RED，不得标记全绿**。
