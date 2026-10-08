# WP-06 Native complex progress — initial and follow-up 2026-10-08

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
- Gateway generated preview reads real config and `models.json`; selected-model and edited JSON-draft publication are tested against temporary files, but the full conflict-resolution workflow remains unimplemented. The JSON editor has line/column diagnostics and error-line color, but no line-number gutter or background highlight. Stats sessionScan attribution, aggregate summaries, virtualized provider/model lists, independent preset/custom range filtering, and per-conversation request paging/nullable token fields are tested against isolated SQLite/JSONL fixtures, but physical GUI behavior against a real session directory and detailed failure errors remain unverified. Current WebUI Stats source is KPI/table based with auto-refresh and JSON/CSV export; no time-series chart component was found. Existing-schema migration behavior was not exercised; tests verify no fact rows changed and no missing DB was created.
- Gate C remains blocked by missing functional parity evidence. Do not retire React/WebView or present these prototypes as migrated production pages.
- Full repository tests remain deferred until WP-05 through WP-08 are complete.
