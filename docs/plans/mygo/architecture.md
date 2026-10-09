# MyGo 迁移的既有架构、技术依赖与数据约束

> 这是所有 WP 的共享前提，而非需要独立交付的工作包；执行任何 WP 前先核对最新 [system-contract](../../system-contract.md)。

## 1. 真实代码边界及预期架构

### 1.1 已有事实来源与入口（迁移时不可复制）

| 领域 | 当前权威入口 | 桌面端对接方式 |
| --- | --- | --- |
| 供应商/渠道/模型 | internal/config、internal/profile | 复用 Profile 服务，前端只消费 DTO |
| 能力判定 | internal/protocol | 唯一能力判断，不维护桌面版枚举 |
| Gateway 计划/发布 | internal/gateway | 复用 canonical preview / PublishPlan |
| 代理、SSE、取消 | internal/server、internal/translator、internal/limit | **HTTP Proxy 继续独立监听**，桌面 UI 不拦截推理流 |
| 请求事实与聚合 | internal/store、internal/stats | 复用 requests.db，统计不重算/改写历史行 |
| 会话发现 | internal/scan、internal/conversation | 只读 Pi sessions、延续当前归属规则 |
| 进程管理 | internal/daemon | 以真实进程 identity + health 判定，不能只看 PID 文件 |
| WebUI 当前契约 | webui/src/api.ts、apiSchema.ts | 保留单一契约；迁移时不得偷偷改返回结构 |
| CLI、TUI | cmd/pi-switch、internal/tui | 保持原有二进制/非 GUI 入口 |

权威文件：[架构导航](../../architecture.md)、[系统契约](../../system-contract.md)、[ADR 目录](../../adr/)。旧的 [manual-test-basic.md](../manual-test-basic.md) 部分 CLI 描述/模型 ID 示例可能过时；执行测试时以当前代码、system-contract 与最新 API 契约为准，不复制旧示例中的路径或斜杠模型 ID。

### 1.2 推荐目标拓扑

~~~text
MyGo Desktop (desktop/, 独立入口 / 生命周期)
  ├── 原生系统层：window / tray / dialog / shortcuts / notifications
  ├── 页面层：首期 React WebView，之后逐页评估 MyGo ui
  └── Desktop adapter：只做界面相关调用、类型映射与事件订阅
                      │
CLI / TUI ────────────┼──────> 共享 Go application services
Mgmt HTTP API ────────┤             │
                      │             ├─ profile / config / protocol / gateway
                      │             ├─ catalog / piagent / store / stats / scan
                      │             └─ daemon / health
Pi ── HTTP :43112 ──> 独立 Proxy Router ──> translator / limit ──> 上游
                                           │
                                          requests.db
~~~

**严格边界**：

- 桌面启动/退出和代理启动/停止是两套不同状态机。关闭所有窗口、点击“退出桌面”**不会隐式停止已运行的 Proxy**；只有显式“停止代理”才停止由 pi-switch 管理且身份匹配的代理。
- 已存在 Proxy/WebUI daemon 时先探测、验证身份与健康度，不因端口被占用就杀进程；不属于本程序的监听者不接管。
- 管理层可阶段性通过本机现有管理 API 工作；若改用 MyGo Bind/自定义协议，应调用**同一业务 service**，并明确鉴权、origin、事件取消与错误格式。禁止形成“REST 一套规则、桌面 RPC 一套规则”。
- 保留默认回环地址/现有外网绑定密码要求；MyGo 自定义页面 origin、远端页面不能随意调用本机 Go Bind；密钥必须按现有脱敏语义处理。
- 需要 app service 抽取时，以最小接口和现有单元测试驱动，不为桌面功能把 internal/server 重构成另一个大框架。

### 1.3 持久化、兼容与数据安全

以下路径原则上维持原样：~/.pi-switch/config.json、~/.pi-switch/requests.db、~/.pi-switch/cache/models-dev.json、~/.pi-switch/backups/、~/.pi/agent/models.json、~/.pi/agent/sessions/。MyGo 的用户配置目录仅能保存**窗口偏好和桌面专属状态**，不得存第二份 provider 配置或请求账本。

**不可破坏的不变量**：

1. config.json 是 Supplier/Channel/Model/exposedModels 唯一事实；exposedModels 缺失、null、空数组的区分遵守 system-contract；空暴露集不代表暴露全部。
2. Gateway current/proposed/diff 必须来自后端 canonical plan；改变供应商数据不会自动发布 Pi models.json。仅用户显式 Apply to Pi 才原子写入，保留第三方 provider。
3. Generated 与 Draft 元数据补齐语义不同：前者按当前规则刷新（FillOverwrite），后者保留显式草稿字段（FillMissing），包括已声明的零价格。
4. Proxy 仅使用裸 model ID；零匹配与歧义匹配继续拒绝，带“/”的旧模型形式不能因 UI 迁移而重新放行。
5. Responses/Chat/Anthropic、stream/SSE、tool calls、取消、usage unknown 和 cost unknown 均不改变协议及统计含义；请求历史不可被 UI 展示重算。
6. pi sessions 只读；显式 conversation ID 优先，sessionScan 按当前 matcher，未知归属不能臆造。
7. 保存前备份、原子写入、坏配置诊断、非成功请求的错误信封及权限边界保持原有合同；禁止在日志、崩溃报告、截图和测试夹具中泄露 Key。
8. 任何持久化格式更改必须配带测试夹具、旧版本读取/失败策略、备份与还原测试，未经批准不得自动不可逆迁移。

## 2. 依赖及技术验证（必须先过 Gate A）

### 2.1 工具链与模块划分

- 调研时 pi-switch 根模块 go.mod 为 **Go 1.24.2**，MyGo 上游 go.mod 为 **Go 1.27.1**；直接往根模块加 MyGo 会提高整个 CLI/Proxy 的最低构建工具链要求。
- **优先实验单独的 desktop/ Go module**：隔离 GUI 依赖与 Go 工具链，桌面包使用同一仓库中的共享 Go core（评估 internal 包导入边界、replace / workspace、版本发布和 CI 可重现性）；只有实测技术与运维成本可接受，才能固定该方案。
- 不能在验证前承诺“CLI Go 1.24.2 不变”与“MyGo Go 1.27.1 可用”必然同时满足；若模块隔离不成立，提交 ADR 对比升级整个仓库、单独桌面仓库、暂缓 MyGo 三种选择。
- 固定 MyGo **release tag + go.sum/lockfile（或具体 commit）**，不得在发布 CI 使用 @latest 或静默漂移依赖。0.x 升级要求单独兼容性验证。
- 原生 UI 路线应当允许 CLI/Headless 在无 GTK/WebKitGTK 环境继续构建与运行；桌面包在支持的平台验证 native-window 与 WebView 两类行为。MyGo native UI 不需要 WebView，但 Linux 桌面窗口仍涉及系统图形栈。

### 2.2 平台疑点验证

桌面支持目标为 Linux amd64/arm64；Windows 和 macOS 不属于桌面支持范围，macOS CLI/npm 支持不变。

| 平台 | 必测项 | 明确限制 / 回退 |
| --- | --- | --- |
| Arch Linux + niri / Wayland | 窗口/关闭/焦点、fractional/整数 scale、3200×2000 scale 2、中文 fcitx5、复制粘贴、键盘导航、托盘、深浅色 | Wayland 全局快捷键需 portal/桌面支持；不可依赖全局坐标定位 |
| 其他 Linux 桌面（GNOME/KDE 或 X11） | WebKitGTK 4.1/4.0 fallback、GTK、字体渲染、桌面菜单、启动器、依赖缺失报错 | Linux MyGo Tray 需要 libayatana-appindicator3 且只支持菜单，不保证 tray click |

- 用真实物理桌面或 CI GUI runner 证明图形功能；“交叉编译成功”**不等于**“窗口、托盘、输入法能用”。
- 提前记录动态库清单、构建可用目标与加载失败提示。Linux tray 不可用时窗口和应用菜单必须仍可操作，不能让应用启动失败。
- 应评估 MyGo ui 对多行/JSON 文本、高亮、大数据表格、统计图、对话浏览器及无障碍操作的真实可行性。未经原型和测试，不删除 React 页面。
