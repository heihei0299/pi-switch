# SPEC: pi-switch 全面迁移至 MyGo Native GUI

- **Spec ID**: `SPEC-MYGO-NATIVE-GUI-001`
- **状态**: Proposed（待批准）
- **基线分支**: `mygo`
- **已核实基线提交**: `95ddc55cc545dbf3fabbeae6825bd98d6c6204c5`（2026-10-09）
- **目标平台**: Linux amd64 / arm64
- **最终形态**: MyGo 原生 GUI + 共享 Go 应用服务 + Go 业务核心 + 独立无头 Proxy

## 1. 背景与问题

当前 `mygo` 分支采用 MyGo 桌面 Shell + WebView 承载 React WebUI，并提供部分可选的 MyGo Native 原型。`desktop/shell.go` 与 `desktop/native_complex.go` 集中管理多领域 UI 状态和任务；Native 与 HTTP 管理入口存在部分业务编排重复；React、Vite、WebView 和 Native UI 并存，增加长期维护成本。

**架构决策**：最终全面采用 MyGo Native GUI。React WebUI / WebView 只允许作为过渡期参照和回退，不作为长期产品形态。

## 2. 目标与非目标

### 2.1 必须达成

1. 所有正式图形交互均由 MyGo Native UI 实现，不再加载 HTML/React 页面。
2. 单一 Go 业务规则与持久化事实来源；GUI 通过共享应用服务调用领域能力。
3. GUI Shell 仅负责窗口、托盘、导航、生命周期和必要的组合；领域状态按功能隔离。
4. 后台任务具备取消、结果失效、错误传播及 UI 线程更新规则。
5. 保持 `config.json`、`models.json`、`requests.db`、Pi session 数据兼容；Gateway 仅显式发布。
6. Proxy 可独立运行；关闭 GUI 不得误杀非托管或需要继续运行的 Proxy。
7. 仅交付 Linux amd64/arm64；彻底移除 Windows/macOS 桌面支持及相关交付义务。
8. 最终删除不再需要的 React/Vite/WebView 运行时与构建链、旧 `webui/` 资源和过时文档。

### 2.2 非目标

- 不重写 translator/protocol/store/gateway 等已稳定的核心领域逻辑。
- 不引入 DI 容器、通用 Repository 框架、全局事件总线或第二套持久化方案。
- 不在结构性迁移时顺带增加新业务功能。
- 不将 GUI 作为 Proxy 正常运行的前置条件。
- 不强制删除全部 CLI：仅保留启动、诊断、自动化及无头运行必需的命令；TUI 及浏览器管理入口进入退役评估。

## 3. 目标架构与依赖规则

```text
MyGo Native GUI (Linux)
  ├─ Shell: window/tray/navigation/lifecycle
  ├─ Pages: overview/profiles/providers/proxy/gateway/stats/sessions/packages/settings
  └─ Controllers: page state, validation feedback, async jobs
                         ↓
Go Application Services: use cases, DTOs, transaction boundaries
                         ↓
Existing Go Domain/Core: config, profile, gateway, translator, stats, store, daemon
                         ↓
Files / SQLite / Pi integration

Headless Proxy ──> shared core (independent of GUI)
Minimal CLI    ──> shared services/core (independent of GUI)
```

**强制依赖方向**：`desktop/native -> application -> domain/core`。`internal/server` 是 HTTP 适配器，不得成为 Native UI 的业务依赖。Domain/core 不得依赖 MyGo、WebView 或 HTTP handlers。只对确有跨入口编排的用例抽取 application service，不机械新增接口。

建议按功能组织 `desktop/internal/native/{overview,profiles,providers,proxy,gateway,stats,sessions,packages,settings}`，`desktop/internal/{shell,controller}`；具体包边界以避免循环依赖及可测试性为准，不要求一次性搬迁。

## 4. 功能覆盖矩阵

| 功能域 | Native 交付要求 | 重点验收 |
|---|---|---|
| Overview | 运行状态、服务概览、错误态、刷新 | 与核心事实一致 |
| Profiles / Providers | 列表、编辑、切换、校验、凭据遮蔽 | 失败不破坏配置 |
| Proxy | 启停、健康状态、监听地址、日志/错误反馈 | 仅管理已识别进程；退出 GUI 不误停 |
| Gateway | 生成预览、模型选择、草稿、差异、冲突、显式发布 | 预览零写入；原子发布；第三方 provider 保留 |
| JSON Editor | 编辑、粘贴、选区、撤销重做、中文 IME、错误定位 | 非法草稿保留；不泄露密钥 |
| Stats / Sessions | 时间范围、筛选、分页、图表、请求明细、会话历史 | unknown 不伪造成 0；SQLite/session 只读查询 |
| Packages | 导入、安装项管理、启停、软卸载、确认 | 复用共享业务服务；错误可恢复 |
| Settings / Backups / Doctor | 设置、备份恢复、诊断、空态与错误态 | 兼容旧数据；危险操作有确认 |
| Shell | 托盘、窗口、焦点、主题、缩放、关闭策略 | Wayland/niri 实机可用 |

不得以“页面可打开”代替业务等价验收；功能可通过原生交互重新设计，但必须保留关键能力和数据安全契约。

## 5. 非功能要求

- **可维护性**：每个页面独立状态和操作；Shell 不持有所有业务字段；跨入口复用相同用例和规则。
- **并发安全**：任务与窗口/页面生命周期绑定；支持 context cancellation 或 generation token；过期结果不可覆盖新状态；UI 更新通过 MyGo 规定的主线程机制。
- **安全**：API Key 不写入日志/测试快照；预览只读；显式确认危险操作；不修改非托管进程。
- **兼容性**：迁移不修改持久化 schema；如确需 schema 变更，必须独立 spec 与回滚设计。
- **性能**：长列表虚拟化；在同一硬件上记录启动、空闲资源、100/1000 项滚动基线；阈值在相关工作包实施前明确，禁止事后调整。
- **可访问性**：键盘操作、焦点顺序、文本缩放、中文 IME、可辨识错误提示。
- **平台**：Linux amd64/arm64；Arch + niri/Wayland 作为明确实机验收环境，另验证受支持的 Linux 桌面发布环境。

## 6. 分阶段工作包

### WP-00 — 行为基线与功能盘点
- 列出现有 React 页面、API 契约、Native 原型及对应测试。
- 固化 Gateway/Proxy/Stats/Packages 的危险边界回归用例。
- 产出功能覆盖矩阵及迁移前后的行为差异清单。
- **门禁**：测试可复现；尚未删除任何旧入口。

### WP-01 — 共享应用服务与依赖纠正
- 抽取 Gateway 预览/发布的共用编排；Packages 与 Stats 不再要求 Native 直接依赖 `internal/server`。
- 保留既有领域函数与数据所有权。
- **门禁**：HTTP/Native 相同输入得到等价结果；依赖方向检查通过。

### WP-02 — Native Shell、页面状态与异步生命周期
- 拆分 `desktop/shell.go` 的窗口/Proxy/业务任务职责。
- 拆分 `desktop/native_complex.go`，按页面隔离状态、视图和控制器。
- 统一异步任务取消、过期结果丢弃、错误状态和主线程更新。
- **门禁**：快速切页、重复刷新、关闭窗口及退出场景的测试通过。

### WP-03 — 常规页面全原生化
- 完成 Overview、Profiles、Providers、Proxy、Settings、Backups、Doctor。
- 与旧界面逐功能对照，允许原生交互重新设计。
- **门禁**：CRUD、错误态、空态、键盘和敏感操作验收通过。

### WP-04 — 复杂页面全原生化
- 完成 Gateway、JSON 编辑器、Stats、Sessions、Packages。
- 对 JSON 编辑、冲突处理、图表、长列表和 session 查询做专项测试。
- **门禁**：功能覆盖矩阵所有关键项通过；真实 Wayland GUI 验收通过。

### WP-05 — 切换唯一正式 GUI
- MyGo Native 成为默认且唯一正式桌面 UI；停止启动 WebView。
- 迁移期间保留独立回退分支/可用构建产物；不让旧 UI 与新 UI 同时写用户数据。
- **门禁**：完整主流程（配置 → Gateway 预览/发布 → Proxy → Stats）通过；退出/恢复正确。

### WP-06 — 旧 UI 与构建链退役
- 清理 `webui/`、React/Vite、静态资源 embed、WebView 路由及依赖。
- 清理或重新定义 `pi-switch webui`、npm 分发和管理 HTTP 入口；先记录破坏性变化，保留无头 Proxy/必要 CLI。
- 更新 CI、文档、发布脚本和安装/卸载路径。
- **门禁**：代码与构建产物无旧 UI 依赖；不破坏无头运行；用户数据保留。

### WP-07 — 最终架构审计与发布验证
- 检查依赖方向、重复业务逻辑、死代码、状态耦合及安全边界。
- 运行完整 CI、Linux amd64/arm64 构建和安装 smoke；进行 GUI 实机测试。
- **门禁**：所有必过项 PASS，未验证项与已知风险有书面记录；人工批准合并。

## 7. 交付及 Git 规则

- 每个独立 issue/spec 工作单元 **一项变更、一次交付 commit**；完成相关测试与 code review 后再 commit。
- `commit` 不等于 `push`；不得未经授权自动 push、合并 `main` 或发布。
- 不使用 `reset --hard`、强制推送、破坏性清理或提交密钥。
- 普通工作单元只运行受影响测试；每个阶段做必要集成验证；合并前执行一次全量测试。
- 每次交付报告：修改文件、行为变化、验证命令与结果、未验证项、风险、commit SHA。
- 若任一安全/数据/Proxy 门禁失败，停止该工作包并采用可审查的修复或 revert，不删除用户数据。

## 8. 最终验收标准（Definition of Done）

- [ ] 所有正式桌面功能均为 MyGo Native UI，无 React/WebView 页面。
- [ ] `webui/`、Vite/React/WebView 的无用代码、依赖、构建链和文档已清理。
- [ ] GUI 页面与应用服务边界明确；无重复领域规则；无 Native 对 HTTP handler 的业务依赖。
- [ ] JSON/IME、Gateway 冲突与发布、Stats/Sessions、Packages 等关键功能通过真实操作验证。
- [ ] 关闭 GUI 不误停 Proxy；无头 Proxy 与必要 CLI 能独立运行。
- [ ] 原有配置、模型注册、请求历史和 session 数据保持兼容与完整。
- [ ] Linux amd64/arm64 CI、打包/安装 smoke、Wayland GUI 验收通过。
- [ ] Windows/macOS 不在桌面构建、测试、文档支持矩阵内。
- [ ] 所有工作包有可追踪提交、验证记录及最终架构审计报告。

## 9. 风险与决策

| 风险 | 应对 |
|---|---|
| MyGo 原生控件无法覆盖复杂 WebUI 交互 | 在 WP-04 先做可验证原型；未达标不得执行旧 UI 退役；可调整原生交互方案，但不永久保留双 UI |
| 大规模迁移导致业务回归 | 先锁定共享服务和数据契约，按页面/功能拆分提交 |
| 异步任务操作已销毁窗口 | 统一生命周期、取消与 generation 校验 |
| 移除 WebUI 影响 CLI/npm 用户 | WP-06 独立列出兼容性破坏及迁移路径，用户确认后退役 |
| MyGo 版本/平台限制 | 固定依赖版本，Linux 实机验证；必要时单独记录上游缺陷与规避方案 |

## 10. 开始实施前需要确认的产品决策

1. **确认**：正式桌面界面必须 100% MyGo Native；不保留 WebView 产品入口。
2. **确认**：Proxy 必须支持 GUI 关闭后的独立运行。
3. **待决定**：`pi-switch webui` 与 npm 浏览器管理入口是直接退役，还是提供有限兼容期。
4. **待决定**：TUI 何时退役；保留哪些 CLI 无头命令。
5. **待决定**：正式支持的 Linux 发行版、桌面协议与安装包格式。

> 本 SPEC 只定义架构与交付计划，不授权直接修改代码、删除文件、提交或推送。