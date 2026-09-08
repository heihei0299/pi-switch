# pi-switch 系统不变量 Contract

- **来源**：`actionable-improvement-plan-20260908-1837.md` 的 IMP-01
- **状态**：accepted；决策见 `docs/adr/0011-canonical-system-contract.md`
- **适用范围**：IMP-02～IMP-05 及后续工作包的行为基线
- **规则**：以下措辞是可测试的 contract；不得用“自动尝试”“必要时 fallback”“尽量保留”等未定义语义替代。

## 1. 所有权与事实来源

| 领域 | 唯一事实来源 | 派生视图/兼容层 | 写入边界 |
|---|---|---|---|
| Supplier、Channel、Model、exposed model | `~/.pi-switch/config.json` 的 `profiles` | Gateway preview、`models.json`、路由目录、WebUI 结构化视图 | Supplier/Channel/Model API；不因 Gateway 失败回写或阻断 Supplier 保存 |
| Gateway 发布配置 | 后端生成的 canonical proposed plan | WebUI preview/current/diff、`~/.pi/agent/models.json`、Pi provider 列表 | 仅 `PUT /models/gateway` / GatewayPanel 显式发布；Supplier 或 settings 变更不自动写入 |
| 请求事实 | SQLite `requests` row | Stats 聚合、请求明细、导出 DTO | 请求完成时写入 provider/model/status/latency/usage/cost 等事实；已落库事实不因展示归属重写 |
| 对话归属 | 请求显式 ID，或统一 matcher 对 session snapshot 的结果 | Stats conversation view、标题和展示解码 | 显式 ID 优先；启用 sessionScan 时按统一 matcher 即时计算；不扩大时间窗口掩盖歧义 |
| pi session | `~/.pi/agent/sessions/<id>/*.jsonl` | session snapshot、标题候选、归属候选 | pi-switch 只读扫描，不把幽灵 `--no-session` 会话写入 session 目录 |
| daemon 运行态 | 受管理进程 identity + 端口 health check | PID 文件、Proxy/WebUI status、WebUI 状态 | PID 文件是定位信息，不单独证明 healthy；发布/配置文件不是运行态事实来源 |
| WebUI 编辑态 | 单一 canonical draft | JSON 文本、structured form、selection、preview | JSON 与结构化视图互相派生；不能用只读结构化字段替代直接 JSON 编辑 |

## 2. 空值、缺失与默认值

### 2.1 通用规则

1. 缺字段、`null`、空数组 `[]` 是三个不同输入状态；decoder、migration 和业务逻辑必须分别处理。
2. 未规定默认值的字段不得由调用方猜测；必须在唯一 boundary 明确拒绝或补默认值。
3. 默认值只在 boundary 产生一次，之后在内部 canonical value 中视为已存在；组件和 handler 不得重复猜测同一默认值。
4. 空字符串在要求非空的 identifier 中等价于缺失并拒绝；在可选文本字段中按该字段 contract 处理。

### 2.2 Config 与模型

| 输入 | 语义 |
|---|---|
| `profiles` 缺失或 `null` | 读取边界视为空 map；写入时序列化为合法 config shape |
| `upstreams` 缺失 | 使用既有 legacy migration 规则映射为单个 `main` channel；不在普通读路径重复迁移 |
| `exposedModels` 缺失 | 仅表示旧配置需要 migration；migration 完成前不得解释为“全部暴露” |
| `exposedModels: []` | 明确表示零模型暴露；Gateway、`/v1/models`、route resolution 和 WebUI 均保持零暴露 |
| `exposedModels: null` | 非法配置；在 boundary 报出字段路径错误，不转换为 `[]` 或全部暴露 |
| model `id` 缺失、`null` 或空字符串 | 非法模型项；不生成空 ID 行、不发布、不路由 |
| model metadata 缺失或 `contextWindow`/`maxTokens` 为 0 | 保留可用的本地模型项；不伪造 context/maxTokens/cost；clamp 不重写请求中的三个 max key，cost 记为 unknown |

### 2.3 Gateway

| 输入 | 语义 |
|---|---|
| models 文件不存在 | current 为空；preview 仍返回 canonical proposed；publish 创建父目录并原子写入 |
| `providers` 缺失或 `null` | current 为空 map；不删除或覆盖任何不存在的第三方 provider |
| fixed provider 无 exposed model | canonical proposed 不包含该 provider；发布后不保留 stale fixed provider |
| 第三方 provider | 不属于 pi-switch 管理范围，发布时原样保留其 entry 和未受管字段 |
| `compat`/`headers`/其他 extra 缺失 | 不生成空的伪字段；current 中存在的受允许 extra 合并进 canonical proposed |
| validation/conflict 非空 | publish 零写入；不部分写入、不先写临时目标再报告冲突 |
| 连续 publish 同一 canonical plan | 结构等价且 `pending_count=0`；不得因 normalization 或 extra merge 产生漂移 |

### 2.4 Responses 与请求

1. `responsesMode` 缺失或新建 profile 默认 `auto`；已有配置不批量迁移。
2. `auto` 只按 provider 声明的 `api` 决定：`openai-responses` 为 passthrough，`openai-completions` 为 convert；不做运行时能力探测。
3. 不兼容的 inbound/provider/mode 组合在发送 upstream 前失败；不猜 URL、不静默降级、不新增 failover。
4. `400 invalid_request_error` 的 max-token retry 只允许按既定规则重写 `max_output_tokens=16` 一次；retry 必须复用同一 outbound request builder 和 header policy。
5. 原始 body 与转换后 body 各自只执行一次同一 `ClampMaxTokens` 算法；不得新增第二套 clamp。
6. usage 中 cached/reasoning 的缺失、零值和已知值保持可区分；reasoning 是 completion 的子集，不重复累加。

### 2.5 Stats 与对话

1. SQLite request row 的 provider、model、status、latency、usage、cost 是 immutable request facts。
2. `cost` 无模型单价时为 `NULL`/unknown；cost 为零是已知零，不得与 unknown 合并。
3. conversation source 只有 `proxy`、`sessionScan`、`off`；`off` 不做归属，`proxy` 无显式 ID 归入 `unlabeled`，`sessionScan` 使用统一 matcher。
4. 显式非空 conversation ID 永远优先；matcher 歧义时返回 `unlabeled`，不得扩大时间窗口或猜测。
5. Stats GET 是读取路径，不执行 legacy log migration 写入；迁移由启动 worker 或显式迁移入口负责。
6. session title 只使用 matcher 采用的 snapshot：`session_info.name`，其次第一条真实 user message，再其次 cwd basename；不存在则使用 session ID。
7. display percent-decode 只在展示 boundary 兼容存量编码，不改变落库 request facts。

### 2.6 WebUI 编辑器与布局

1. Config JSON 始终是可直接编辑的受控文本；Format 按钮固定在编辑器左下；structured view 不能替代它。
2. JSON parse/semantic error 只标记真实错误位置；合法字段不得因为 spellcheck、Grammarly 或默认样式出现红线。
3. canonical draft 的 parsed value 是业务事实；raw JSON 只在 parse error 时保留未提交文本；selection 和 preview 从 canonical value 派生。
4. Page 只负责页面底色，Card 负责 surface/border，Table 默认透明并继承 Card，sticky cell 不另造冲突 surface。
5. viewport contract 固定覆盖 375、768、900、1024、1440；组件按内容最小宽度选择 breakpoint，不把所有组件绑定到同一设备断点。
6. 长 model ID、Supplier 名和 badge 必须遵守 `min-w-0`、wrap、break-all policy；业务状态不因视觉调整改变。

### 2.7 Daemon

1. 生命周期至少区分 `Absent`、`Spawning`、`Healthy`、`AliveButUnhealthy`、`StalePid`、`UnmanagedListener`、`Stopping`、`Stopped`、`Failed`。
2. 启动顺序必须是 spawn → 保存真实 PID → health check → 成功后 release；health 失败时 kill 子进程并清理 PID 文件。
3. PID 文件只能定位受管理实例；`running` 必须同时满足 PID identity 与目标端口 health。
4. 固定 pid 文件只支持一个受管理 Proxy 和一个受管理 WebUI；多实例不是本期能力，Status 必须明确提示 unmanaged/multiple listener。
5. stop 是幂等的：实例已退出、PID stale 或 PID 文件缺失都不能误杀其他进程，也不能报告仍在运行。

## 3. IMP-01～IMP-05 追踪矩阵

| 不变量 | 规范证据 | 后续工作包 | 直接回归测试 | 真实验收 |
|---|---|---|---|---|
| Supplier/Channel/Model/exposed model 只由 config.json 负责 | §1 Supplier/Channel/Model | IMP-05；后续 IMP-06/08 | config ownership 与 Supplier CRUD tests | config.json、Gateway preview、route 三者对照 |
| Gateway 只经显式 publish 写入 models.json | §1 Gateway；§2.3 | IMP-05 | preview/publish boundary tests | Supplier/settings 变更后 models.json 不变 |
| session files 只读，不能持久化 ephemeral 会话 | §1 pi session | 后续 IMP-09 | session scan fixture tests | 真实 session 目录副本与 `--no-session` 请求核对 |
| SQLite row 保存 immutable request facts | §1 请求事实；§2.5.1 | 后续 IMP-10/11 | store fact immutability tests | Stats 展示不改写落库 row |
| daemon 状态由 managed identity 与 health 共同决定 | §1 daemon；§2.7 | 后续 IMP-13 | lifecycle/status tests | 真实 PID、端口和 Status 三者一致 |
| WebUI 只有一个 canonical draft，JSON 始终可编辑 | §1 WebUI；§2.6.1/3 | 后续 IMP-06/08/12 | draft/editor component tests | 浏览器直接编辑、Format 左下、保存重载 |
| profiles/upstreams 缺失按 boundary migration，不解释为全部暴露 | §2.2 profiles/upstreams/exposedModels | IMP-05；后续 IMP-07/10 | config decoder/migration tests | 旧 config 读取后 shape 明确且只迁移一次 |
| exposedModels=[] 是零暴露 | §2.2 exposedModels:[] | IMP-05 | gateway、`/v1/models`、route tests | 实际列表、Gateway provider 和 route 均为空 |
| exposedModels=null 是非法，不转成 [] 或全部 | §2.2 exposedModels:null | IMP-05；后续 IMP-07 | validation error path tests | API 保存前返回字段错误且无文件写入 |
| model id 缺失/null/空字符串不发布、不路由 | §2.2 model id | IMP-05；后续 IMP-07/08 | model validation tests | Gateway 和 `/v1/models` 无空 ID |
| 无模型 metadata/context/maxTokens 不伪造，三个 max key 不重写 | §2.2 model metadata；§2.4.5 | IMP-02 | clamp nil/zero metadata tests | 无 metadata 的真实请求保持客户端 max 值且不触发本地伪造 |
| models 文件不存在时 current 为空，publish 创建目录并原子写 | §2.3 models file | IMP-05 | golden/atomic write tests | 真实 models.json 副本首次 publish 可被 Pi 解析 |
| providers 缺失/null 不删除不存在的第三方 provider | §2.3 providers | IMP-05 | current-empty/preservation tests | 真实第三方 provider 字节/结构保留 |
| fixed provider 无 exposed model 时不保留 stale entry | §2.3 fixed provider | IMP-05 | stale provider cleanup tests | 删除模型后重新 preview/publish 不复活 |
| compat/headers/extra 由 canonical merge 保留 | §2.3 extras | IMP-05；后续 IMP-06 | extra preservation tests | publish 后再次 preview pending 为 0 |
| conflict/validation 非空时零写入 | §2.3 conflict | IMP-05 | failure atomicity tests | 冲突前后 models.json 字节不变 |
| 同一 canonical plan 连续 publish 幂等 | §2.3 idempotency | IMP-05 | golden/idempotency tests | 第二次 publish 后 `pending_count=0` |
| responsesMode 只按声明 api 决定 passthrough/convert | §2.4.1/2 | IMP-04 | table-driven PlanRequest tests | Responses/Chat provider 实际 endpoint 与事件语义一致 |
| 不兼容组合发送 upstream 前失败，不探测/降级/failover | §2.4.3 | IMP-04 | preflight rejection tests | upstream 捕获不到不兼容请求，客户端得到明确错误 |
| 三个 max key、/3、encrypted compensation、safety、16 floor、maxTokens 只有一套 | §2.4.4/5；§2.4.6 | IMP-02 | limit/server retry/stream tests | 长会话真实 upstream 不触发已知 context 400 |
| 首次、stream、retry 共享 URL/header/affinity/UA builder | §2.4.4；追踪矩阵 outbound | IMP-03 | httptest 完整 header 比较 | opencode.ai 实际收到 affinity、UA、channel headers |
| reasoning 是 completion 子集，cached/reasoning 缺失/零/已知可区分 | §2.4.6；§2.5.2 | IMP-04；后续 IMP-11 | usage table/stream tests | Stats token/cost 与 upstream usage 对照 |
| conversation source、显式 ID 优先、歧义不猜测 | §2.5.3/4 | 后续 IMP-09 | matcher table/concurrent fixture tests | 真实 session 目录中同模型并发归入 unlabeled |
| Stats GET 只读 SQLite，不触发 legacy migration 写入 | §2.5.5 | 后续 IMP-10/11 | handler read-only/concurrency tests | daemon 启动健康且首次 Stats 不同步导入 |
| session title 与 matcher snapshot 使用同一优先级 | §2.5.6 | 后续 IMP-09 | JSONL title fixture tests | Stats 标题与真实 pi session name/user/cwd 一致 |
| percent-decode 只在 display boundary，不能改写 facts | §2.5.7 | 后续 IMP-09/11 | storage/display separation tests | 存量编码显示正确，SQLite 原值不变 |
| JSON 错误只标真实位置，合法内容无 spellcheck/默认红线 | §2.6.2 | 后续 IMP-06/07/12 | JsonEditor/validation tests | 浏览器实际错误行和合法模型 ID 视觉核对 |
| parsed value 是 canonical draft，selection/preview 从它派生 | §2.6.3 | 后续 IMP-06/08 | reducer/component tests | 选择、JSON、structured preview、publish payload 一致 |
| Page/Card/Table/sticky surface 层级唯一 | §2.6.4 | 后续 IMP-12 | viewport/screenshot tests | 375/768/900/1024/1440 无黑块和覆盖 |
| breakpoint 与 overflow 遵守组件内容约束 | §2.6.5/6 | 后续 IMP-12 | responsive interaction tests | 长 model ID/Supplier/badge 无溢出和重叠 |
| daemon 生命周期区分 Absent/Spawning/Healthy/AliveButUnhealthy/StalePid/UnmanagedListener/Stopping/Stopped/Failed | §2.7.1 | 后续 IMP-13 | state transition tests | 真实 start/status/stop 结果与端口状态一致 |
| spawn 后先保存真实 PID，health 失败 kill 并清理 | §2.7.2 | 后续 IMP-13 | process lifecycle tests | PID 文件 PID 等于真实子进程，失败无残留 |
| PID 文件不是 running 证明，stop 幂等且不误杀 | §2.7.3/5 | 后续 IMP-13 | stale PID/port conflict tests | stale、重复 stop、端口冲突实际结果正确 |
| 固定 pid 文件只支持一个受管 Proxy/WebUI，多实例明确提示 | §2.7.4 | 后续 IMP-13 | multiple-listener tests | Status 提示 unmanaged/multiple listener |

## 4. 变更纪律

- IMP-02～IMP-05 必须逐项引用本 contract 中的条款和矩阵行。
- 新增实现不得引入第二套 clamp、request builder、conversation matcher 或 Gateway projection。
- 任何修改 contract 的行为变更必须先更新本文件、对应测试矩阵和必要 ADR 草稿，再修改运行时代码。
- 本文件不替代运行时 validation；它是测试和 code review 的判定基线。
