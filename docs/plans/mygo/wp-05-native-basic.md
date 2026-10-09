# WP-05：简单页面原生 UI 迁移

> 状态：实施中（可选 Native Overview 已加入；WebView 仍为默认/回退入口，Gate B 未通过） · 前置依赖：Gate B · 验收关联：AT-11, AT-12, AT-14, AT-15
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 评估 MyGo ui 组件架构、状态更新、列表虚拟化、快捷键、屏幕阅读器，逐页迁移 Home、设置概要、Proxy 状态/控制、基本 Profiles 列表。
- 每个页面在完成等价自动/手动测试后才替换默认入口；原 WebView 页面保留退路。
- **验收**：同功能、同错误与空状态、键盘完全可操作、中文 IME 和高 DPI 无退化，测量性能并留截图/视频。

## 可逐项执行的任务

- [ ] 先迁 Home/Proxy 状态、基本 Profiles 列表和设置概要，单个页面完成后才切换默认入口。
- [ ] 复用相同服务 DTO、校验和错误语义；为 native ui 视图补单测和输入/焦点回归。
- [ ] 覆盖列表虚拟化、空数据、长文本、深浅主题、键盘导航、无障碍及 fcitx5 组合输入。
- [ ] 用重复测量对照 WebView/Native 的可交互时间、RSS、CPU 与滚动行为，保留可随时回退的旧页面。

## 当前进展（2026-10-08）

- 已加入可选的原生概览窗口，显示 Proxy 状态/控制、基本 Profiles 列表和 Proxy/Web UI 地址摘要；WebView 仍是默认入口，未因本地单测切换用户默认页面。
- 原生列表使用共享 config 类型读取且只渲染 profile 名称，不显示 API Key；Proxy 操作复用现有 daemon 服务。还未完成全部页面的 DTO/行为等价验证。
- 自动 UI 测试覆盖空态、长列表虚拟化、键盘 Home/Up/Down/End 导航、名称过滤、模拟 IME composition 和 scale 2/dark theme 渲染；真实 fcitx5、屏幕阅读器及桌面窗口操作未验证。
- Gate B、跨平台 GUI 和重复性能测量仍未通过；详见[本 WP 验证记录](../../test-reports/mygo-wp-05-native-basic-2026-10-08.md)。

## 后续推进（2026-10-08）

- Native Overview 增加当前 Profile 的 API、上游主机和模型/暴露数量摘要，并支持安全切换；写入走共享配置锁，过期 Profile 名称会被拒绝。
- 上游地址只显示 scheme/host/port，不显示 URL userinfo、path 或 query；隔离测试确认 API Key 与 URL 凭据不进入 UI。
- 概览刷新失败时保留上次成功的 Profile/地址摘要和选择，同时显示错误；成功刷新才替换快照并重置列表选择。
- 此增量仍不构成 Home/Profiles/Settings 的完整行为等价证据，Gate B 保持未通过。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-11 | Linux 实机 | Arch+niri/Wayland：打开/隐藏、焦点、菜单、粘贴、fcitx5 中文组合与候选、3200×2000@scale2 的缩放/点击位置/滚动；无残影、文字截断、候选错位 |
| AT-12 | 已退役 | Windows 桌面支持已移除；不再作为 Gate 或发布验收目标。Linux 桌面目标为 amd64/arm64 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |

## 完成条件

每个已切换页面通过相同业务测试、键盘/IME/HiDPI 测试；保留原页面作为回退。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
