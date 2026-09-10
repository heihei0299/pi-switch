# 09: 成本计算单一实现

**What to build:** 成本计算只剩 `internal/proxy.CalcCost` 一份实现且被真实调用，money path 不再有行为分叉。端到端行为：代理请求日志中的消费值与统计聚合口径不变（(输入−缓存)×输入单价 + 缓存×缓存读取单价 + 输出×输出单价，nil 单价记 unknown），但这条链路只由一份代码产出。

**实现要点**（spec D6）：

- `internal/proxy.CalcCost` 改为指针入参（nil entry 返回 nil），成为唯一实现
- `proxy_handlers.go` 中的调用点直接调用 `proxy.CalcCost`（spec 记为 6 处；实现时以实际调用点为准）
- 删除 `internal/server` 内的 `computeCost` 副本
- `internal/proxy/cost_test.go` 随签名更新

**保留的边界用例**：nil 单价、缓存命中大于输入（负值裁剪）、常规三档——三个 case 不得在签名调整中丢失。`internal/proxy` 由此获得真实调用者，为后续抽 proxy core 预留落点。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] `proxy.CalcCost` 指针签名就绪：nil entry 返回 nil，负的非缓存输入裁剪为 0，常规输入按 1M 单价折算
- [x] `internal/server` 内不再存在 `computeCost`，全部调用点直连 `proxy.CalcCost`
- [x] `internal/proxy/cost_test.go` 随签名更新且保留 nil / 负值 / 常规三个 case
- [x] 消费行为不变：既有请求日志与统计相关测试零改动通过，无需更新既有 API 契约测试
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包；`internal/server` 7.50s）
- [x] 不新增 service 接口层、不改统计 JSON 契约（spec D10）

## 实施记录

- 提交：`efa42ab`
- `internal/proxy/cost.go`：`CalcCost` 改为 `entry *config.ModelEntry` 指针入参，nil entry 与 nil cost 均返回 nil，成为唯一实现
- `internal/server/proxy_handlers.go`：6 个调用点直连 `proxy.CalcCost`（`handleChatCompletions` 2 处、`handleStream` 2 处、`streamPassthrough` 1 处、`streamConvert` 1 处），删除 `server.computeCost`，新增 `internal/proxy` import
- `internal/proxy/cost_test.go`：3 处调用改为 `&me`；新增 `TestCalcCost_NilEntryYieldsNil` 覆盖新增的指针语义；原有 nil cost / 负值裁剪 / 常规折算三个 case 全部保留并通过
- `internal/proxy` 由此获得真实调用者（此前 `CalcCost` 零调用），为后续抽 proxy core 预留落点
- 验收命令：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出
- 事实核对：`computeCost` 的调用点是 **6 个**，spec D6 的计数正确；本 spec 早期记录的「实际 5 个」源于一处用 `\b` 词边界匹配的 grep 漏掉了 `handleChatCompletions` 嵌套分支内的一处
