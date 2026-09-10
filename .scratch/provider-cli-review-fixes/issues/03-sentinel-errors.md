# 03: 服务端错误分类改用哨兵，恢复 `as required`

Status: resolved (2026-09-11)

**What to build:** profile 域导出 outcome 哨兵（`ErrProfileNotFound` / `ErrProfileExists` / `ErrUnknownChannel` / `ErrUpstreamFetchFailed` / `ErrPersistFailed` / `ErrTargetNameRequired`），核心函数返回带 kind 的错误，handler 用 `errors.Is` 映射状态码，不再 `strings.Contains(err.Error(), ...)`。

**为什么**（评审 Standards 硬性结论，已独立复核）：`profile_handlers.go:367/369/872` 用错误文本分类。同仓库 `package_handlers.go:187` 的 `ErrPackageNotFound` 注释写明该 sentinel 存在的理由就是让调用方**不必从错误字符串去猜**，且同批次的 package 域与 CLI 都已用 `errors.Is`。具体后果：文本里含 "not found" 的**持久化**错误会被答成 404 而不是 500（分类顺序上 not-found 分支先命中）。

同时修复两处派生的契约偏移：
- `DuplicateProfile` 把 CLI 措辞 `--as <new> required` 泄进了 HTTP 400 body（改动前是 `as required`）——核心回到 `as required`，CLI 自己给 `--as` 提示。
- `isPersistError` 靠 `*os.PathError`/`*os.LinkError` 类型嗅探，只对今天的 `SaveAtPath` 成立；改为哨兵后未来任何包装过的持久化错误不会静默降级 500→400。

**Seam**：真实 HTTP（`NewMgmtRouter` + `callMgmt`）。

**验收**
- [ ] `POST /api/profiles/:n/duplicate` 缺 `as` → 400 `{"error":"as required"}`（恢复改动前文案）
- [ ] 持久化失败且错误文本含 "not found" → **500**，不是 404
- [ ] 未知渠道 → 400 `unknown channel "x"`；未知 profile → 404 `not found`（文案不变）
- [ ] fetch-models 上游失败仍是 500 且原因可见（`api_contract_test.go`、`channel_fetch_test.go` 保持绿）
- [ ] 代码中不再有按错误文本分类的 `strings.Contains(err.Error(), ...)`

**测试**：`internal/server/profile_error_mapping_test.go`（复用 `unwritableConfig` 的注入思路，目录名含 "not found" 以复现脆弱性）。
