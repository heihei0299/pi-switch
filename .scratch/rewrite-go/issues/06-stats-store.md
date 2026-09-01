## Question

Go 重写中统计与存储层如何设计？保留 SQLite 还是重构为通用 store 接口？

pi-switch 现有 `stats.rs` (122KB) + `database.rs` 使用 rusqlite + SQLite 存储 requests.log，用量解析通过 StreamTee/SseUsageParser，统计查询在 Rust 中完成。CLIProxyAPI 使用 `internal/store` + `internal/cache` 等更通用的存储抽象，支持多种后端。

需要决策：
1. 存储后端：保留 SQLite（rusqlite→modernc.org/sqlite 或 mattn/go-sqlite3）/ 改为纯文件 JSONL / 引入通用 store 接口？
2. 用量解析：StreamTee / SSE 解析逻辑如何迁移到 Go？
3. 查询与导出：stats 查询、export_logs_json/csv、webui stats 面板如何迁移？
4. Session 归因：现有 sessionScan（扫描 ~/.pi/agent/sessions）逻辑是否保留？与 proxy 的解耦方式？

## Type

grilling

## Status

resolved

## Answer

1. **存储后端**: 保留 SQLite + requests.log 结构，统计查询、导出逻辑 1:1 移植到 Go，零迁移。
2. **驱动**: modernc.org/sqlite（纯 Go，无 CGO），交叉编译简单，与 Go 交叉编译矩阵兼容。
3. **用量解析**: StreamTee / SseUsageParser 逻辑移植到 Go（internal/stats/stream.go），保持与代理 tee 路径一致。
4. **查询与导出**: stats 查询、export_logs_json/csv、WebUI stats 面板逻辑完整迁移。
5. **Session 归因**: 保留现有 sessionScan（扫描 ~/.pi/agent/sessions JSONL）逻辑，与 proxy 解耦，通过 settings.conversationSource 控制。
