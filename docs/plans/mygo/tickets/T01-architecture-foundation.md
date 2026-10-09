# T01 — 建立 MyGo 原生 GUI 的可维护架构基础

- **源需求**：SPEC §3、§5、§6 WP-00/01/02
- **优先级**：P0
- **前置**：SPEC 已审阅，核实远程 `mygo` HEAD 和现有系统契约
- **交付**：一个可独立审查的架构重构 Commit；不增加用户可见功能，不删除 WebUI

## 问题与范围

`desktop/shell.go` 同时管理窗口/托盘、Proxy 和业务任务；`desktop/native_complex.go` 混有 Gateway、JSON、Stats、Sessions、Packages 的状态、视图及调用编排。Native 直接借用 `internal/server` 的 Packages/Stats 业务入口，Gateway 流程与 HTTP adapter 有部分重复，界面异步任务分散。

本 Ticket **只优化基础设施和现有行为的结构**，不完成新的业务页面，也不重写 `internal/translator`、`protocol`、`store`、`gateway` 等核心规则。

## 实施步骤

1. **锁定回归基线**：登记现有页面、关键 HTTP DTO/状态码与用户数据写入边界；选择最相关的 Gateway Preview/Publish、Packages、Stats、daemon 测试与夹具。
2. **提取共享用例**：Gateway 生成、草稿与选择预览、冲突、显式发布的共同编排调用现有 canonical plan；抽离可复用的 Packages 业务操作及 Stats service 初始化，使桌面不以 HTTP handler 作为业务依赖。仅有真实跨入口编排时才增加 `internal/application` 服务，不引入 DI 容器、泛型 Repository 或第二套 DTO 真相。
3. **剥离 Shell 业务职责**：Shell 仅保留窗口、托盘、导航、生命周期及依赖组合；各领域 controller 管理业务状态和调用。现有 Native 界面仍可使用。
4. **按域隔离状态/视图**：逐步拆分 `desktop/native_complex.go`（Gateway/JSON、Stats/Sessions、Packages）并保持原型行为不变；不以文件行数为唯一指标，不引入循环依赖。
5. **统一异步任务规则**：建立必要的 `context.Context`、请求 generation/epoch、busy/error 生命周期；过期异步响应不覆盖新结果；窗口销毁后不更新视图；通过 MyGo 指定 UI 更新通道调度。严禁 UI 关闭自动停止 Proxy。
6. **依赖检查**：Native → service/core；`internal/server` 只负责适配 HTTP；core 不依赖 MyGo。

## 验收（全部必须）

- [ ] 相同输入下 HTTP/Native Gateway 预览与发布结果等价；草稿 Metadata 的 FillMissing 与 Generated FillOverwrite 保持原语义。
- [ ] 无额外 Gateway 隐式发布；冲突/无变化时不写 `models.json`，第三方 provider 不被删除；预览只读。
- [ ] Packages 的启停/导入/软卸载语义一致；Stats、Sessions 的只读事实与 unknown 语义不变。
- [ ] Desktop 的业务功能不再以 `internal/server` handler 函数作为入口；保持 API 行为兼容。
- [ ] 快速连续刷新、切页、关闭窗口与迟到结果不会覆盖有效状态或触发失效 UI 更新；异常可以恢复。
- [ ] 原有 Proxy 管理严格验证进程身份；退出桌面不隐式停止独立 Proxy。
- [ ] 本 Ticket 未改变现有 WebView 管理入口及数据文件 schema。

## 验证与证据

- 根模块相关测试：`go test ./internal/gateway ./internal/server ./internal/daemon`（实际改动其他包则补测）。
- 桌面模块：`go -C desktop test ./...`、`go -C desktop vet ./...`；按真实工具链与工作流的 CGO 配置执行。
- 增加针对任务取消、过期结果、关闭后更新的确定性测试；使用临时配置/数据库/模型路径测试危险写入。
- 现有 MyGo GUI smoke：人工确认至少启动、打开现有入口、切换原型、关闭并检查 Proxy 仍正常；无图形环境须标记未验证。

## 不在范围内

完成新页面、改变 CLI 或 npm 行为、删除 React/WebView、修改数据 schema、网络或模型协议重写。若发现既有独立 Bug，单独开 Ticket/Commit。

