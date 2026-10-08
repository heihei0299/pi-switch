# WP-06：复杂页面原生 UI 迁移与决策

> 状态：待实施 · 前置依赖：WP-05 · 验收关联：AT-04, AT-07, AT-11, AT-12, AT-14, AT-15
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
