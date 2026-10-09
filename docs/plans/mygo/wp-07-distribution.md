# WP-07：构建、分发与升级

> 状态：实施中（独立 test-artifact workflow 已加入；未发布，Gate D 未通过） · 前置依赖：Gate B（M3 可后续追加） · 验收关联：AT-01, AT-10, AT-12, AT-13, AT-14, AT-16, AT-17, AT-18
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 桌面构建和 CLI/npm 分发分离：版本与 release 身份可追溯，桌面安装包/依赖说明完整；Linux amd64/arm64 构建及实机安装验收。Windows 和 macOS 不属于桌面支持范围；macOS CLI/npm 目标不变。
- 不破坏现有 .github/workflows/ci.yml 的 CLI 6 目标矩阵；桌面另加 build/test matrix，并确保桌面失败不会误发 npm。
- 更新用户说明、开发手册、已知限制、安全边界与升级回滚步骤。正式签名与公证由受控凭据完成，缺少时明确仅内部/测试版本。
- **验收 Gate D**：AT-14～AT-18；旧版和新版的配置/请求记录均可验证保留，禁用或卸载桌面不会破坏 CLI/Proxy。

## 可逐项执行的任务

- [ ] 保持原有 CLI/npm 发布矩阵；建立与 CLI 发布相互隔离的桌面编译、安装包、GUI smoke 与更新渠道。
- [ ] 打包 Linux 系统依赖与缺失诊断；固定 MyGo 依赖。
- [ ] 在洁净机器执行安装、重装、升级、卸载、旧 CLI 回退及 daemon 留存检查；核对 config/models.json/requests.db。
- [ ] 确保 source commit 与二进制身份一致，secret scan 和打包内容无凭据；没签名/公证时只标记测试分发。

## 当前进展（2026-10-08）

- 独立 Desktop CI 最初为 Linux/Windows 四个目标打包短期 Actions test artifact；2026-10-09 退役 Windows 后仅保留 Linux amd64/arm64。CI 检查 CLI resource 内嵌的 commit/target 并生成对应身份清单，不发布 Release/npm。已在隔离 worktree 成功构建 Linux amd64 `.deb`/tarball/app。
- Linux amd64 打包 job 现在会从隔离 `HOME` 对生成的 tarball 执行安装、覆盖式重装和卸载 smoke；还验证含绝对 symlink 的恶意归档会被拒绝，不会覆盖现有安装或写出 staging，并验证模拟 config/models/requests 数据及用户同名命令保留。
- 保持 spike app ID；Linux 安装依赖由 MyGo 声明 GTK 3/WebKitGTK，AppIndicator 可缺省。
- Linux tarball `install.sh` 已在临时 HOME 验证安装、重装替换、卸载、config/models/requests 数据保留，以及与应用同名的用户命令不会被覆盖或卸载；此外在临时源码副本构建 `0.1.1` 包，并用其 installer 将隔离 HOME 中的 `0.1.0` 包升级，确认新版二进制替换、旧文件移除、用户数据保留。该测试没有发布版本，也未验证真实用户数据迁移；Debian 系统级安装/回滚、GitHub workflow 和跨平台 GUI 仍未验收，Gate B/D 未通过。验证结果见[本 WP 记录](../../test-reports/mygo-wp-07-distribution-2026-10-08.md)。
- 现有 Linux amd64 `.deb` 的 ar/control/data 归档结构、包名/版本/架构、GTK/WebKitGTK 依赖和安装载荷已静态检查；未安装 `dpkg`/`dpkg-deb`，不代表系统级安装或升级验证。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-01 | CI + CLI | 原有 Go、WebUI、Playwright、smoke、跨平台构建通过；记录并单独关闭继承的 README 测试失败 |
| AT-10 | 安全测试 | 外部 URL/普通浏览器不能调用私有 MyGo Bind；本机管理 API 的回环/非回环鉴权合同保留，拒绝恶意 origin 与未授权写入；Key 不进入日志/状态快照/错误弹窗 |
| AT-12 | 已退役 | Windows 桌面支持已移除；不再作为 Gate 或发布验收目标。Linux 桌面目标为 amd64/arm64 |
| AT-13 | Linux 降级 | libayatana-appindicator3 缺失、无 system tray、WebKitGTK 缺失/版本不匹配、Wayland portal 不支持快捷键：应用不死锁，可找到替代入口，错误可读；托盘仅菜单模式 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-16 | 安装与回滚 | 干净系统安装、旧版本升级、备份/恢复、卸载桌面、恢复旧 CLI；config/models.json/requests.db 未丢失、Key 未泄露、daemon 不遗留异常进程 |
| AT-17 | 依赖与秘密 | 固定依赖/锁文件、依赖许可证、敏感信息扫描、打包内容检查、构建身份与 commit 可追溯；未经签名的发行物明确标注不可正式分发 |
| AT-18 | CI/发布隔离 | main 的原六目标 CLI build 不退化；desktop 分离构建并在相应平台跑可执行 smoke；tag/npm 发布条件不被 desktop PR 误触发 |

## 完成条件

Gate D：CI、原生安装与回滚、数据完整性、受支持平台实测及签名发布策略通过；CLI/npm 不退化。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
