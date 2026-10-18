# 架构可维护性改善：实施与验证记录

日期：2026-10-16。基线：`mygo` / `0c3dbf7` 加任务开始时已有的未提交修改。

结论：M0–M2 实现与 M3 本地检查完成；隔离 GUI 复测因无法连接测试机而 BLOCKED。无提交、无推送，不构成远程 CI 或完整 GUI 验收通过。

## 1. 实际改动

### Gateway

- 配置通过 `ConfigReader` 注入现有 Gateway 用例。
- `PreviewGenerated`、`PreviewDraft`、`PreviewSelected`、`PreviewSelectedDraft` 负责共享读取和计划编排；HTTP、CLI、Native Gateway/JSON 验证迁移到这些操作。
- 保留 generated/draft 的 overwrite/fill-missing 策略、Native nil/显式空选择差异、HTTP 选择＋草稿编辑内容和发布告警。
- 预览不调用 Publish；原 filegateway 发布锁、stale 检查和原子替换未改动。
- TUI 的已加载配置快照、旧 gateway 兼容函数和 JSON 初始草稿生成保留，复用原计划规则，而非复制另一份规则。

### Provider 管理

- Save/Delete/SelectCurrent/Duplicate 的 application 函数接受 `context.Context` 和已装配的 `*providers.Service`。未新建应用层接口或依赖注入框架。
- 入口负责装配；移除旧 `saveProviderContext` / `deleteProviderContext` 转发包装。
- Native 标准任务把真实任务 Context 交给回调，Provider 写入和 Overview 选中操作向下传递。Settings 的已有 Context 参数也接入该任务。
- 保留校验、revision、备份与错误映射；取消和超时不会被重新包装成普通字段校验提示。
- 配置恢复、全量替换、全局 DB 生命周期不在本批范围。取消不是回滚，已提交的原子写入不撤销。

## 2. 行为证据

| 合同 | 证据位置/测试 |
|---|---|
| 预览每个依赖只读一次、无发布 | `internal/usecase/gateway/preview_test.go`：`TestPreviewFlowsReadOnceWithoutPublishing` |
| nil 与显式空选择不同 | 同文件：`TestPreviewSelectedDistinguishesNilAndEmpty` |
| 配置/current/catalog 失败及取消不继续执行后续依赖 | 同文件：`TestPreviewFailureStopsBeforeLaterDependencies` |
| 无效选择保留错误类别与读取顺序 | 同文件：`TestPreviewSelectedDraftInvalidSelectionPrecedesCurrentRead` |
| generated/draft 的 catalog 填充差异 | `internal/usecase/gateway/service_test.go`：`TestGeneratedAndDraftPlanUseDifferentCatalogFillPolicies`；HTTP draft enrich 测试 |
| 保存、删除、选中、复制可用内存实现测试 | `internal/application/provider_management_test.go`：注入写入失败、成功状态、revision 冲突、取消与超时测试 |
| 文件备份、凭据恢复、无关配置保留 | `internal/application/management_test.go`、`provider_operations_test.go`、`internal/server/profile_mutation_contract_test.go` |
| Task Context 实际传给 worker | `desktop/tasks_test.go`：`TestStandardTaskPassesCancellationToWorker`；这是 headless 控制流测试，不是真实窗口交互验收 |
| 非法 publish 不获取 catalog | `internal/server/gateway_publish_validation_test.go`：`TestGatewayPublishMissingProvidersDoesNotFetchCatalog` |

最后一项测试在实现收尾时抓到一次回归：缺少 providers 的请求虽返回 400，却多调用了一次 catalog。已把该请求字段检查前置，测试由失败变为通过。正常输入的发布合同不变；同时存在请求字段错误和配置/文件错误时，现在优先返回请求字段错误。

## 3. 已执行检查

以下检查均在本地执行，按变化范围补跑；未因文档改动重复跑已通过的代码测试。

| 检查 | 结果 |
|---|---|
| 实施前：Gateway 用例、filegateway、Provider 用例、application 与 HTTP Gateway/契约定向基线 | PASS |
| `go test ./internal/application ./internal/usecase/providers ./internal/usecase/gateway ./internal/server ./cmd/pi-switch ./internal/gateway ./internal/adapters/outbound/fileconfig -count=1` | PASS |
| 后续取消映射调整：`go test ./internal/application -count=1` | PASS |
| 后续 publish 字段校验调整：HTTP `Test(Gateway\|PublishedAuthCaveat\|StrictConfig_)` 定向测试 | PASS |
| Desktop：`CGO_ENABLED=0 go test -mod=readonly ./... -count=1` | PASS |
| 随后新增 Task Context 测试：Desktop 定向运行该 Test | PASS |
| `go run ./internal/archcheck` | PASS，两个模块 |
| 相关根模块包 `go vet`；HTTP 后续调整后补跑该包 | PASS |
| Desktop：`CGO_ENABLED=0 go vet -mod=readonly ./...` | PASS |
| 相关 diff 与格式检查 | PASS |

`internal/domain/gateway` 当前没有独立测试文件，不将 `[no test files]` 计为覆盖；域规则通过 `internal/gateway`、Gateway 用例及入口测试执行。现有 server 中的 `t.Skip` 未解禁，不将其计为已验证场景。

## 4. 未通过或未执行

- **BLOCKED：隔离 GUI 复测。** 已执行 `ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=8 gui-test@10.10.10.244 ...`；返回 `ssh: connect to host 10.10.10.244 port 22: No route to host`，退出码 255。
- 未绕过主机密钥校验，未重启、重装或重置容器，未操作原 GUI/Proxy 进程或未知监听端口。
- 未执行根仓库全量测试、远程 CI、真实硬件/IME/托盘复测，也未将本地 headless Desktop 测试冒充这些验收。
- 下一步：测试机恢复可达后，在独立 HOME/XDG/config/models 路径中核验 Provider 保存/复制/删除/选中及窗口退出时的任务取消，再验证 Gateway/JSON 预览和显式发布确认。只能使用假凭据和隔离数据。

## 5. 范围保护

任务开始时保存了现有源码与计划的比较基线；只修改列入本计划及直接受影响调用链的文件。此前已有的端口/响应限制/架构门禁修复继续保留；用户的归档、文档清理及其他无关改动未回滚。所有改动仍在工作树中，未提交、未推送。
