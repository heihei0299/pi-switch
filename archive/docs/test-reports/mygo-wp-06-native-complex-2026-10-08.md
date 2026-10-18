# WP-06 Native complex progress — initial and follow-up 2026-10-08

## Current formal GUI gate status (2026-10-11 update)

**Task 2 real-GUI acceptance: PENDING (not run in this continuation; not PASS).** Wayland/IME, tray, display scaling, and controlled Proxy lifecycle acceptance remain pending. The existing Ubuntu/Xvfb interaction rows below are preserved as historical supplemental evidence and do not satisfy the formal GUI gate.

**Status: IN PROGRESS / Gate C NOT PASSED.** Complex native screens are opt-in prototypes. Gateway previews real config/models without writing and has separate explicit-confirmation publish actions for selected models and validated JSON drafts; Stats uses the same configured service as the management API. Existing React/WebView pages remain authoritative and available.

## Component assessment

| Component | Prototype result | Decision |
| --- | --- | --- |
| JSON editor | MyGo TextArea preserves selection, undo/redo, composition and invalid text; syntax errors report Unicode-aware line/column and color the entire error line; Load safe Gateway draft omits persisted third-party/config credentials, validation uses shared `gateway.BuildDraftPlan`, and a valid edited draft can be published after confirmation. | Not equivalent yet: no line-number gutter, background highlight or complete conflict-resolution workflow. Keep React as the real editor. |
| Gateway draft/diff | Native preview uses shared generated/canonical-plan services, supports selected-model subsets, shows provider-level added/removed/changed diff for JSON drafts, and publishes only after confirmation through `gateway.PublishPlan`; conflicts/no-op are blocked without rendering persisted credentials. | Still not equivalent: conflict resolution and parity with the existing editor/diff experience are incomplete. Keep React. |
| Usage/session view | Native view uses the configured management stats service for SQLite totals, failed count, token totals, cache rate, average latency and expandable virtualized provider/model summaries; a labeled horizontal bar chart shows provider request share; it has independent Today/24h/7d/Custom/All-time filters and paging for requests/conversations, plus all-time paged per-conversation details; recent requests show timestamp, status and per-request token/cache/cost dimensions; an isolated JSONL fixture verifies `sessionScan`; unknown facts remain nullable. | Not equivalent: current WebUI Stats is KPI/table based (no time-series chart found); native still lacks WebUI auto-refresh and JSON/CSV export, detailed failure error text and physical GUI/session-directory evidence. Keep React. |
| Packages | Native prototype uses the shared package registry for Pi Agent import, spec add, enable/disable and confirmed soft uninstall; list loading does not create a missing DB and git URL credentials are omitted from display. | Still not equivalent: React remains the production route pending physical GUI/accessibility and full regression evidence. |

The prototype window is reachable from the app/tray menus. Gateway preview reads local config and `models.json` without writing; publication is a separate explicit-confirmation action. Stats uses the same configured service as the management API; a missing request database returns an empty state without creating it. The JSON editor loads a generated draft containing only pi-switch-owned providers and validates without writing. Package registry actions use the shared backend; no persisted package ID/spec URL credentials are rendered.

## Verification

| Check | Result |
| --- | --- |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test -mod=readonly ./internal/server` | PASS; shared package toggle service included |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `./node_modules/.bin/mygo vet .` (`desktop/`) | PASS |
| `outdir=$(mktemp -d /tmp/pi-switch-wp06.XXXXXX) && GOTOOLCHAIN=local GOPROXY=off go -C desktop build -mod=readonly -o "$outdir/pi-switch-desktop" .` | PASS; output kept outside the worktree |
| UI tester: select-all/edit, invalid JSON preservation, undo/redo, Chinese IME composition, zero-valued Gateway metadata, unknown usage/cost, next-page state, and session outline | PASS |
| UI tester: invalid JSON reports the correct line/column after a preceding multibyte Chinese character, while retaining the draft | PASS |
| JSON syntax diagnostic highlights the correct whole line after multibyte text, including incomplete EOF lines, and renders it in the theme danger color in the UI tester | PASS |
| Native Packages UI with isolated Pi Agent root and DB: missing-DB list read creates no DB, imports configured package/capabilities, adds spec, toggles enabled state, cancel preserves package, confirmed uninstall soft-removes it; credential-bearing git URL display name is reduced to repository basename | PASS |
| Gateway JSON draft: loaded generated pi-switch-only draft, verified persisted config/models credentials are absent, validated success and unsupported-provider conflicts, and confirmed files remain byte-identical | PASS |
| Temporary SQLite + JSONL integration: 1,000 recent + 1 older request, token/cache/latency summary metrics, independent request/conversation preset and custom date filters, 10-row request/conversation/per-conversation-detail paging, unknown token/cost retention, sessionScan attribution, unchanged fact-row count/session file, and no DB creation when missing | PASS |
| Temporary SQLite provider/model aggregation: deterministic provider/model summaries, plus a 1,000-model UI list checked for virtualization and End-key navigation | PASS |
| Separate isolated SQLite fixture with one successful and one failed request; verified failure count, success-only token/cost aggregation, rendered summary and unchanged row count | PASS |
| UI tester: custom request/conversation date inputs, inclusive date windows, changed-date refresh, and reversed-range error while retaining the last successful snapshot | PASS |
| Native conversation request formatting: status and token dimensions shown; unknown values preserved; raw provider error text omitted | PASS |
| Native recent-request list displays timestamp and success/status for a failed request; raw error body remains omitted | PASS |
| Native recent-request list formats input/output/cached/reasoning/cache rate/total/cost and retains unknown dimensions | PASS |
| Provider request-share bar chart reports 75%/25% for isolated 3:1 fixture and shows an empty state when totals are zero | PASS |
| UI tester: shared generated Gateway plan, provider/model status summary, credential omission, and unchanged temporary `config.json`/`models.json` bytes | PASS |
| Gateway selection/publish UI: selected one of two exposed models, verified subset preview, cancel-confirmation no-write, confirmed publish of only the selected model, conflict-rejection no-write, third-party provider preservation, credential omission, and unchanged config | PASS; all writes used temporary `PI_SWITCH_MODELS`/config paths |
| Gateway JSON-draft preview/publish UI: rendered shared provider-level diff, kept third-party providers out of removals, canceled with unchanged `models.json`, confirmed publish, verified revalidation clears the diff and blocks no-op confirmation, then rejected a conflicting draft without changing config/published models | PASS; all writes used temporary paths |

## Not verified / remaining

- No interactive native window, real IME, screen reader, cross-platform behavior or performance measurements were tested in this TTY session. JSON syntax errors now color the entire error line, but the editor still has no line-number gutter or background highlight.
- Gateway generated preview reads real config and `models.json`; automated temporary-file tests now cover conflict diagnosis, restoring a valid draft, cancel, and confirmed publish. Physical Wayland preview/edit/resolve behavior remains unverified. The JSON editor has line/column diagnostics and error-line color, but no separate line-number gutter; no screen-reader or visual contrast check was run. Stats sessionScan attribution, aggregate summaries, virtualized provider/model lists, independent preset/custom range filtering, and per-conversation request paging/nullable token fields are tested against isolated SQLite/JSONL fixtures, but physical GUI behavior against a real session directory and detailed failure UI remain unverified. Current WebUI Stats source is KPI/table based with auto-refresh and JSON/CSV export; no time-series chart component was found. Existing-schema migration behavior was not exercised; tests verify no fact rows changed and no missing DB was created.
- Gate C remains blocked by missing functional parity evidence. Do not retire React/WebView or present these prototypes as migrated production pages.
- Full repository tests remain deferred until WP-05 through WP-08 are complete.

## 执行计划任务 1/2 自动化实现进度（2026-10-11）

**状态：实现有进展，验收仍 BLOCK；任务 1/2 均未关闭。** 本节更新上文的自动化证据，但不把 UI tester 结果冒充真实 Wayland 操作。

| 范围 | 本轮改动与证据 | 未通过/未验证 |
| --- | --- | --- |
| 任务 1：JSON 编辑器 | 新增粘贴覆盖选区测试；同一 UI tester 流程完成安全草稿预览 → 粘贴冲突草稿（诊断、无发布且文件字节不变）→ 粘贴有效草稿 → 取消（无写入）→ 确认发布。既有测试继续覆盖撤销/重做、非法草稿保留、中文 composition、Unicode 行列和错误行着色；写操作均使用临时文件 | 错误行用行/列状态和整行文字着色定位，尚无独立 gutter，真实环境下的视觉可读性和 Wayland/fcitx5 IME 未验证。UI tester 不替代计划要求的真实 Preview → Conflict/Cancel → Apply。任务 1 不关闭 |
| 任务 2：Stats 导出 | `internal/stats.Service.ExportRequests` 增加全历史 JSON/CSV 导出，按请求 ID 正序，并保持旧 WebUI 的 JSON 别名和 CSV 列合同；只导出原始 SQLite 归属，不读取或推断 Pi session。HTTP 旧导出入口也改为复用该服务。Native UI 增加导出按钮与敏感字段提示；同目录临时文件写入后原子 rename，新文件权限为 `0600`，写失败不覆盖已有导出 | 导出的 error/upstream URL 可能包含敏感信息，UI 已提示但仍需使用者确认保存位置；未做真实 GUI 导出操作 |
| 任务 2：Stats 自动刷新 | Native 提供 Off/5s/30s/5min，默认 Off；仅 Stats 页面显示时用 MyGo `Context.After` 调度，并复用当前筛选与分页；加载中跳过本轮，避免并行刷新。UI tester 覆盖四档映射、到期刷新、当前窗口透传、无重叠及 Off 停止 | 目标 GUI 主机尚不可达，真实 GUI 交互未验证 |
| 任务 2：Packages 长列表 | UI tester 用 100/1000 项夹具验证单屏最多 24 行、点击后列表获得焦点、滚轮可到列表末尾及 End 键导航到末项；列表绑定选中项 | 未在真实 GUI 测滚动、焦点及操作反馈；任务 2 不关闭 |

| 实际命令 | 结果 |
| --- | --- |
| `go test ./internal/stats -count=1 && go vet ./internal/stats` | PASS |
| `go test ./internal/server -count=1 && go vet ./internal/server` | PASS；旧 JSON/CSV 导出入口复用共享 service 后，server 合同回归通过 |
| `go test ./... -count=1 && go vet ./...`（`desktop/`） | PASS |
| `go test -run '^TestNativeStatsAutoRefreshUsesConfiguredIntervalWithoutOverlap$' -count=1`（`desktop/`） | PASS；自动刷新档位、筛选/分页透传、加载时不重叠及 Off 停止 |
| `go build -o "$outdir/pi-switch-desktop" .`（`desktop/`，输出在 `/tmp/pi-switch-mygo-build.4ajfAB/`） | PASS；自动刷新改动后的当前构建生成 x86-64 ELF，未安装或部署 |
| `gofmt -l`（本轮修改的 Go 文件）、`git diff --check` | PASS |
| GUI 实机及完整计划操作 | BLOCK；2026-10-11 SSH 到目标 `gui-test@10.10.10.244:22` 返回 `Connection refused`。项目记录的 Ubuntu Xvfb/Openbox GUI 主机不可达，不能执行或声称通过；这也不构成 Arch+niri/Wayland 验收 |

本轮修改尚未提交、未 push；没有启动/停止 Proxy、依赖安装或修改真实用户数据。Stats 导出明确包含完整历史，并可能包含敏感错误文本和上游 URL；使用者应只保存到可信路径。任务 1 的行号 gutter 与任务 1/2 实机验收仍是后续门槛。

## 隔离 GUI 主机 Xvfb 交互补验（2026-10-11）

先前“SSH Connection refused”记录对应较早尝试；本次已成功登录经核验指纹的 Ubuntu 24.04 测试主机。使用当前 dirty worktree 构建（SHA256 `e356cc9f2e9b0b19fb290107728a37b9ef1ddc0a1b9c1def3f03d5819ecb3e83`），所有文件/数据库位于 `/home/gui-test/pi-switch-gui/sandbox/runs/plan-ae3b9ef-20261011083843`。以下只作为 Xvfb/Openbox/llvmpipe 的交互证据；不代表 Wayland、IME、真实硬件或 Gate C 通过。

| 实际交互 | 结果 | 观察 |
| --- | --- | --- |
| Gateway JSON draft | PASS（隔离假数据） | 加载仅含 pi-switch 自有项的生成草稿；有效草稿验证、加入 `pi-switch-chat`；加入不支持的 `pi-switch-extra` 后显示冲突且禁用发布，config/models 哈希不变；取消确认不写入；确认发布后生成自有模型，并保留第三方假 Key 和自定义字段。截图在 `screenshots/gateway-*.png` |
| Stats / Sessions | PASS（隔离假数据） | 查看汇总、筛选、请求分页、会话分页和 session 请求明细；5s 自动刷新在未手动刷新的情况下反映新插入的假请求；关闭刷新后保持 Off。JSON/CSV 文件对话框导出各 26 条，CSV 列合同与既有格式一致，导出文件权限 `0600`；刷新前后请求事实与 session 文件哈希未变。截图在 `screenshots/stats-*.png`、`screenshots/conversations-*.png`、`screenshots/session-*.png`、`screenshots/export-*.png` |
| Packages | PASS（隔离注册表） | 导入本地假 `demo-pi`、禁用/启用、增加 `npm:manual-gui-test` 注册项；取消移除保持注册，确认后软移除（`installed=0`）。未执行网络包安装，也未访问真实 Pi Agent 目录。截图在 `screenshots/packages-*.png` |

该 run 的唯一二进制与远端校验一致。没有启动/停止 Proxy，也没有修改基线应用、真实用户目录或工作树源代码。当前环境不支持计划要求的 Arch+niri/Wayland 与 fcitx5；全量会话/窗口管理、物理缩放、托盘降级和 Gate C 等价性仍待验证。任务 1/2 继续保持 IN PROGRESS，不能据此退役 React/WebView。

## JSON Editor 真实 X11 语法诊断补验（2026-10-11）

在当前 dirty-worktree Native GUI 的 JSON Editor 中加载安全生成草稿，通过 X11 键盘事件输入三行非法 JSON 并执行 Validate。界面显示 `Invalid JSON at line 3, column 10 — draft preserved`，错误所在整行以 danger 色着色，Publish 仍禁用；随后重新加载安全草稿，未执行发布。截图：`/home/gui-test/pi-switch-gui/sandbox/runs/plan-ae3b9ef-20261011083843/screenshots/gateway-syntax-error-current.png` 与 `gateway-draft-reset-current.png`。这是 Xvfb/Openbox 的实际窗口证据，不是 Arch+niri/Wayland 或 fcitx5 中文 IME 证据；选区粘贴仍只有 UI tester 覆盖。

## Native JSON 行号 gutter API 可行性核查（2026-10-11）

检查当前锁定的 `github.com/egoist/mygo v0.3.3` UI API：`TextArea` 公开垂直 `TrackScroll` 状态，但内部按段落布局并自动换行；公开 `TextRange` 只有字符范围、前景色和字重，没有背景色或视觉行映射。单纯增加按 `\n` 生成的侧栏行号，在长行换行后会与编辑器后续行错位；关闭换行又没有 TextArea 水平滚动能力，长行会被裁切。因此不加一个显示上有行号、实际却可能误导定位的 gutter。当前保留可读的行/列诊断和错误行前景色，独立 gutter/背景高亮仍是 Task 1 未关闭项；升级该控件或实现可测量的同步行布局前再做。
