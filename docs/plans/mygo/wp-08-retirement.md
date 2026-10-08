# WP-08：旧 WebUI 退役（条件性）

> 状态：待实施 · 前置依赖：Gate C + Gate D · 验收关联：AT-01, AT-15, AT-16, AT-18
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 独立提案列出将删除的依赖、文件、文档和发布影响；对 CLI/npm 浏览器可用性的保留策略先做决策。
- 先证明等价并更新测试/构建/升级，再删除不再需要的 React/Vite/WebKitGTK 依赖；不得先删后补。
- **验收**：所有旧契约与跨平台测试保持通过，下载/启动/回滚路径无断裂，PR 中列出对用户的实际破坏性变更和迁移说明。

## 可逐项执行的任务

- [ ] 单独列出待删除的 React/Vite/WebView 文件、依赖、CI 步骤、安装/浏览器访问能力及用户可见行为。
- [ ] 仅在 Gate C + Gate D 全部满足并形成 ADR 后，确定是“完全删除”还是为 CLI/WebUI 保留浏览器前端。
- [ ] 一块一块删除，反向核对 AT-01、AT-15、AT-16、AT-18、更新/回滚和 npm 分发。
- [ ] 有任一关键功能退化立即停止退役；回滚通过受控 revert/修复提交，不能删用户数据。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-01 | CI + CLI | 原有 Go、WebUI、Playwright、smoke、跨平台构建通过；记录并单独关闭继承的 README 测试失败 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |
| AT-16 | 安装与回滚 | 干净系统安装、旧版本升级、备份/恢复、卸载桌面、恢复旧 CLI；config/models.json/requests.db 未丢失、Key 未泄露、daemon 不遗留异常进程 |
| AT-18 | CI/发布隔离 | main 的原六目标 CLI build 不退化；desktop 分离构建并在相应平台跑可执行 smoke；tag/npm 发布条件不被 desktop PR 误触发 |

## 完成条件

用户能力无回退、全部发布与回滚矩阵通过；若无法满足，保留旧前端并关闭退役工作包。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
