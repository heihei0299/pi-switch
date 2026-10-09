# MyGo 测试命令（按改动选择）

> 远程 `mygo` 分支已包含 `desktop/` MyGo 模块、代理控制、管理适配器和 React WebUI 挂载。只验证本次改动；不重新执行 WP-00 的完整基线，也不每个任务跑一遍 CI。

## 单项开发：选择相关命令

```bash
# 改了 Go 核心：以实际包路径替换，测试受影响模块
go test ./internal/<实际包>/...

# 改了 MyGo 桌面模块（Go 1.27.1 工具链）
go -C desktop test ./...

# 改了 React WebUI
npm run test:webui
npm --prefix webui run typecheck

# 修改前端构建/资源嵌入时，才运行相应构建
npm run build:webui
npm --prefix desktop run build:web
```

桌面窗口相关改动，启动实际应用手动操作一次即可；**不能用编译通过代替 GUI 功能正常**。新改动没涉及某套测试，就不要求运行该套。

## 仅风险相关改动使用隔离目录

```bash
export PI_SWITCH_CONFIG=/tmp/pi-switch-mygo-test/config.json
export PI_SWITCH_CONFIG_DIR=/tmp/pi-switch-mygo-test
export PI_SWITCH_DB=/tmp/pi-switch-mygo-test/requests.db
export PI_SWITCH_MODELS=/tmp/pi-switch-mygo-test/models.json
```

先确认程序确实读取这四个临时路径，再用假配置测试对应写入或代理启停。不能对真实用户配置或真实 API Key 做试验。

## 最后一次全量验证

准备合并 `main` 时，通过现有 `.github/workflows/ci.yml` 跑完整 Go/WebUI 测试、构建及安全扫描，再人工检查供应商、Gateway 显式发布、Proxy、Stats 主流程。CI 中独立桌面模块的测试和实际 GUI 检查按本次变更补充；普通 `mygo` push 不会自动触发原工作流，需要显式运行或在 PR 上验证。

Linux amd64/arm64 是新版本唯一支持目标；桌面 GUI 与安装实测仍按发布 Gate 验收。macOS/Windows 不要求新版本 GUI、打包、签名或进程管理验收，也不再验证这些已退役平台的 CLI/npm 构建；Linux CLI/npm 发布仍按独立发布门禁验证，历史发行物不回溯修改。

## 结果记录

一行即可：`修改范围；运行命令；通过/失败；未测试项目；commit SHA`。

遇到失败先修复当前任务；独立 bug 独立提交。最终全量测试通过后再决定是否合并，不自动合并或发布。
