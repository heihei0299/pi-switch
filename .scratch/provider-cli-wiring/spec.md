# spec: provider CLI 接线（把 help 与 README 承诺的命令接到已有实现）

**来源**：`docs/architecture-review.md` A2（CLI 假成功）的同族遗留。票 03 只覆盖了 `package`/`ccs`/`presets`/`stats`/`doctor`，`provider` 一族未处理。

**已核实的事实**（2026-09-11，在隔离环境构建真实二进制逐条实测）：

- `pi-switch provider --help` 宣布 8 个子命令：`list|show|add|delete|duplicate|use|test|fetch-models`
- **实际只识别 3 个 + 2 个未宣传的**：`list`、`show`、`delete`、`use`、`remove`；`add`/`duplicate`/`test`/`fetch-models` 一律返回 `unknown provider subcommand` + 退出 1
- `README.md:94-99` 宣传 `provider add <name> [--preset <id>] [--api-key <key>]`、`provider expose <name> <model-ids...>`、`provider fetch-models <name>`——其中 `expose` **连 help 都没列**
- **关键在于服务端这五个能力都是真实实现的**：`POST /api/profiles`、`POST /api/profiles/:name/duplicate`、`POST /api/profiles/:name/test`、`POST /api/profiles/:name/fetch-models`、`PUT /api/profiles/:name/expose`
- `provider fetch-models <name>` 与 `provider test <name>` 会发起**真实上游请求**（要 apiKey、会消耗上游额度）；`test` 是只读探测，`fetch-models` 成功后会把模型合并落盘

## 判据（沿用票 03 的那把尺子）

1. 能接已有实现的**接线**，不为 CLI 另写一套逻辑：抽 handler 的核心函数供两边共用。
2. 未实现的（若有）→ stderr 说明 + 非零退出 + stdout 不含成功文案。
3. help 与 README 只列真实可用的东西；`expose` 要么接线要么从 README 移除。
4. 每个命令至少一条断言真实副作用的测试；失败路径断言非零 + stderr 有原因。
5. `go build ./...`、`go test ./... -count=1` 全绿、`gofmt -l` 无输出。

## 明确不做

- 不改服务端的路由、认证、JSON 契约（D10 精神）。
- 不为 CLI 引入交互式 TUI 选择器：`add` 用 flag 表达（`--preset/--api-key/--base-url/--api`），保持可脚本化。
- 不在 CLI 侧做「保存前备份」等新机制。
