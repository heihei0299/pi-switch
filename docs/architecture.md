# pi-switch 后端架构罗盘

> 这是一份按当前源码导航的架构文档。先从入口定位，再沿真实调用链阅读实现。
>
> 事实边界：`config.json` 保存供应商/渠道/模型事实，`models.json` 是显式发布的 Gateway 视图，`requests.db` 保存请求事实；Legacy `requests.log` 只作为兼容写入和历史导入来源。
>
> 本文对应 Architecture V2 的 W20–W25 实施状态（2026-10-14）。W26 的 Arch+niri/Wayland/Fcitx5 实机验收是独立门禁，不能用本文或自动化测试替代。

## 1. 先看哪里

| 目标 | 入口 | 继续阅读 |
|---|---|---|
| CLI / TUI 命令分发 | `cmd/pi-switch/main.go` | proxy、webui、gateway、settings 等命令处理 |
| 管理 API | `internal/server/server.go:NewMgmtAPIRouterWithAuth` | `*_handlers.go` 管理面 handler；历史 HTTP 入口仍位于 `internal/server` |
| Proxy API | `internal/server/server.go:NewProxyRouter` | `internal/server/proxy_handlers.go`、`internal/usecase/inference` |
| Desktop GUI | `desktop/main.go` → `desktop/shell.go` | MyGo Native 页面、`internal/application` 和共享用例 |
| Provider / Channel / Model | `internal/usecase/providers` | `internal/domain/provider`、`internal/adapters/outbound/fileconfig`、`profilevalidation` |
| Gateway 预览与发布 | `internal/usecase/gateway` | `internal/domain/gateway`、`internal/adapters/outbound/filegateway`、`catalog` |
| Proxy 上游请求 | `internal/adapters/outbound/proxy` | 请求构造、header、上游 HTTP 调用；不写客户端响应 |
| Proxy 客户端响应 | `internal/adapters/inbound/httpinfer` | SSE writer、flush、响应 header/status/body |
| 请求日志 | `internal/usecase/requestlog` | `internal/adapters/outbound/requestlog`、`internal/bootstrap/requestlog.go` |
| Analytics / Stats / Sessions | `internal/usecase/analytics` | `internal/adapters/outbound/sqlanalytics`、`stats_handlers.go` |
| 配置 / Gateway / Catalog / Daemon 外部 IO | `internal/adapters/outbound/*` | `fileconfig`、`filegateway`、`catalog`、`daemon`、`piagentpackages` |
| 依赖方向检查 | `internal/archcheck` | 扫描根模块和 `desktop` 模块生产 Go 源 |

## 2. 分层和依赖方向

```text
Inbound（HTTP/CLI/TUI/Native 输入输出与协议适配）
        │ 调用
        ▼
Usecase（业务行为、事务边界、结果语义）
        │ 依赖 domain 和消费者侧最小接口
        ▼
Domain（纯业务模型与规则）
        ▲
        │ 实现 usecase ports
Outbound（文件、SQLite、上游 HTTP、进程、Pi 文件）
        ▲
        │ 显式装配
Bootstrap / composition roots（cmd、desktop）
```

当前目录对应关系：

```text
internal/domain/**                  纯规则：provider、gateway、protocol、limit、usage、catalog、conversation、revision
internal/usecase/**                 业务用例：providers、gateway、settings、analytics、packages、proxycontrol、inference、requestlog
internal/adapters/inbound/httpinfer  推理 HTTP 的客户端响应/SSE 适配
internal/adapters/outbound/**       文件、SQLite、上游 HTTP、daemon、Pi Agent 等外部系统适配
internal/bootstrap/**               具体 adapter 与 usecase 的显式装配
internal/server/**                  兼容 HTTP 入口和路由协调；已迁移的响应写入不在 outbound 中
internal/application/**             Desktop 与管理功能共用的应用服务/管理编排
```

静态规则由 `go run ./internal/archcheck` 检查，且同时扫描根模块和 `desktop` 模块：

- `domain` 只能依赖标准库和 `internal/domain/**`。
- `usecase` 只能依赖标准库、domain 和 usecase；不得依赖 HTTP、具体 adapter、文件或数据库实现。
- `adapters/inbound` 可以依赖 domain/usecase、同一 inbound 子树和入口所需外部库；不得依赖 outbound。
- `adapters/outbound` 可以依赖 domain/usecase、同一 outbound 子树和 IO 所需外部库；不得依赖 inbound。
- `bootstrap` 用于装配，不把独立 `desktop` module 引入根模块业务包。
- `internal/usecase/requestlog` 额外禁止 `database/sql`、`internal/store`、`internal/config`、`internal/server`；SQLite 和 Legacy 文件写入只能位于 `internal/adapters/outbound/requestlog`。
- 测试文件不计入生产依赖图；兼容包可以继续存在，但不得向新用例层新增越层依赖。

## 3. 共享入口和管理面

CLI/TUI、管理 API、Desktop Native 都通过共享的 application/usecase/domain 规则工作；Desktop 不把 HTTP handler 当作业务服务。历史 `pi-switch webui` 命令仍启动管理 API，以保持 CLI/API 兼容，但不再启动旧 SPA/WebView。

```text
cmd/pi-switch/main.go
  ├── CLI/TUI 输入
  ├── proxy / webui / gateway 等命令
  └── 根模块组合与兼容入口

internal/server
  ├── 管理 API 路由和错误信封
  ├── Proxy HTTP 路由
  └── 将输入交给 application/usecase；不直接执行已迁移的请求日志 SQL/文件追加

desktop/main.go + desktop/shell.go
  └── MyGo Native 窗口、导航和 UI 状态；调用共享 application/usecase
```

管理面保留既有协议：`/healthz` 对探针开放，`/api/*` 和认证行为不变；根路径返回退役提示，其他未知路径返回 JSON 404。管理面和推理面的错误 envelope 仍分别遵循既有合同。

## 4. Proxy / Inference 请求链

```text
客户端
  → internal/server（解码、认证、兼容路由）
  → internal/usecase/inference
      Decode → Route → Validate/Clamp → Protocol Plan
  → internal/adapters/outbound/proxy（构造并发送上游请求）
  → internal/adapters/inbound/httpinfer（status/header/body/SSE/Flush）
  → usage/cost 事实归一化
  → internal/usecase/requestlog
      ├── outbound/requestlog.SQLite
      └── outbound/requestlog.LegacyFile
```

`internal/adapters/outbound/proxy` 只负责上游请求构造、header 处理和 HTTP 调用；它不持有 `http.ResponseWriter`，不调用 `WriteHeader`、`Flush` 或写客户端 body。`internal/adapters/inbound/httpinfer` 保留客户端 status/header、SSE 字节顺序、flush 和取消语义。

`internal/usecase/inference` 负责流式/非流式共用的路由、协议计划、上游结果归一化和取消处理；HTTP handler 只做输入输出协议协调。裸 model ID、`/v1/*`、错误状态码、工具调用、多轮历史、限额和失败事实记录保持兼容。

Proxy 是独立进程。GUI 退出不隐式停止 Proxy；daemon 的 PID、身份、锁、健康检查和停止安全由 `internal/adapters/outbound/daemon` 与 `internal/usecase/proxycontrol` 共同维护。

## 5. RequestLog / Legacy Log

新请求记录由 `internal/usecase/requestlog.Service` 协调两个独立目标：

```text
requestlog.Record
       │
       ├── internal/adapters/outbound/requestlog.SQLite
       │       └── INSERT INTO requests
       └── internal/adapters/outbound/requestlog.LegacyFile
               └── requests.log 追加
```

两个目标独立尝试：SQLite 失败仍尝试 Legacy Log，Legacy Log 失败不撤销 SQLite；错误被记录，不把单一兼容日志目标失败直接改写成推理 API 失败。`internal/bootstrap/requestlog.go` 负责取得 DB、配置路径并装配具体 adapter。

`internal/server/legacy_log.go` 只保留旧 `requests.log` 的启动/显式历史导入兼容逻辑，统计读取走 SQLite；新请求的 Legacy 追加不在该文件或 handler 中实现。

## 6. Analytics / Stats / Sessions

```text
internal/server/stats_handlers.go
  → internal/usecase/analytics.Service
      ├── 分页、时间窗口、排序、Total、Stats/Conversations 语义
      ├── conversation domain matcher
      └── 可选 AggregationFact 快路径
  → internal/adapters/outbound/sqlanalytics.Repository
      └── SQLite 查询、分页边界和聚合事实扫描
```

Analytics usecase 不依赖 SQLite。`sqlanalytics` 负责 SQL、NULL 读取和分页；极端 page/limit 在结果边界或 SQL offset 计算前被安全处理，不进行由用户输入驱动的异常大容量预分配。正常分页、Total、排序和空数据语义保持不变。

W24 的聚合路径使用轻量 value-based `AggregationFact`，仅用于 Stats/Conversations 聚合；完整 `RequestFact` 仍用于分页、DTO 和导出等需要保留 NULL 语义的路径。该优化不新增第二个事实源，也不改变统计含义。

## 7. Gateway、Provider 和其他外部系统

### Gateway

`internal/domain/gateway` 负责 canonical plan、校验、diff/merge 等规则；`internal/usecase/gateway` 负责 Preview、Catalog enrich、Validate 和显式 Publish 编排；`internal/adapters/outbound/filegateway` 负责读取 current、锁、临时文件和原子替换；`internal/adapters/outbound/catalog` 负责 models.dev 缓存 IO，`internal/domain/catalog` 负责快照解析和补齐规则。

- Generated 使用 `FillOverwrite`；Draft 使用 `FillMissing`。
- `models.json` 只在显式 Publish/Apply 后写入，不因 config 修改自动发布。
- 第三方 provider 保留，冲突阻止发布，最终文件权限和原子替换合同保持不变。

### Provider / Config

`internal/domain/config`、`internal/domain/provider` 保存 schema 和规则；`internal/usecase/providers` 负责 CRUD、凭证处理、模型抓取和 revision 语义；`internal/adapters/outbound/fileconfig` 负责锁、备份、权限和原子保存。旧 `internal/config`、`internal/profile` 仍作为兼容实现/桥接存在，不能被文档误认为新的职责归属。

### Packages / Sessions / Daemon

- `internal/usecase/packages` + `internal/adapters/outbound/piagentpackages`：包发现、导入和 Pi Agent 目录扫描。
- `internal/adapters/outbound/daemon` + `internal/usecase/proxycontrol`：进程身份、PID、启停和健康。
- `internal/application`：Desktop/管理入口共用的高层应用编排，不是外部 IO 存储层。

## 8. 核心持久化位置

| 数据 | 当前职责归属 | 用途 |
|---|---|---|
| `~/.pi-switch/config.json` | `domain/config` + `outbound/fileconfig`（`internal/config` 为兼容入口） | 供应商、渠道、模型池、暴露集 |
| `~/.pi/agent/models.json` | `domain/gateway` + `usecase/gateway` + `outbound/filegateway` | Pi 已发布 Gateway provider 视图 |
| `~/.pi-switch/requests.db` 请求记录 | `usecase/requestlog` + `outbound/requestlog` | 请求事实、usage、cost、latency |
| `~/.pi-switch/requests.db` 统计读取 | `usecase/analytics` + `outbound/sqlanalytics` | Stats、Conversations、Sessions 查询 |
| `~/.pi-switch/requests.log` | `outbound/requestlog.LegacyFile`；`server/legacy_log.go` 负责历史导入 | Legacy 双写和旧日志迁移 |
| `~/.pi-switch/cache/models-dev.json` | `domain/catalog` + `outbound/catalog` | models.dev 元数据缓存 |
| `~/.pi-switch/backups/` | `usecase/settings` + `outbound/fileconfig` | 配置备份/恢复 |
| `~/.pi-switch/*.pid/*.log` | `usecase/proxycontrol` + `outbound/daemon` | Proxy/daemon 生命周期 |
| `~/.pi/agent/sessions/` | `outbound/piagentpackages` | Pi session 只读扫描 |

## 9. 修改边界速查

1. 改 Provider、Channel、Model 暴露：从 `internal/usecase/providers` 和 `internal/domain/provider` 入手；HTTP/CLI 只做输入和错误映射。
2. 改 Gateway：从 `internal/domain/gateway` / `internal/usecase/gateway` 入手；文件发布只改 `outbound/filegateway`。
3. 改请求协议和流式行为：业务规则在 `internal/usecase/inference`，上游 IO 在 `outbound/proxy`，客户端响应适配在 `inbound/httpinfer`。
4. 改请求日志：从 `internal/usecase/requestlog` 入手；SQLite/Legacy 文件实现只能在 `outbound/requestlog`。
5. 改 Stats/Sessions/Export：从 `internal/usecase/analytics` 入手；SQL 只在 `outbound/sqlanalytics`。
6. 改 Native Desktop：从 `internal/application` 和页面操作入手，不调用 HTTP handler 作为业务服务。
7. 改 API 错误信封或兼容路由：从 `internal/server` 入手，但不得把持久化实现塞回 handler。
8. 改依赖边界：先更新 `internal/archcheck` 的规则和负例测试，再改生产包；最终必须输出 `architecture dependencies: clean (2 modules)`。
