# 请求状态与错误事实丢失
Status: resolved

## Requirements
- 请求完成时已知的 status、error、upstream URL 写入 SQLite 并原样用于 Stats 最近请求、对话明细及 JSON/CSV 导出。
- 不从 success 布尔值推断 200/500；旧 SQLite 缺失字段保留 NULL，成功的空 error 为 NULL。
- 新导入的 legacy 行保留其中已存在的状态、错误和 URL；读取仍不触发导入或历史改写。
- schema migration 可重入，原有行及事实不变。

## Validation
- public HTTP 原路径先红后绿，覆盖非流/SSE 401、429、503 和非流成功 201；Stats、明细、JSON/CSV 保存相同状态、原始错误与 URL。
- 新 legacy 已知/缺失事实与旧 schema 重复 Open 迁移测试通过；store/stats/server 包通过。
- Standards / Spec 审查均未发现遗留问题。
