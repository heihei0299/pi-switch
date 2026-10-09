# T05 — 移除旧 WebUI、WebView 与前端构建链

- **源需求**：SPEC §2.1(8)、§6 WP-06、§9
- **优先级**：P1
- **前置**：T04 已通过完整 Native 功能验收；有可恢复旧版本与用户可见破坏性变化清单
- **交付**：正式源码/构建不再依赖 React、Vite 或 WebView；本 Ticket 一个交付 Commit

## 关键兼容决策（执行删除前必须留痕）

明确 `pi-switch webui start/stop/status`、npm 浏览器管理包、TUI 的处置：直接废弃、保留兼容窗口或给出替代命令。记录功能映射、迁移说明和现有用户影响；**无头 Proxy 和必要 CLI 必须继续工作**。不以“删除前端”为理由删除推理 Proxy API 或仍有外部消费者的必要管理接口。

## 实施步骤

1. 逐项审计并删除不再被 Native 使用的 `webui/`、React/Vite/npm 前端资源、`webui/embed.go`、WebView custom-scheme 路由、`desktop/management.go`、旧 `desktop/index.html` / `desktop/src` / `desktop/vite.config.ts` 等。**是否删除某文件以真实引用关系为准**；保留仍属于 Go Proxy/服务接口的实现。
2. 调整 `desktop/mygo.config.ts`、`desktop/package.json`、`desktop/bun.lock`、桌面构建脚本与 MyGo 依赖；确认 MyGo Native 启动不需 WebView runtime 或 HTML 构建产物。不要误删 MyGo CLI 必须的 Node/Bun 构建工具；必要工具与 WebView 依赖要区分。
3. 更新根 `package.json`、`scripts/build-*.sh`、npm 包内容检查、`.github/workflows/ci.yml` 和 `.github/workflows/desktop.yml`；取消旧 WebUI TypeScript、Playwright、dist/embed 依赖，替换为 Native/Go 相关检查，保留 Secret Scan、Go 回归、Linux 构建/冒烟。
4. 根据审批过的兼容决策，清理或重定向 `pi-switch webui`、浏览器管理 URL、TUI 和 npm 发布契约。不能静默地让旧命令成功返回但实际不工作。
5. 同步 README、中文指南、WP 历史文档的说明，使默认运行路径、升级、卸载与迁移行为准确。历史计划可以保留标注，不伪造曾经的门禁通过记录。
6. 检索并审查旧 UI 的全部源码、测试、部署和文档引用；确保删掉的是**已经无消费者**的代码。

## 验收（全部必须）

- [ ] 正式桌面应用与 Go CLI/Proxy 的编译、运行均不依赖 React/WebView/HTML 资源；构建流程不生成多余 `webui/dist` 或 `desktop/dist`。
- [ ] 旧 UI 路由/静态资源、引用与构建步骤清理完整，没有空壳 stub 或默默破坏现有发布脚本。
- [ ] Linux amd64/arm64 编译、Desktop package、原有非 UI Go tests 和无头 Proxy smoke 正常。
- [ ] `pi-switch webui`/npm/TUI 的退役决策与用户迁移路径已明确；必要 CLI 和无头 Proxy 在无图形环境下仍可用。
- [ ] 原有 `config.json`、`models.json`、`requests.db`、Sessions 不删除、不改 schema；升级/重新安装可继续读取。
- [ ] 在该阶段未合并 `main` 或发布；不执行破坏性文件清理/强推。

## 验证与证据

- 运行 `go test ./...`、`go vet ./...`、`go -C desktop test ./...`、`go -C desktop vet ./...`；构建 Native Linux amd64/arm64 测试包，执行原有无头功能 smoke。
- 使用可复现静态搜索列出残余 `webui`、`WebView`、`vite`、`react`、`embed` 引用，并人工分类：真正遗留/历史说明/仍需保留的无头服务。
- 在隔离 HOME 上模拟旧版本数据 → 新版启动 → GUI 功能验证 → 无头 Proxy 仍可访问。

## 阻断条件

任何原生关键功能退化、用户数据无法读取、Proxy 不能无头运行或已使用的旧入口没有迁移方案，立即暂停删除；仅通过独立修复或 revert 恢复，禁止绕过门禁。

