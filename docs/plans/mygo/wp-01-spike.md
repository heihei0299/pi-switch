# WP-01：MyGo 技术可行性原型

> 状态：待实施 · 前置依赖：WP-00 · 验收关联：AT-11, AT-12, AT-13, AT-14, AT-17
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 提供独立 Hello Window、原生小控件页、WebView 加载现有静态页面、Go Bind demo、单实例、退出与轻量性能采样。
- 同时验证 desktop module 与根模块的 Go 版本隔离；写下环境依赖、交叉编译命令、MyGo 版本锁定及风险结论。
- 演示 Linux niri 的窗口、中文输入和缩放；无托盘依赖时可启动。
- **验收 Gate A**：Linux Wayland GUI 实机演示成功；Go headless 不受 GUI 依赖影响；Bind 的类型/错误/取消符合需求。任何失败按记录决定替代路线，不进入批量改 UI。

## 可逐项执行的任务

- [ ] 确定 MyGo 发布 tag/commit 并锁定，先评估是否能够用独立 desktop Go module 隔离 Go 1.27.1 与旧根模块 Go 1.24.2。
- [ ] 分别建立最小 Native UI 窗口、WebView 加载 React、MyGo Bind/类型化 IPC 示例；记录资源和错误处理边界。
- [ ] 实测 Linux niri/Wayland + fcitx5 中文输入 + scale 2，并探测 Linux GTK/AppIndicator 依赖。
- [ ] 记录冷/热启动与进程 RSS/CPU 基线；将工具链不兼容、输入法、托盘差异列为 Gate A 的明确阻断条件。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-11 | Linux 实机 | Arch+niri/Wayland：打开/隐藏、焦点、菜单、粘贴、fcitx5 中文组合与候选、3200×2000@scale2 的缩放/点击位置/滚动；无残影、文字截断、候选错位 |
| AT-12 | 已退役 | Windows 桌面支持已移除；不再作为 Gate 或发布验收目标。Linux 桌面目标为 amd64/arm64 |
| AT-13 | Linux 降级 | libayatana-appindicator3 缺失、无 system tray、WebKitGTK 缺失/版本不匹配、Wayland portal 不支持快捷键：应用不死锁，可找到替代入口，错误可读；托盘仅菜单模式 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-17 | 依赖与秘密 | 固定依赖/锁文件、依赖许可证、敏感信息扫描、打包内容检查、构建身份与 commit 可追溯；未经签名的发行物明确标注不可正式分发 |

## 完成条件

Gate A：Linux Wayland GUI 可交互；Go/工具链隔离结论清楚；绑定安全、IME、缩放和启动依赖不存在未记录的阻断项。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
