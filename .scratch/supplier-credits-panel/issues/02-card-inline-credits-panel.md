# 02 — 供应商卡片内联余量面板

**What to build:** 在 `ProfilesPanel` 每张 opencode-go 供应商卡片内联余量区，挂载自动查询、手动刷新、四字段与进度条展示、错误内联重试，非命中不渲染。

**Blocked by:** 01 — 后端余量代理与归一化抽象

**Status:** resolved

- [x] opencode-go 供应商卡片显示余量区：余额/已用/总额/过期 + 进度条(percent) + 刷新按钮，小字注明“主上游”
- [x] 卡片挂载时自动调一次 `GET /api/profiles/:name/credits`，刷新按钮可重调，loading 显示 spinner
- [x] 查询失败时卡片内联显示错误文案与重试按钮，不弹全屏 toast，不阻断编辑/删除/暴露
- [x] 非 opencode-go 供应商卡片不显示余量区，保持简洁
- [x] 多上游供应商卡片仅展示主上游余量，不为每条 upstream 列表
## 实施总结
- 提交：`9b678a5` — `feat(credits): 供应商卡片内联余量面板 (#02)`；文档对齐：`016c37a` — `docs: align WEBUI_GUIDE with credits proxy (#02)`
- 实现的 seams：
  - S1 `api.getCredits(name)` → `GET /api/profiles/:name/credits` 归一化返回
  - S2 `isCreditsSupported(profile)` 主上游 baseUrl 含 opencode.ai 判定（大小写不敏感，仅 resolvedUpstreams()[0]）
  - S3 余量区展示：余额/已用/总额/过期 + percent 进度条 + 刷新按钮 + 主上游小字
  - S4 挂载自动查询与手动刷新（含 loading spinner）
  - S5 查询失败内联红字+重试，不弹 toast，不阻断编辑/删除/暴露
  - S6 非命中不渲染 + 多上游仅主上游单面板单请求
- 验收标准：
  - [x] opencode-go 供应商卡片显示余量区：余额/已用/总额/过期 + 进度条(percent) + 刷新按钮，小字注明“主上游”
  - [x] 卡片挂载时自动调一次 `GET /api/profiles/:name/credits`，刷新按钮可重调，loading 显示 spinner
  - [x] 查询失败时卡片内联显示错误文案与重试按钮，不弹全屏 toast，不阻断编辑/删除/暴露
  - [x] 非 opencode-go 供应商卡片不显示余量区，保持简洁
  - [x] 多上游供应商卡片仅展示主上游余量，不为每条 upstream 列表
- 测试结果：webui 196 项全绿（新增 19：credits 4 + api.credits 3 + SupplierCreditsPanel 7 + ProfilesPanel.credits 5），cargo 318 项全绿（--test-threads=1 稳定；并发时偶发文件锁竞争重试后全绿）
- typecheck：通过（`npm --prefix webui run typecheck` + `cargo check` 视改动通过）
- 文档对齐：已更新 `WEBUI_GUIDE.md` REST ↔ core map 新增 `GET /api/profiles/:name/credits`；README 架构中 `credits.rs` 已存在，无需更新
- 遗留 / 后续建议：codex 分支待后端 CodexFetcher 扩展，前端零改动；当前仅 opencode.ai 命中，后续可扩展 isCreditsSupported 分支
