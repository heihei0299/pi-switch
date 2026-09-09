# 旧版本统计数据兼容（Go 重写版读取 requests.log 历史）

Status: ready-for-agent

## Problem Statement

Go 重写后的代理只把请求记入 SQLite（`requests.db`，当前仅 5 行），不再读写旧版本的请求日志（`requests.log`，存量 23394 行、9.6MB）。统计页、对话统计、请求明细、导出四个接口全部只查 SQLite，导致升级后历史 Token 使用量与消费统计全部不可见，等同于统计清零。重写 spec 承诺的“`requests.log` 追加保留、旧行无消费字段兼容、零迁移”在当前实现中缺失。

## Solution

Go 版恢复对请求日志的读写兼容：启动时把请求日志中的历史行幂等导入 SQLite（去重后插入，坏行跳过），新请求恢复双写（SQLite + 请求日志，沿用旧字段形状）。导入后统计、对话、明细、导出四个接口无需改口径即可看到全部历史。

## User Stories

1. As a pi 用户， I want 升级 Go 版后统计页仍能看到升级前的总请求数与 Token 使用量， so that 历史消费审计不丢失
2. As a pi 用户， I want 升级前的对话统计仍可按对话查看， so that 单次对话成本可回溯
3. As a pi 用户， I want 请求明细与导出（JSON/CSV）包含升级前的请求， so that 离线分析完整
4. As a pi 用户， I want 升级前的请求日志文件原样保留、不被改写或删除， so that 降级回旧版仍可用
5. As a pi 用户， I want 重复重启代理不会产生重复统计， so that 数据口径稳定
6. As a pi 用户， I want 旧日志中缺消费字段的行在统计中显示为未知而非报错或按现价重算， so that 历史消费不被篡改
7. As a pi 用户， I want 新产生的请求继续追加进请求日志， so that 旧工具链与备份逻辑继续有效

## Implementation Decisions

- **接入方式**：启动时一次性幂等导入，而非每次查询时合并——查询热路径保持只读 SQLite，23k 行文件扫描只发生在启动时；导入以（时间、供应商、模型、输入、输出 token）自然键去重，不存在即插入。
- **双写恢复**：新请求同时写入 SQLite 与请求日志；日志行沿用旧字段形状（`ok/status/error/upstreamUrl/promptTokens/completionTokens/cachedTokens/reasoningTokens/costTotal/conversationId/conversationName`），缺消费时省略 `costTotal`。
- **字段映射**：`ok→`成功标志，`promptTokens/completionTokens/cachedTokens/reasoningTokens→`对应 token 列，`costTotal→`消费列（缺失记 unknown/NULL，前端显示 `-`），`conversationId/conversationName→`对话标识列，旧日志无延迟字段记 NULL（聚合时跳过，沿用现有语义）。
- **统计口径不变**：仅成功且含输入/输出 token 的行参与求和，未标记对话归 `unlabeled`，对话查询时 join（sessionScan）逻辑不受影响。
- **失败容忍**：坏行（非 JSON/缺关键字段）跳过并计数，不阻断启动；导入失败不阻断代理启动，仅记录日志。

## Testing Decisions

- **好的测试**：只测外部行为——给定 fixture 旧格式日志 + mock 上游，断言统计接口总数包含旧行、导出包含旧行、新请求在两处存储同时落盘；不测内部解析函数与私有状态。
- **缝（Seam）**：单一最高缝——Go 二进制的外部行为（管理 API 的统计/明细/导出 + 文件系统 `requests.log`/`requests.db` 落盘结果）。覆盖全部决策的外部可观测行为。
- **先例**：管理 API 集成测试（`httptest` mock 上游 + 临时配置目录隔离 `PI_SWITCH_CONFIG`/`PI_SWITCH_DB`），WebUI 侧统计面板测试仅验证展示、不碰导入逻辑。

## Out of Scope

- 不做 `config.json` 旧字段迁移（已有版本升级逻辑覆盖）
- 不碰 `pi-switch.db` 的 package 相关表
- 不按现价重算旧行的消费（缺失即 unknown）
- 不回填旧行的对话归因与延迟（查询时现有逻辑处理）
- 不删除、压缩或改写旧请求日志文件

## Further Notes

- 术语以 `CONTEXT.md` 为准：请求日志、请求明细、Token 使用量、消费、统计窗口、对话/未标记。
- 与 ADR-0004 一致：请求日志维持追加式不可变语义，导入只读不写回。
