# MyGo Native GUI 迁移执行 Tickets（精简版）

- **上游 SPEC**：[SPEC-MYGO-NATIVE-GUI-001](../SPEC-MYGO-NATIVE-GUI-001.md)
- **规划基线**：`mygo` @ `f1809cb59d097d294cff62f3aa5d132bf32aac53`（2026-10-09）
- **目标**：Linux amd64/arm64 仅保留 MyGo 原生 GUI；WebUI/WebView/React/Vite 最终退役；共享 Go 核心及独立无头 Proxy 保留
- **性质**：实施计划，不代表已有实现、验证通过或授权合并/发布

## 执行顺序

| ID | Ticket | 前置条件 | 交付门禁 |
| --- | --- | --- | --- |
| [T01](./T01-architecture-foundation.md) | 共享服务、Shell、状态与异步生命周期 | SPEC；现有业务回归基线 | 无业务规则分叉；桌面任务安全 |
| [T02](./T02-native-standard-pages.md) | 常规功能 MyGo 原生 GUI | T01 | 常规页面及危险操作可用 |
| [T03](./T03-native-complex-pages.md) | Gateway / JSON / Stats / Sessions / Packages | T01；可与 T02 开发但提交前须完成 T01 | 复杂交互等价，数据安全 |
| [T04](./T04-native-only-switch.md) | 默认且唯一正式 Native GUI | T02 + T03 | 完整主流程通过，无 WebView 正式入口 |
| [T05](./T05-retire-webui.md) | 旧 WebUI、WebView、前端构建链退役 | T04；兼容性决定记录 | 构建不再依赖旧 UI；无头不受影响 |
| [T06](./T06-final-validation.md) | Linux 集成验收、发布前审计 | T05 | 必过 Gate 全部 PASS；人工批准后才可合并 |

T02 与 T03 各自管理自己的功能域，可在 T01 完成后并行实现，但保持两个独立交付 Commit。T04 必须等待两项均通过；**T04 完成前禁止实施 T05 的删除操作**。

## 全局强制规则

1. **每张 Ticket 对应一个交付 Commit**。内部允许检查点、逐文件重构和局部测试，但不得把另一张 Ticket 的代码混进提交；新发现的独立 Bug 单独成任务/Commit。
2. 先完成相关测试与 Code Review，再 Commit。Commit ≠ Push；Push、合并 `main`、打 Tag 和发布必须另外获得授权。
3. 普通 Ticket 只验证受影响模块；T06 在合并前执行最终全量 CI 和 Linux GUI 实机验收。所有验证须记录命令、PASS/FAIL/BLOCK、未验证项，不将“构建通过”写成“GUI 验收通过”。
4. 测试均使用临时 HOME 与隔离配置/数据库/`models.json`，不得读取或覆盖真实用户密钥与数据；不提交凭据。
5. 不改变 `config.json`、`~/.pi/agent/models.json`、`requests.db`、Pi sessions 的持久化契约；Gateway 仅用户明确确认才发布；Proxy 独立于 GUI；不接管或终止未识别的进程。
6. 仅支持 Linux amd64/arm64；不恢复 Windows/macOS GUI 支持；禁止 `reset --hard`、强推、破坏性清理。
7. 目标是**没有正式 React/WebView GUI**，不是移除服务端 HTTP Proxy、CLI 所需无头接口，也不是修改既有推理协议。
8. 原有 [WP-00～WP-08](../README.md) 为历史阶段计划；本索引是对 `SPEC-MYGO-NATIVE-GUI-001` 的精简执行拆分。若两者冲突，以 SPEC 与本索引的最终纯 Native 目标为准；项目实际规范（如 `docs/system-contract.md`）继续约束数据行为。

## 每张 Ticket 交付报告模板

- **修改**：文件/包和功能边界、可见行为差异
- **验证**：逐条列命令/操作、实际结果与 PASS/FAIL/BLOCK
- **未验证/风险**：未覆盖环境及潜在兼容问题
- **Review**：架构和安全边界检查结论
- **Commit**：SHA（如已获授权提交）；Push/合并状态分别说明

## 实施前关键决策

- T05 前必须单独记录 `pi-switch webui`、npm 浏览器管理入口、TUI 的处理方式、受影响用户和迁移路径；默认保留必要的无头 CLI/Proxy 能力，不凭“全面 GUI 化”直接删除所有命令。
- 发行版/包格式、Wayland/X11 及托盘能力限制在 T06 发布门禁前明确；无法验证的环境不能标称支持。

