# spec: architecture-review 工程项批次（A4 / A10 / A11 / A13 / A14）

**来源**：`docs/architecture-review.md` §3 中五项低风险、判据明确的工程项。它们不属于 `trust-boundary-and-honesty`（那批是 P0 信任边界与诚实性），故单独成 spec。

**范围**：五项一起收敛，因为它们互不依赖、都不改产品行为（唯一例外是 A10 改变了失败时的返回码/退出码，这是本批的主目的），可共用一次全量验证。

| 项 | 问题 | 判据 |
|---|---|---|
| **A10** | 保存失败静默吞掉：`_ = saveConfig(cfg)` 六处（profile 5 + settings 1）与 CLI 两处，随后仍返回 `200 {"ok":true}` / 打印成功 | 写盘失败时 handler 返回 5xx、CLI 非零退出且不打印成功；全仓 `_ = saveConfig` 归零 |
| **A4** | `retry.go` 读起来像生效中的策略引擎 | 文件头说明"当前休眠、代理路径为单候选直通、复用前提见 spec"；架构文档补同一事实 |
| **A11** | CI `go-version: "1.23"` 与 `go.mod` 的 `go 1.24.2` 不匹配，依赖 `GOTOOLCHAIN=auto` 隐式下载 | CI 不再隐式下载工具链；版本以 `go.mod` 为单一事实来源 |
| **A13** | `docs/adr/` 下两个 `0004-*` 并存，引用有歧义 | 编号唯一且连续 |
| **A14** | README 徽章 `20260902.0.0` 与 `package.json` 的 `20260910.0.2` 漂移 | 两者一致，或不一致会被 CI 检出 |

## 实施前核实的事实

- A10：`_ = saveConfig` 实测 6 处（`profile_handlers.go` 5 + `settings_handlers.go` 1），CLI 侧 `_ = saveConfigFile` 2 处（`provider use` / `provider delete`）；`config.SaveAtPath` 会如实返回写盘错误，`saveConfig` 原样传播——所以传播链是通的，只是被调用方丢弃。
- A4：`retry.go` 的**调度引擎在生产代码中零调用**（`expandAttempts`/`admitForRound`/`waitForRound`/`classifyUpstreamError` 等只被本包测试引用），但两个校验函数仍在用（`validateRetryFields` 被 profile 三处调用、`validateSettingsRetry` 被 settings 与 profile 调用）。注释必须同时说明这两件事，否则会误导成"整个文件都是死代码"。
- A13：按引入时间排序为 `0004-responses-provider-passthrough`（2026-08-07）< `0005-models-dev-...`（08-28）< `0004-supplier-side-pi-session-scan`（08-31）。故后者重编为 `0012`，编号与时间同时恢复单调。
- A14：徽章指 `20260902.0.0`，`package.json` 为 `20260910.0.2`。
- A11：`ci.yml` 三处 `go-version: "1.23"`；`go.mod` 为 `go 1.24.2`。

## 明确不做

1. 不删除 `retry.go` 的休眠引擎（`remove-failover-chain/spec.md` D1 明确保留供后续复用），只加说明。
2. 不为 A10 引入"保存前备份"或重试写盘等新机制，只让错误可见。
3. 不回改 `.scratch` 下历史 spec 里指向旧 ADR 编号的引用（历史记录按惯例不回改）。
4. 不做 `docs/architecture-review.md` 中本批之外的项（A5/A6/A12/A15 与三个"200 但无工作"端点）。
