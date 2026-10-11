# pi-switch MyGo Native GUI 下一阶段执行计划

- **计划日期**：2026-10-11
- **执行分支**：mygo
- **代码基线**：5041101b406a82b006512adb6948e69c06924b5b
- **上位规范**：[SPEC-MYGO-NATIVE-GUI-001](./SPEC-MYGO-NATIVE-GUI-001.md)
- **目标**：Linux amd64/arm64 上实现唯一正式 MyGo Native GUI，复用现有 Go 应用服务与业务核心，保留独立无头 Proxy。
- **性质**：基于最新代码的执行计划与验收门禁；不代表已获准修改业务代码、合并或发布。

> 本文件是本阶段唯一新增执行计划，不恢复已删除的 WP/Ticket 目录。架构边界、数据合同和最终目标以 SPEC 为准；实际验收记录继续保存在 docs/test-reports/。

## 1. 最新基线与已完成范围

截至上述基线：

| 范围 | 实际状态 | 不可混淆的限制 |
| --- | --- | --- |
| Linux-only 平台退役 | 新版 CLI/Desktop 分发已收敛到 Linux amd64/arm64 | 历史发行物不做破坏性修改 |
| T01 共享应用服务 | 已提交 a148f49；抽出 internal/application、internal/packages 等，Native 开始复用共享用例 | 大范围 HTTP/配置兼容仍应在当前 HEAD 跑 Root CI |
| T02 Native 常规页面 | 已提交 3de4088；七类常规页面与 --native 入口已存在 | 实施报告明确记载真实 Wayland GUI 门禁 BLOCK，提交不等于验收完成 |
| T03 复杂 Native 功能 | 已有 Gateway、JSON、Stats/Sessions、Packages 可选原型 | 不具备完整 WebUI 功能等价证据 |
| T04 Native 默认入口 | 未执行 | 默认管理窗口仍是 React WebView |
| T05 旧 WebUI 退役 | 未执行 | React/Vite、静态 embed、管理协议与打包链仍有消费者 |
| T06 最终验收 | 未执行 | 尚不能宣称具备正式桌面发布资格 |

**CI 证据快照（2026-10-11）：**

- Desktop CI：提交 3de4088 的 run 38118990227 已成功。
- 最新提交 5041101 的 Desktop CI run 38119611724 在计划制定时仍处于执行中；其模块验证与 linux-amd64 打包已成功，不能提前写成全绿。
- Root CI 最近已确认成功的是 95ddc55 的 run 37914636213；当前 HEAD 尚缺 Root CI 完整成功记录。
- Actions 构建/安装 smoke 不等同于 niri/Wayland、fcitx5、真实窗口或运行中 Proxy 生命周期验收。

## 2. 严格范围

### 必须保留

1. 原有 config.json、models.json、requests.db 和 Pi sessions 合同及兼容性。
2. 独立 Headless Proxy、必要 CLI、translator/protocol/store/gateway 既有稳定规则。
3. Gateway 只有显式确认后才发布；预览、取消、冲突及失败不改写目标文件。
4. 不停止、不接管无法确认身份的其他进程；GUI 关闭不隐式停止 Proxy。
5. 应用层为单一业务事实来源：Native UI 不调用 HTTP handler 作为业务服务。

### 明确不做

- 不为迁移重写推理协议、Proxy、数据库 schema 或引入第二套业务框架。
- 不引入 DI 容器、全局事件总线或无用的通用抽象。
- 不因为理论风险、格式建议或未来扩展重新打开已经完成的修复任务。
- T04 前不切换默认入口；T04 完成并验收前不删除旧 WebUI。
- 不自动推送后续实现、合并 main、打 tag、发布 npm/GitHub Release。

## 3. 执行顺序总览

| 顺序 | 交付单元 | 来源 | 前置门禁 | 预期提交 |
| --- | --- | --- | --- | --- |
| 0 | T01/T02 基线与真实 GUI 补验 | 已有 T01/T02 | 当前 HEAD 的相关验证与风险记录 | 无代码变化则不提交 |
| 1 | Gateway 与 JSON 原生交互闭环 | SPEC WP-04 / 原 T03-A | T01 已落地，保留 T02 验收记录 | 1 commit |
| 2 | Stats、Sessions、Packages 原生交互闭环 | SPEC WP-04 / 原 T03-B | 任务 1 的交付边界已明确 | 1 commit |
| 3 | 默认且唯一正式 Native GUI | SPEC WP-05 / 原 T04 | T02、任务 1/2 的必需功能和真实 GUI 门禁通过 | 1 commit |
| 4 | 旧 WebUI/WebView 与构建链退役 | SPEC WP-06 / 原 T05 | 任务 3 通过；旧 CLI/npm/TUI 处置已明确 | 1 commit |
| 5 | 最终架构审计与 Linux 发布验收 | SPEC WP-07 / 原 T06 | 任务 4 完成 | 验证无改动则不提交 |

执行按上述顺序推进。每个任务有自己的变更与验收边界，不把两个独立问题混到一个提交中。任务 1、2 的代码准备可以技术上独立，但按单任务线性交付控制风险。

## 4. 阶段 0：关闭 T01/T02 遗留验证

**目的**：确认上一阶段交付的真实性，不借验收名义重构。

**操作与证据：**

1. 在 mygo 当前 HEAD 手动运行 Root CI，检查 Go、WebUI、Playwright、CLI、npm 内容验证及 smoke；记录 workflow run、HEAD SHA 和失败步骤。
2. 核对最新 Desktop CI 在同一个 HEAD 的 linux-amd64、linux-arm64 任务均完成；不能用旧提交的绿色状态替代。
3. 在真正的 Arch Linux + niri/Wayland 图形会话中，用临时 HOME、PI_SWITCH_CONFIG、PI_SWITCH_CONFIG_DIR、PI_SWITCH_DB、PI_SWITCH_MODELS、PI_AGENT_ROOT 和假 Key 启动新的 --native 进程。
4. 在七个常规页面完成：空数据 → Provider/Profile 创建编辑/切换/复制 → 取消与非法草稿 → Settings 保存 → Backup 创建与恢复 → Doctor 诊断。
5. 仅使用受控测试进程验证 Proxy 启动、刷新、停止、未知监听端口拒绝接管，以及 GUI 关闭/退出后独立 Proxy 的预期存活。
6. 验证 fcitx5 中文组合/候选、焦点、键盘导航、复制粘贴、3200×2000 scale 2、菜单/托盘缺失降级、窗口关闭与单实例。
7. 将 PASS/FAIL/BLOCK 和真实环境记录到现有 T02 测试报告；有明确缺陷时只修复该缺陷并建立对应测试。

**验收门禁：**

- T01 引出的旧 HTTP/CLI 功能回归不得被隐藏；
- T02 实机验收 PASS 才能宣称 T02 已完成；
- 无 GUI 环境应记 BLOCK，而不是借 UI tester/CI 冒充 PASS；
- 如果 T02 仍 BLOCK，可以准备 T03 的独立功能代码，但不得执行 T04 默认入口切换。

**建议命令（从仓库根目录）：**

~~~bash
go test ./internal/application ./internal/config ./internal/profile ./internal/server ./internal/packages
go -C desktop test ./...
go -C desktop vet ./...
gh workflow run ci.yml --ref mygo
# 后续按该次 run 的 head SHA 审查 Root CI/desktop CI，另做真实 GUI 手测。
~~~

## 5. 任务 1：Gateway 与 JSON Editor 全原生化

**范围**：desktop/native_complex.go 中 Gateway/JSON 原型及必要的页面级代码、共享 application 调用与测试。

**必须完成：**

- 统一原生 Gateway 模型候选、生成/草稿预览、当前与建议差异、冲突诊断、显式确认发布流程。
- 使用共享 canonical plan、选择/草稿差异和 PublishPlan；不在 UI 层复制规则。
- JSON TextArea 支持粘贴、选区、撤销/重做、错误定位、输入法组合及非法草稿保留；行号/高亮使用满足实际可读性的原生实现。
- 预览、取消、冲突、零变更和失败情况下，config.json / models.json 字节不变。
- 成功发布只更新允许的目标、保留第三方 provider 与已约定的零值/unknown 语义。
- 异步旧结果不可覆盖较新的选择、草稿或页面状态；按窗口生命周期更新 UI。

**验收**：共享 Gateway/应用服务测试、桌面 UI tester、隔离文件前后字节检查、真实 Wayland 中文 IME 编辑与一次完整 Preview → Conflict/Cancel → Apply 操作。

**禁止**：顺带重写 Proxy、Gateway domain、Stats、Packages、旧 WebUI 或默认窗口。

**建议 commit**：feat(desktop): complete native gateway and json editor

## 6. 任务 2：Stats / Sessions / Packages 全原生化

**范围**：原生 Stats/Sessions/Packages 交互、页面状态、共享统计及包管理服务适配与定向测试。

**必须完成：**

- Stats 支持原 WebUI 的核心时间窗、刷新、筛选、分页、模型/供应商汇总、必要图表、请求明细与导出能力。
- 维持失败请求、cached/reasoning、unknown usage/cost 与统计汇总语义；不得把 unknown 显示成零。
- Sessions 支持真实归属来源、分页及单会话历史；扫描 Pi sessions 只读，未知归属不猜测。
- Packages 复用 internal/packages，覆盖列表、导入、添加、启停、显式确认软卸载、取消与失败反馈。
- 在 100/1000 项夹具下验证虚拟化、键盘导航、滚动/焦点；性能阈值在测试前明确。
- 使用临时 SQLite、Pi session 夹具确认列表/统计读取不会变更事实行和 session 文件。

**验收**：定向 Go/desktop 测试、隔离 SQLite/JSONL 回归、真实 GUI 上完成筛选/会话分页/Packages 变更和取消操作。

**禁止**：重写请求入库、翻译协议或建立第二套统计账本。

**建议 commit**：feat(desktop): complete native stats sessions and packages

## 7. 任务 3：Native 成为默认且唯一正式 GUI

**前置**：阶段 0、任务 1/2 的实际功能门禁通过；旧 WebUI 仍作为回退实现保留。

**必须完成：**

1. 默认窗口以 MyGo Content / ui.View 创建，而非 URL/WebView。
2. 将七类常规页面与复杂页面整合为同一个正式原生导航系统。
3. File/tray 菜单、单实例二次打开、显示/隐藏/退出统一进入 Native。
4. 冷启动到实际用户主路径全程无需 React 页面：Profile → Gateway 预览/发布 → Proxy → Stats/Sessions → Packages/Settings。
5. GUI 退出/关闭与显式 Stop Proxy 保持独立；不影响非托管进程。
6. 留存可审查的回退 commit/构建，并列出 T05 要移除的旧 UI 消费者。

**验收**：真实 Linux GUI 完整主流程、关闭/重启/托盘降级、数据完整性和相关自动化测试通过。

**本任务不删除旧 React/Vite/webui 文件。**

**建议 commit**：feat(desktop): switch default gui to mygo native

## 8. 任务 4：旧 WebUI/WebView 与构建链退役

**当前执行状态（2026-10-11）**：T02、任务 2 与 T03 的正式 Arch+niri/Wayland/IME/托盘/硬件缩放/受控 Proxy 生命周期验收仍为 **PENDING**（不是 PASS）。本轮只在 Ubuntu 24.04/Xvfb/Openbox 做了补充 GUI smoke，托盘能力不可用，不能替代正式门禁。用户明确授权在门禁待定时继续按计划推进；这不豁免验收条件，也不表示发布就绪。

**已完成的 T05 子范围（HEAD `1fc2c56`）**：CLI `webui` 入口已 API-only，根路径显示退役说明；npm Linux CLI 保留，单独的 `webui/dist/` 已从 npm `files` 移除，包检查会拒绝重新加入。Desktop Native 不再配置 MyGo `frontendDist`，仅用于占位 HTML 的 Vite/TypeScript/npm/Bun 构建链已移除，Desktop CI 改用 `desktop/go.mod` 固定的 MyGo Go CLI。Production CLI 也不再链接 `webui` 包或 React 文件，`/api/buildInfo` 所需兼容 metadata 由构建脚本注入；Desktop `--web` fallback 仍显式注入 embed。React、Desktop Go embed 与 fallback 仍保留，因此 T05 尚未完成。

Root CI `38143769439` 与 Desktop CI `38143737004` 均在精确提交 `1fc2c5641a5b50c0eadf1b75849ae2c722287522` 成功。Root CI 覆盖 Go/WebUI checks、双架构 CLI build、functional/API smoke 与 npm package check；Desktop CI 覆盖 Go/MyGo vet、双架构 package build，amd64 installer smoke 通过、arm64 installer smoke 按条件跳过。Release preflight/publish 均跳过。此前 `47ffa91` 的 npm/API 子范围与 `6096da3` 的 Desktop Vite 清理 CI 结果仍有效。

**前置**：任务 3 已证明所有正式桌面能力可纯 Native 操作，且有可恢复旧构建。

**用户已明确的兼容决策**：`webui start|stop|status` 保留为 API-only，`/api/*` 与认证行为不变，根路径显示退役说明；npm/`pi install` Linux CLI 保留且不再携带独立 `webui/dist/`；TUI、Proxy `/v1/*` 保持；Desktop `--web` / `pi-switch-ui://` 至少保留到正式 GUI 门禁通过并发布弃用说明。API/UI 路由与 Desktop fallback 已按这些决策实施。未经新授权，不停止无头 CLI/Proxy，也不删除尚受门禁约束的 fallback。

**必须完成：**

1. 按真实引用关系清理 webui/、React/Vite、WebView custom protocol、Go 静态资源 embed、desktop/management.go 和过渡 UI 资源。
2. 更新 desktop/mygo.config.ts、desktop/package.json / bun.lock、根 package.json、构建脚本与 npm 包检查。
3. 将 Desktop CI 与 Root CI 中旧 WebUI build、TS/Playwright 验证替换为实际 Native/Go 测试，保留 Linux 双架构构建、Secrets Scan、CLI/Proxy smoke。
4. 注意区分 MyGo 可能仍需的构建工具和应退役的 WebView/React 运行时，不能按字符串搜索一概删除。
5. 确认 root Go 包、CLI、Headless Proxy 不因移除 WebUI embed 而无法编译或无图形运行。
6. 对 README、CLI 帮助、npm 分发、安装回滚说明同步更新；历史测试记录可以保留，不能伪造历史验收通过。

**验收**：不构建旧 webui/dist；正式 GUI 无 HTML/React/WebView 页面依赖；Go/CLI/Proxy、Native Desktop Linux 构建和隔离旧数据升级 smoke 全通过。

**建议 commit**：refactor(gui): retire legacy webui and webview

## 9. 任务 5：最终架构审核与发布门禁

**审计内容：**

- 依赖必须遵守 Native → application → core；HTTP handlers 为适配器，不成为 Native 的业务依赖。
- 核查重复业务规则、页面状态耦合、后台任务过期结果、取消与窗口销毁安全。
- 核查旧前端与过时构建链残留、真正允许保留的 HTTP/CLI 消费者。
- 检查凭据遮蔽、配置并发写保护、备份恢复、Gateway 显式发布、Proxy 身份与进程保活。
- Root Go 全量测试、Desktop Go 全量测试、vet、Secrets Scan、Linux amd64/arm64 构建和安装/覆盖/卸载 smoke。
- Arch+niri/Wayland 实机完整主流程、fcitx5、焦点、缩放、托盘、长列表和输入编辑。
- 新旧配置、模型注册、SQLite 请求、session 文件兼容；明确 deb/tarball、发行版支持与未测平台边界。

**验收**：全部必过门禁 PASS；FAIL 必须闭环，BLOCK 不得写成 PASS；产出最终审计报告、提交清单、验证环境、已知限制和回滚说明。合并 main 和正式发布仍需单独人工授权。

**建议验证命令（以任务 4 后实际脚本为准）：**

~~~bash
go test ./...
go vet ./...
go -C desktop test ./...
go -C desktop vet ./...
bash scripts/check-secrets.sh
# 运行变更后的 Root CI 与 Desktop CI，确认 HEAD SHA 完全一致；
# Linux installer smoke 和 Wayland GUI 实机操作另行记录证据。
~~~

## 10. 全局完成条件

- [ ] T01/T02 旧入口与 Native 实机验收记录完整；Root CI 在本计划基线之后的有效 HEAD 上通过。
- [ ] Gateway/JSON 的 Native 功能和实际 IME 通过。
- [ ] Stats/Sessions/Packages Native 关键业务等价与数据安全通过。
- [ ] 默认桌面窗口仅使用 MyGo Native，正式操作不依赖 WebView。
- [ ] React/Vite/WebView 旧界面及无用构建链退役，必要 CLI/Headless Proxy 独立可用。
- [ ] 配置/模型/SQLite/session 数据兼容；退出 GUI 不误停 Proxy，不接管未知进程。
- [ ] Linux amd64/arm64 CI、安装 smoke 和真实 Wayland GUI 验收通过。
- [ ] 各交付单元有独立 Commit、实际测试、Review、未验证项和风险记录。
- [ ] 未经人工明确授权，不合并 main、不发布 npm/Release、不修改真实用户数据。

## 11. 交付纪律与停止条件

- 一个独立任务一个交付 commit；验证与审查先于提交。需要修复新发现的独立 bug 时另立单独任务，不扩大原任务。
- 若使用子代理，按 Goal 模式一个任务一个子代理、线性推进，禁止同一提交混入其他任务。
- 一个任务关闭后，不因新增风格建议或未经证实的理论风险重新打开。
- 交付报告固定包含：修改文件、行为变化、验证命令及 PASS/FAIL/BLOCK、未验证项、风险、Commit SHA；明确写明是否 push。
- 禁止提交 Key、删除真实配置、reset --hard、force push 或其他破坏性清理。
- 某任务出现配置丢失、密钥泄露、误停 Proxy、隐式 Gateway 发布、核心契约退化等阻断问题，立即暂停该任务；先形成最小复现和定向修复，不能通过绕过验收继续向下游推进。

**计划结束标志**：正式 Linux GUI 完全 MyGo Native 化、旧前端安全退役、共享业务合同保持不变、完整 CI 与实机验收均有可复查证据。
