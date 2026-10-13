# pi-switch Architecture V2 — 线性迁移工单

基线：`mygo@51530102c94dfd5412df25eb0e612c73a5d668f1`。目标设计见 `SPEC-ARCHITECTURE-V2.md`。本计划允许大规模内部变更，但每张工单必须是可独立验证、可独立交付的变更；不得同时维护两套冲突的真相源。

## W00 — 锁定架构与既有合同（基础阶段）

**范围：** 架构设计、导入约束、现有行为基线及测试夹具，不变更业务行为。

- 录入分层与包依赖规则；为后续新增 `internal/domain`、`internal/usecase`、`internal/adapters` 的 import 定义检查。
- 固定 HTTP / Native / CLI Provider 操作的现有备份、revision、rename、错误、无副作用条件。
- 固定 Gateway 发布、Proxy 进程身份、Stats unknown/token 等合同。

**验收：** 依赖检查可在旧代码上运行；违反规则的样例可被正确拒绝；基线测试结果/无法运行项如实记录。交付：SPEC、工具和合同测试。

## W01 — Providers 统一业务用例（第一项生产重构）

**范围：** `internal/application/management.go`、`internal/profile`、`internal/server/profile_handlers.go`、`desktop/native_providers.go` 的 Provider CRUD，先从 `PUT`、`DELETE` 切入。

- 构建一套类型化 Provider command/result 和内部 mutation 核心。
- 将 HTTP `PUT/DELETE` 的业务更新闭包迁入 usecase，adapter 仅处理 HTTP 映射。
- Legacy HTTP 与 Native 交互式保存分别明确声明当前保存语义；不要未经授权给 HTTP 增加 revision 要求或改变 rename/current/backup 语义。
- CLI、TUI 消费同一底层规则，不引入复制的字段校验。

**验收：** 相同输入的旧/新 HTTP response、配置字节语义、备份个数、错误优先级保持一致；Native 的乐观并发和备份仍然有效；双入口并发更新不丢写；根与 Desktop 相关测试通过。

## W02 — Config / Backup / Credential IO 边界

**范围：** config 持久化、恢复、文件锁、权限、credential mask；抽最小存储 Port 与 Adapter。

**验收：** 旧 config shape/migration 行为不变；跨进程锁定写、原子保存、0600、失败零写入、恢复旧字节及 secret 不外泄有明确测试；其他入口不再复制文件事务逻辑。

## W03 — Gateway 纯规则与发布 IO 分离

**范围：** canonical plan、catalog enrich、Gateway file reader/publisher、Native/HTTP 发布用例。

**验收：** 生成/手工 draft 的补齐规则不变；preview 与 publish 同 plan；冲突/取消/no-op 零写；`models.json` 第三方 provider、metadata、权限和过期检测不回归；publish 对持久化只有一个出口。

## W04 — Desktop 交互与状态控制器

**范围：** 将 `desktop/shell.go` 的业务任务编排移入按页面分离的 controller/presenter；`desktopShell` 保留窗口、托盘、导航、进程退出和 UI 线程协调。

**验收：** 每一页状态仅由对应 controller 管理，跨页联动通过显式方法；旧任务不覆盖新页面状态；关闭窗口不会显示过期结果；已有 Native tester 通过；Proxy 不因 GUI 退出自动停止。禁止新框架/事件总线。

## W05 — 推理 Pipeline 拆分

**范围：** 从 `internal/server/proxy_handlers.go` 提取 Route/Plan/Outbound/Result/Usage/Stream 协调；transport handler 仅进行 HTTP adapter 工作。

**验收：** Chat、Responses、Messages × stream/non-stream 的 status/envelope/SSE/tool history/cost/unknown 事实与基线一致；取消传播与回滚点明确；错误时无假 success；代理单候选语义及旧裸 ID 规则不变。该工单视工作量拆为 W05a～W05d，各自独立交付。

## W06 — Stats / Sessions 查询与性能

**范围：** Stats service 与 SQLite 查询隔离、聚合去重、分页/索引、Session matcher，最后由 Native/API 共用一个 analytics usecase。

**验收：** 全量历史与分页统计不改变 unknown、cache rate、reasoning subset、cost、conversation attribution；10k/100k/1M（资源允许时）数据 benchmark 有基线与前后结果；减小重复扫描，报告内存与响应时间，不能只是换名字。

## W07 — Packages / Settings / Daemon 边界

**范围：** packages 命令 DTO、settings 与配置保存、daemon 的独立进程能力；逐步停止 UI/HTTP 直接越层操作。

**验收：** Packages CRUD/导入仍兼容；原子配置及备份符合合同；PID 身份与无主监听者拒绝接管语义保持，止动/重启/停止幂等；运行时状态不被文件存在性误判。

## W08 — 入口适配收敛、移除旧路径与全量验证

**范围：** 清除已无消费者的旧逻辑；收敛 `main.go`、`desktop` wiring 和 HTTP handlers；移除新路径之外的业务编排。

**验收：** `go test ./...`、`go vet ./...`、`go -C desktop test ./...`、`go -C desktop vet ./...`、Secrets Scan、Root CI、Desktop CI、CLI/API/Proxy smoke 均有最终 HEAD 证据；Go 静态依赖规则零违规；有兼容性对照与未测实机项；没有额外网络接口或持久化格式变化。

## 统一工单执行过程

1. 核对上一工单提交与当前 HEAD，不在未知工作树上覆盖代码。
2. 先写针对旧行为的定向回归 fixture（必要时做 benchmark）。
3. 移动一条完整业务用例到新边界，原有入口转为薄适配层。
4. 运行相应包测试、两个 Go module 的相关编译/测试；修复全部回归。
5. 一个工单最多一个交付 Commit；记录 SHA、修改范围、实际验证与遗留风险。无变更不造空 Commit。
6. 未明确授权不 Push、合并 `main`、发布；真实用户数据不可作为迁移测试夹具。

## 停止条件

- 无法证明历史 API/CLI/文件合同兼容；
- Gateway 自动发布或 Provider 修改错误联动 models.json；
- 数据文件权限降低、密钥泄露或旧配置被不可逆迁移；
- daemon 可能错误停止无主进程；
- 测试尚未通过却试图推进依赖该行为的下一工单。

实机 GUI 验收可由后续阶段单独完成，但不能用自动化测试声称已通过。
