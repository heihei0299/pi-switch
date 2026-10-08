# pi-switch → MyGo 桌面化/原生 UI 重写：执行与验收计划

> 状态：**计划基线（尚未开始实现）**；分支：**mygo**；制定日期：2026-10-08。
>
> 任务范围：本次提交**只记录执行与验收计划**，不改业务代码、不变更现有发布渠道、不创建 PR、不合并 main。
>
> 基线：pi-switch main = **18f877e60ef62e294bea8ad65bfebc5838ee96c1**（2026-10-03）；MyGo 上游为 [egoist/mygo](https://github.com/egoist/mygo)，调研时可用发布版本 **v0.3.3**（2026-10-08）。**版本是调研快照，不构成已锁定或已验证的依赖**。

## 0. 目标、边界、路线

### 0.1 项目目标

1. 为 pi-switch 增加 Windows、macOS、Linux 桌面入口：独立窗口、单实例、托盘菜单、系统通知、安装包和受控退出。
2. 保留已存在且经验证的 **Go 业务核心**，不重写 Proxy/Responses/Chat/Anthropic 转换、Gateway 发布、模型元数据、Token/费用、SQLite 统计、会话匹配、配置持久化。
3. **CLI / TUI / Headless Proxy 在没有图形环境时仍可独立运行**；桌面应用绝不能成为 Pi 模型调用的必要依赖。
4. 首先实现“桌面端与现有 WebUI 功能等价”；其后通过独立的原生 UI 技术验收决定是否把 React/Vite/WebView 全部替换为 MyGo ui。**完全原生是有条件的后续目标，不是未经验证的前提**。
5. 新旧版本读取同一用户数据，不产生第二套配置或互相覆盖；需要改变数据格式时必须单独制定可回滚迁移方案。

### 0.2 确定不做

- 不更改对外 API 请求格式、模型裸 ID 路由规则、两套固定 Gateway provider（pi-switch-res / pi-switch-chat）及显式发布语义。
- 不恢复已休眠的 failover 调度，不拓展供应商/协议功能，不顺手做无关重构。
- 不直接删除 CLI、TUI、npm 包、浏览器 WebUI 或已有发布工作流。
- 不自动接管用户已有的 daemon、修改用户私有 session 文件或收集额外遥测。
- 不以“UI 能打开”代替协议、数据、运行时、升级及安全验收。

### 0.3 路线与决策门

- **里程碑 M1 / 验证封装**：MyGo + 现有 React 页面（WebView）+ 现有 Go 服务；证明平台依赖、生命周期与管理数据访问可用。该里程碑是实验验证，不能称为完成重写。
- **里程碑 M2 / 可发布桌面版（优先）**：桌面外壳、系统功能、服务边界、构建、升级、诊断具备验收证据；复杂页面允许暂时保留 React。老 WebUI、CLI/TUI 保持可用。
- **里程碑 M3 / 可选完整原生重写**：所有可见页面（尤其 JSON 编辑器、Gateway diff、Stats 图表/会话浏览器）经过能力、输入法、可访问性和等价测试后，才能提出移除 React/Vite/WebView 的独立工作包。若任何关键条件不满足，停在 M2，不为追求“纯 Go”牺牲功能。

每个里程碑必须有“通过/未通过/豁免”记录；豁免需明确影响、替代路径和批准人，不能以“基本可用”作为门槛。

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

权威文件：[架构导航](../architecture.md)、[系统契约](../system-contract.md)、[ADR 目录](../adr/)。旧的 [manual-test-basic.md](../manual-test-basic.md) 部分 CLI 描述/模型 ID 示例可能过时；执行测试时以当前代码、system-contract 与最新 API 契约为准，不复制旧示例中的路径或斜杠模型 ID。

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

| 平台 | 必测项 | 明确限制 / 回退 |
| --- | --- | --- |
| Arch Linux + niri / Wayland | 窗口/关闭/焦点、fractional/整数 scale、3200×2000 scale 2、中文 fcitx5、复制粘贴、键盘导航、托盘、深浅色 | Wayland 全局快捷键需 portal/桌面支持；不可依赖全局坐标定位 |
| 其他 Linux 桌面（GNOME/KDE 或 X11） | WebKitGTK 4.1/4.0 fallback、GTK、字体渲染、桌面菜单、启动器、依赖缺失报错 | Linux MyGo Tray 需要 libayatana-appindicator3 且只支持菜单，不保证 tray click |
| Windows 10/11（amd64） | WebView2 环境、窗口与托盘、单实例、关闭保活、升级/卸载、IME、高 DPI | 系统组件缺失需可诊断，不假定打包自带浏览器 |
| macOS（arm64，可能加 amd64） | WKWebView、托盘、菜单、通知、签名/公证、睡眠恢复 | 分发签名/公证缺少凭据时标记阻断，不伪称可正式分发 |

- 用真实物理桌面或 CI GUI runner 证明图形功能；“交叉编译成功”**不等于**“窗口、托盘、输入法能用”。
- 提前记录动态库清单、构建可用目标与加载失败提示。Linux tray 不可用时窗口和应用菜单必须仍可操作，不能让应用启动失败。
- 应评估 MyGo ui 对多行/JSON 文本、高亮、大数据表格、统计图、对话浏览器及无障碍操作的真实可行性。未经原型和测试，不删除 React 页面。

## 3. 工作包、顺序与交付物

**工作约束：一个独立工作包/issue 对应一次交付 commit；一个明确 bug 单独 commit；先验收再 commit；不要把多个未验证修复混成一个 commit；commit 不等于合并。** 每个 PR 附运行命令、结果、未验证项和风险；每工作包独立 code review，最后执行一次全量验证。

### WP-00 — 准入基线与已知失败隔离（依赖：无）

- 记录主分支 SHAs、当前 Go/Node 版本、行为基线、页面与 API 清单、已有测试和平台构建结果。
- **先修或明确标注 main 继承的 CI 失败**：2026-10-03 的最新 CI 运行 37118224938，verify/job 的 TestProviderCLI_ReadmeCommandsAreRecognized 因 README 命令示例匹配测试失败；跨平台 binary build 成功。此失败不归责 MyGo，但在 M2 发布前必须修复并重跑 CI，修复应另立一事一 commit。
- 更新使用说明中与代码不一致的 CLI/模型 ID 手动测试脚本（如需要，独立工作包），并建立脱敏基准测试数据集。
- **验收**：现存 bug、预期失败、测试日志和基线版本可复现；迁移失败与继承失败有清楚分界。

### WP-01 — MyGo 最小技术 Spike（依赖：WP-00）

- 提供独立 Hello Window、原生小控件页、WebView 加载现有静态页面、Go Bind demo、单实例、退出与轻量性能采样。
- 同时验证 desktop module 与根模块的 Go 版本隔离；写下环境依赖、交叉编译命令、MyGo 版本锁定及风险结论。
- 演示 Linux niri 的窗口、中文输入和缩放；Windows 核对 WebView2；无托盘依赖时可启动。
- **验收 Gate A**：至少 Linux Wayland + Windows GUI 实机演示成功；Go headless 不受 GUI 依赖影响；Bind 的类型/错误/取消符合需求。任何失败按记录决定替代路线，不进入批量改 UI。

### WP-02 — Desktop Shell 与代理生命周期（依赖：Gate A）

- 建立桌面独立入口和图形窗口、托盘菜单、显示/隐藏、单实例、启动最小化选项、退出动作。
- 复用已有 daemon/proxy start/stop/status 与 health；避免重复拉起及 PID 假阳性；窗口退出不停止已存在代理，应用崩溃不影响独立守护进程。
- 仅向桌面写入非敏感窗口偏好，检测端口冲突、缺失库和进程身份异常并提供可行动错误提示。
- **验收**：AT-08、AT-09、AT-11、AT-13；代理服务状态有确定性测试且不误杀其他监听进程。

### WP-03 — 管理能力复用/桌面适配器（依赖：WP-02）

- 先复用 internal/server 的管理 HTTP 契约；评估将逻辑抽成最小共享 application service 与 MyGo Bind。
- 逐项追踪 desktop action → application service → config/gateway/store，杜绝 UI 直接操作 config.json/models.json/requests.db。
- 为网络可取消请求和桌面调用添加上下文取消、错误映射及授权测试；兼容旧 API Schema decoder，新增契约测试。
- **验收**：AT-02～AT-07、AT-10；桌面与 HTTP 在相同夹具下产生相同事实/结果；浏览器页面无法越权调用 GUI 私有操作。

### WP-04 — M1 React 页面承载与 M2 核心功能等价（依赖：WP-03）

- 原有 WebUI 在 MyGo 窗口内可用；维护 Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 全部功能。
- 显式校验双态草稿、JSON 编辑、Gateway current/proposed/diff、过滤/分页、请求取消、错误提示和长列表。
- 桌面页面可以阶段性调用 loopback 管理服务，但应登记 origin/鉴权/CSRF 方案；只有通过安全门槛才能交付。
- **验收 Gate B**：与同版本浏览器 WebUI 的行为回归清单 100% 逐项核对；无 P0/P1 缺陷；桌面有独立启动/退出和守护进程测试。

### WP-05 — 原生 UI 简单页面（依赖：Gate B）

- 评估 MyGo ui 组件架构、状态更新、列表虚拟化、快捷键、屏幕阅读器，逐页迁移 Home、设置概要、Proxy 状态/控制、基本 Profiles 列表。
- 每个页面在完成等价自动/手动测试后才替换默认入口；原 WebView 页面保留退路。
- **验收**：同功能、同错误与空状态、键盘完全可操作、中文 IME 和高 DPI 无退化，测量性能并留截图/视频。

### WP-06 — 原生 UI 复杂页面及 M3 决策（依赖：WP-05）

- 分别研究 JSON 文本编辑器（语法/校验/撤销/选区/输入法）、Gateway 模型草稿和 diff、Stats 图表与会话树/分页、Packages 导入/移除。
- 对每一块做“组件能力、可访问性、速度、维护成本、可回滚性”的书面结论。
- **验收 Gate C**：如任一关键功能不及现有 WebUI，保留混合架构并明确原因；全部页面等价且跨平台通过时，才批准移除 React/Vite/WebView 的独立变更。只迁移完成的页面不得强迫用户使用不完整 Native UI。

### WP-07 — 构建、发布、升级、诊断与文档（依赖：Gate B；M3 可随后追加）

- 桌面构建和 CLI/npm 分发分离：版本与 release 身份可追溯，桌面安装包/依赖说明完整；Windows、macOS、Linux 构建及实机安装验收。
- 不破坏现有 .github/workflows/ci.yml 的 CLI 6 目标矩阵；桌面另加 build/test matrix，并确保桌面失败不会误发 npm。
- 更新用户说明、开发手册、已知限制、安全边界与升级回滚步骤。正式签名与公证由受控凭据完成，缺少时明确仅内部/测试版本。
- **验收 Gate D**：AT-14～AT-18；旧版和新版的配置/请求记录均可验证保留，禁用或卸载桌面不会破坏 CLI/Proxy。

### WP-08 — 可选老前端退役（必须：Gate C + Gate D）

- 独立提案列出将删除的依赖、文件、文档和发布影响；对 CLI/npm 浏览器可用性的保留策略先做决策。
- 先证明等价并更新测试/构建/升级，再删除不再需要的 React/Vite/WebKitGTK 依赖；不得先删后补。
- **验收**：所有旧契约与跨平台测试保持通过，下载/启动/回滚路径无断裂，PR 中列出对用户的实际破坏性变更和迁移说明。

## 4. 验收用例（逐项记录证据）

下列 AT-* 是必须维护的验收 ID。每个记录包含：环境（OS/架构/桌面/显示 scale/Go/MyGo 版本）、操作与夹具、预期、实际、日志或测试链接、pass/fail/block、操作者、日期。涉及 API Key 的截图/日志全部脱敏。

| ID | 测试方式 | 操作与可观察断言 |
| --- | --- | --- |
| AT-01 | CI + CLI | 原有 Go、WebUI、Playwright、smoke、跨平台构建通过；记录并单独关闭继承的 README 测试失败 |
| AT-02 | Go contract + desktop | 新建/更新/复制/删除供应商，含无渠道、多个渠道、非法 API、空暴露集；磁盘 config 语义与旧服务一致；desktop 不直接修改 config |
| AT-03 | Go contract + desktop | 获取/测试模型、手动模型、暴露/取消暴露；未知与歧义路由仍拒绝；任何界面操作不会“默认全部暴露” |
| AT-04 | 单测 + temp paths | Gateway generated/draft 预览、metadata 0 值、diff、conflict、只选部分；发布前 models.json 字节不变，显式发布后原子更新、第三方 provider 保留；失败不产生半成品 |
| AT-05 | HTTP integration | Pi 从 :43112 调用裸模型 ID；Responses、Chat Completions、Anthropic 的适用模型按现有 protocol 能力路由；旧斜杠形式拒绝；错误结构匹配合同 |
| AT-06 | HTTP streaming | SSE 分块顺序、tool call、非流式、上游错误、取消/断流、超时；不会重复返回或丢失已接收用量事实 |
| AT-07 | SQLite + GUI | 每个请求只写一条历史；usage/cost/unknown、时间窗、会话归属/分页与旧版一致；历史事实不被迁移重写；session 目录只读 |
| AT-08 | 进程集成 | 窗口关闭→已运行 Proxy 仍可 :43112/healthz 且能服务请求；“退出桌面”不杀代理；显式“停止代理”仅作用于正确身份；GUI 重启不重复创建 |
| AT-09 | 进程集成 | 端口被无关程序占用、旧 PID、损坏锁、崩溃、双实例、用户登录启动；不得误杀/接管，不以 PID 存在伪称 healthy |
| AT-10 | 安全测试 | 外部 URL/普通浏览器不能调用私有 MyGo Bind；本机管理 API 的回环/非回环鉴权合同保留，拒绝恶意 origin 与未授权写入；Key 不进入日志/状态快照/错误弹窗 |
| AT-11 | Linux 实机 | Arch+niri/Wayland：打开/隐藏、焦点、菜单、粘贴、fcitx5 中文组合与候选、3200×2000@scale2 的缩放/点击位置/滚动；无残影、文字截断、候选错位 |
| AT-12 | Windows/macOS 实机 | Windows 10/11 WebView2/IME/高 DPI/单实例；macOS WKWebView/托盘/睡眠恢复/签名状态。每个受支持目标都运行可交互 GUI smoke，缺设备标记 block，不拿交叉编译充数 |
| AT-13 | Linux 降级 | libayatana-appindicator3 缺失、无 system tray、WebKitGTK 缺失/版本不匹配、Wayland portal 不支持快捷键：应用不死锁，可找到替代入口，错误可读；托盘仅菜单模式 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |
| AT-16 | 安装与回滚 | 干净系统安装、旧版本升级、备份/恢复、卸载桌面、恢复旧 CLI；config/models.json/requests.db 未丢失、Key 未泄露、daemon 不遗留异常进程 |
| AT-17 | 依赖与秘密 | 固定依赖/锁文件、依赖许可证、敏感信息扫描、打包内容检查、构建身份与 commit 可追溯；未经签名的发行物明确标注不可正式分发 |
| AT-18 | CI/发布隔离 | main 的原六目标 CLI build 不退化；desktop 分离构建并在相应平台跑可执行 smoke；tag/npm 发布条件不被 desktop PR 误触发 |

**安全测试方式**：以临时文件隔离 PI_SWITCH_CONFIG、PI_SWITCH_CONFIG_DIR、PI_SWITCH_DB、PI_SWITCH_MODELS（四个变量同时设置），启动前验证读取的是临时空配置，完成后检查真实用户目录未变化。禁止将真 API Key、现有工作目录的 secrets 复制到 CI 或 public issue。

## 5. 验证命令与 CI 门槛

以下是**执行阶段应运行的命令清单**，不是本计划提交已运行的测试。若脚本/工具链变化，记录新命令与原因，不能静默跳过。

### 5.1 既有核心基线（仓库根目录）

~~~bash
go version
node --version
npm install
npm run build:webui
go vet ./...
PI_SWITCH_DB="$(mktemp -u)" go test ./...
npm --prefix webui run typecheck
npm run test:webui
cd webui && npx playwright install --with-deps chromium && cd ..
npm --prefix webui run test:e2e
bash scripts/check-secrets.sh
npm run build:go
bash scripts/functional-check.sh bin/pi-switch
bin/pi-switch --build-info
~~~

受限机器可以用仓库自带 <code>bash scripts/test-limited.sh</code> 跑 Go/WebUI/TS/smoke 套件，但仍需另跑 CI 中的 vet、Playwright、secret scan、构建身份验证；该脚本不能替代完整 CI。CI 当前 Go 测试失败项应按 WP-00 单独解决后才把全绿作为实施前置。

**注意**：上面 PI_SWITCH_DB 的一次性空路径只用于测试套件；写入配置或触发 daemon 的集成实验必须使用 AT 测试专门目录并同时设置四个隔离环境变量，不能仅靠 PI_SWITCH_CONFIG_DIR。

### 5.2 新增桌面验证（WP-01 验证通过后确定并落库）

建议独立桌面模块，以实际创建的命令和依赖版本为准：

~~~bash
# 示例路径/任务，不表示 desktop/ 目前已经存在
go -C desktop version
go -C desktop test ./...
go -C desktop vet ./...
# mygo dev/build：使用锁定版 CLI，记录精确调用参数和输出目录
# 每个目标平台：编译产物 + 安装包内容校验 + 真机 GUI smoke
~~~

- 新建 GUI/IPC 测试：服务调用 DTO、错误/取消、安全 origin、单实例和窗口生命周期；
- 新建原生 UI 测试（如果选择 M3）：页面事件、键盘焦点、长列表、草稿编辑、无障碍；
- 新建跨进程回归：GUI 与 Proxy/daemon 在相互崩溃或退出后仍符合所有权约定；
- 新增主分支保护前先检查 CI 的触发规则：现有 workflows 仅对 main push、面向 main PR 和 workflow_dispatch 运行；在 mygo 分支的普通 push 不会自动获得完整 CI 证明。

### 5.3 最终 PR 验收顺序

1. 单一工作包的新增/改动测试（必过），再执行对应核心回归（必过）。
2. code review：确认未复制业务规则、无密钥/未授权文件读取、遵守系统契约。
3. 完成工作包的验证后提交一个 commit；报告 commit SHA、确实执行的命令、失败与未验证项。
4. M2 候选走全量 Go + WebUI + Desktop + CI cross build + 安全审计 + 至少 Linux/Windows 真机 GUI 验收。
5. 主分支全量测试和评审通过后**再单独决定**是否合并；不得自动合并、推送额外分支或发布 npm。
6. M3 删除旧栈之前另做完整一轮行为、性能、用户数据迁移与回滚验证。

## 6. 失败处理、回滚与发布门

### 6.1 停止条件

以下任一条件出现，阻断对应里程碑：

- 配置或请求历史丢失；第三方 Pi provider 被覆盖；Gateway 发生隐式发布。
- Proxy 被关窗意外杀死，或者 GUI 管理误杀别的进程/端口。
- API Key 泄露到日志、IPC 不可信 origin、远程管理接口绕过密码。
- SSE/usage/session/错误信封任一协议合同破坏，且无明确批准的兼容决策。
- Windows 或 Linux 必需目标 GUI 无法交互；无法确认输入法与基础键盘操作。
- 一项跨平台失败被误判为“本地能编译”；旧 CLI/npm 升级路径未经验证。

### 6.2 回滚设计

- 桌面应用与 CLI 构建/安装路径分离；试验版本优先使用隔离的 app ID 与安装路径，不覆盖生产可执行文件。
- 所有数据迁移前备份；只在明确标记且通过旧版对照的情况下改写现有文件；记录恢复命令/步骤并做实际恢复测试。
- 回滚优先卸载或禁用桌面层、继续使用当前 CLI/TUI/WebUI/Proxy；保持旧二进制可读取的数据格式。
- 若必须升级数据结构，发布前增加 downgrade 测试或明确的导出/恢复工序，未通过不得发布。
- 不用 git reset --hard / force push / 清空配置等破坏性操作作为常规回滚手段；代码回退通过新提交或受控 revert 流程。

### 6.3 对外交付条件

- M2：至少功能清单等价、CI 完整通过、受支持平台实测、安装/卸载/回滚可用、安全和数据完整性通过，文档准确披露托盘、WebView 和 Wayland 限制。
- M3：在 M2 所有条件上追加完整原生页面等价、组件维护能力、性能、可访问性及替代旧 WebUI 分发策略审查。
- 无签名、公证或可信安装链时，产物只能标记开发/测试，不得宣称正式可安全分发。
- 确认已知主分支测试失败解决、mygo branch PR 获完整 CI 结果并完成人工验收之后，才有合并/打 tag/发布资格；任何提交不会隐含自动发布授权。

## 7. 必须留存的实施记录

每个工作包/issue 至少包含：

~~~text
WP/Issue：WP-xx / #xx
目标及不在范围内的事项：
前置条件 / 基线 SHA / MyGo 锁定版本：
修改文件及架构边界：
具体测试命令（实际执行）：
验收 AT-*：PASS / FAIL / BLOCK（含链接）
跨平台结果：Linux Wayland / Windows / macOS（未测必须写明）
安全与数据兼容检查：
code review 结论：
遗留风险与回滚步骤：
交付 commit SHA（每工作包一次；不含无关变更）：
是否提议合并：否 / 待批准
~~~

验收证据可以保存为独立 docs/test-reports/ 文档、CI artifact 或 issue 评论，**不得包含真实 API Key 或任何敏感文件**。不复用旧会话的隐含结论；后续实现首先读取本计划和最新 system-contract，若发现冲突先新增 ADR 决策，不悄悄更改合同。

## 8. 参考

- 当前 pi-switch：[架构导航](../architecture.md)、[系统合同](../system-contract.md)、[WebUI 使用说明](../../WEBUI_GUIDE.md)、[工作流](../../.github/workflows/ci.yml)、[测试脚本](../../scripts/test-limited.sh)。
- MyGo：[README](https://github.com/egoist/mygo/blob/main/README.md)、[架构](https://github.com/egoist/mygo/blob/main/docs/architecture.md)、[Web 前端](https://github.com/egoist/mygo/blob/main/docs/frontend.md)、[Go Bind](https://github.com/egoist/mygo/blob/main/docs/bindings.md)、[原生 UI](https://github.com/egoist/mygo/blob/main/docs/ui/README.md)、[托盘](https://github.com/egoist/mygo/blob/main/docs/menus.md)、[跨平台分发](https://github.com/egoist/mygo/blob/main/docs/distribution.md)。
- 现有 CI 已知问题：[运行 37118224938](https://github.com/heihei0299/pi-switch/actions/runs/37118224938)（2026-10-03；Go README CLI 示例匹配测试失败，跨平台 Go build 成功）。
