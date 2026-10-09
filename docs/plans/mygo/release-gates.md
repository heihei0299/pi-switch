# 暂停条件、回滚与发布验收

> 适用于所有工作包的跨领域风险控制与 M2/M3 最终交付准入。

## 6. 失败处理、回滚与发布门

### 6.1 停止条件

以下任一条件出现，阻断对应里程碑：

- 配置或请求历史丢失；第三方 Pi provider 被覆盖；Gateway 发生隐式发布。
- Proxy 被关窗意外杀死，或者 GUI 管理误杀别的进程/端口。
- API Key 泄露到日志、IPC 不可信 origin、远程管理接口绕过密码。
- SSE/usage/session/错误信封任一协议合同破坏，且无明确批准的兼容决策。
- Linux amd64/arm64 必需目标 GUI 无法交互；无法确认输入法与基础键盘操作。
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

## 8. 参考

- 当前 pi-switch：[架构导航](../../architecture.md)、[系统合同](../../system-contract.md)、[WebUI 使用说明](../../../WEBUI_GUIDE.md)、[工作流](../../../.github/workflows/ci.yml)、[测试脚本](../../../scripts/test-limited.sh)。
- MyGo：[README](https://github.com/egoist/mygo/blob/main/README.md)、[架构](https://github.com/egoist/mygo/blob/main/docs/architecture.md)、[Web 前端](https://github.com/egoist/mygo/blob/main/docs/frontend.md)、[Go Bind](https://github.com/egoist/mygo/blob/main/docs/bindings.md)、[原生 UI](https://github.com/egoist/mygo/blob/main/docs/ui/README.md)、[托盘](https://github.com/egoist/mygo/blob/main/docs/menus.md)、[跨平台分发](https://github.com/egoist/mygo/blob/main/docs/distribution.md)。
- 现有 CI 已知问题：[运行 37118224938](https://github.com/heihei0299/pi-switch/actions/runs/37118224938)（2026-10-03；Go README CLI 示例匹配测试失败，跨平台 Go build 成功）。
