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

## 实施结果（2026-09-11）

5 张票全部 resolved，各一个提交：`4339e6e`(add)、`54ff461`(duplicate)、`5caeb40`(test)、`9d38bc8`(fetch-models)、`df947dd`(expose + 文档)。

**超出原计划的一项**：实施中发现 `provider expose` 在两个 README 里被宣传、且有 6 处示例，但它既不在 help 里也未被接线。与其在多处加"仅 WebUI/API"的说明，不如把它一并接线（服务端 `handlePutExpose` 早已实现），故本批实际接线 **6 个命令**。

**过程中发现并修掉的真问题**：

1. **`provider add --base-url` 建不出可路由的 profile**：顶层 `baseUrl` 不构成渠道，路由与 expose 都以 `upstreams[]` 为单位。此前新建的 profile 没有任何渠道，`fetch-models`/`expose` 都无从操作。已改为给了 `--base-url` 就同时建一个名为 `main` 的渠道，并新增 `--models` 让该渠道自带模型池。
2. **`expose` 的报错顺序**：未知 profile 时先抱怨缺 `--channel`，掩盖了真正原因。已改为先判 profile。
3. **测试断言用错文案**：CLI 对未知 profile 用 `unknown profile %q`（与 show/use/delete 一致），服务端才用 `not found`。我两次把测试写成断言服务端文案，两次都按 CLI 的既有惯例改了测试而非实现，并把这一点写进断言注释。

**机械核验**：脚本枚举两个 README 中出现的每个 `provider <sub>` 并真实执行，**9/9 可识别**（此前 add/duplicate/test/fetch-models/expose 五个报 unknown subcommand）。

**未收敛项**：handler 的 `handleFetchModels`（无渠道分支）仍保留自己的 JSON 解析拉取逻辑，与 CLI 用的 `fetchUpstreamIDs`/`FetchUpstreamModelIDs` 是两份实现；本票为控制风险只接线未重构它，两者语义一致（都是打 `/models` 与 `/v1/models`、Bearer 鉴权）。建议后续单独立票把 handler 也切到同一原语。
