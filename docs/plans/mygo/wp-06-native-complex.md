# WP-06：复杂页面原生 UI 迁移与决策

> 状态：实施中（原生复杂页面能力原型已加入；尚未与真实数据/服务等价） · 前置依赖：WP-05 · 验收关联：AT-04, AT-07, AT-11, AT-12, AT-14, AT-15
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 分别研究 JSON 文本编辑器（语法/校验/撤销/选区/输入法）、Gateway 模型草稿和 diff、Stats 图表与会话树/分页、Packages 导入/移除。
- 对每一块做“组件能力、可访问性、速度、维护成本、可回滚性”的书面结论。
- **验收 Gate C**：如任一关键功能不及现有 WebUI，保留混合架构并明确原因；全部页面等价且跨平台通过时，才批准移除 React/Vite/WebView 的独立变更。只迁移完成的页面不得强迫用户使用不完整 Native UI。

## 可逐项执行的任务

- [ ] 分别为 JSON 编辑器、Gateway 草稿/差异预览、Stats 图表和会话浏览树设计可重复的功能原型。
- [ ] 编辑器必须验证中文 IME、撤销重做、选区、JSON 校验、非法输入保留、草稿不丢失。
- [ ] 图表与列表须验证大数据集、unknown usage/cost、筛选分页和会话归属，不修改事实表。
- [ ] 逐组件形成“等价/不等价/待验证”结论；一项关键功能不达标即保持混合界面，不进入退役阶段。

## 当前进展（2026-10-08）

- 已加入 opt-in 原生复杂页面：Gateway/Stats 读取真实本地服务，Gateway 选择可显式发布；JSON 编辑器加载只含生成 pi-switch providers 的安全草稿并共享校验，编辑草稿现可经显式确认发布。
- MyGo TextArea 的选择、undo/redo、中文 IME composition、非法 JSON 保留、基于 Unicode 字符的 JSON 语法错误行/列提示及共享 Gateway plan 冲突校验在 UI tester 中通过；语法错误所在整行现会着色提示，仍缺行号 gutter、背景高亮和完整冲突处理流程。
- Packages 原生原型已接入共享列表/导入/添加/启停/软卸载服务；WebUI 仍是生产入口，Gate C 未通过，React/WebView 不退役。
- Native Stats 增加按 provider 请求占比显示的可访问水平条图；最近请求显示时间、成功/状态码、input/output/cached/reasoning/cache rate/total/cost 维度并保留 unknown；请求错误正文仍不直接显示，避免原样暴露上游返回内容。
- 逐组件结论和验证限制见[本 WP 验证记录](../../test-reports/mygo-wp-06-native-complex-2026-10-08.md)。

## 后续推进（2026-10-08）

- Gateway 预览调用共享生成/canonical-plan 服务读取真实配置与 `models.json`，显示 provider/model 状态摘要且不渲染 API Key；预览不写文件，另有独立的显式确认发布操作。
- 临时路径测试确认预览保持 `config.json` 与 `models.json` 字节不变，且 UI 不显示配置和模型文件中的测试凭据。
- Native Gateway 现在可按已暴露模型勾选子集并重新生成共享 canonical preview；默认勾选当前已发布项。该交互仅改变预览，不写入 `models.json` 或配置。
- Gateway 变更现在须经显式确认后才会调用共享 `gateway.PublishPlan`；有冲突或无变更时禁止发布，第三方 provider 由共享 canonical plan 保留。
- JSON 草稿预览显示共享 canonical plan 的 provider 新增/删除/变更；冲突或无变更不会打开确认，取消不写入，确认后仅更新 `models.json` 并保留第三方 provider；已发布草稿再次校验为零变更。
- Packages 原型复用 `internal/server` 的 package registry 服务；打开列表时不存在的 pi-switch DB 不会被创建，删除动作沿用软卸载并需确认。
- 隔离的双模型测试确认选择一个候选只把该模型放入 proposal；取消确认不写文件，确认后只发布选中模型并保留第三方 provider，配置不变且 UI 不显示凭据。
- Stats 页现在复用管理 API 的配置化 service，按设置读取本地 SQLite 汇总；请求列表与会话列表可独立选 Today/24h/7d/Custom/All-time 并分页，单会话完整历史仍可独立分页查看，显示失败计数、token 汇总、cache rate、平均延迟、可展开的 provider/model 汇总列表，以及逐条请求的时间、状态、provider/model、token 维度与 cost，unknown 值保持 unknown。
- 临时 SQLite + JSONL fixture 测试覆盖 1,000 条近期待查请求加一条 8 天旧记录，验证预设/自定义日期范围过滤、独立分页、unknown tokens/cost、sessionScan 会话归属及请求事实/会话文件不变；真实用户 sessionScan 目录和物理 GUI 尚未实测。
- provider/model 汇总列表在 1,000 个模型项下经过 UI tester 的虚拟化和 End 键导航测试。
- 独立 SQLite fixture 含成功与失败请求，验证失败计数会显示、失败请求的 usage 不计入 token/cost 汇总、零行与 missing-DB 状态显示 unknown 占位且读取不改写事实。
- 缺失数据库/使用数据时的 Stats 汇总保留 `-` unknown 表示，不伪造 cache/token/latency 值。
- Gate C 仍未通过：Stats 缺原版图表和失败错误详情；JSON 编辑缺行号 gutter/错误高亮，Gateway 缺完整冲突处理，Packages 保留 WebUI。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-04 | 单测 + temp paths | Gateway generated/draft 预览、metadata 0 值、diff、conflict、只选部分；发布前 models.json 字节不变，显式发布后原子更新、第三方 provider 保留；失败不产生半成品 |
| AT-07 | SQLite + GUI | 每个请求只写一条历史；usage/cost/unknown、时间窗、会话归属/分页与旧版一致；历史事实不被迁移重写；session 目录只读 |
| AT-11 | Linux 实机 | Arch+niri/Wayland：打开/隐藏、焦点、菜单、粘贴、fcitx5 中文组合与候选、3200×2000@scale2 的缩放/点击位置/滚动；无残影、文字截断、候选错位 |
| AT-12 | Windows/macOS 实机 | Windows 10/11 WebView2/IME/高 DPI/单实例；macOS WKWebView/托盘/睡眠恢复/签名状态。每个受支持目标都运行可交互 GUI smoke，缺设备标记 block，不拿交叉编译充数 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |

## 完成条件

Gate C：所有拟退役的关键交互功能达到等价并有证据；否则明确记录不等价原因并止步混合路线。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
