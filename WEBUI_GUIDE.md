# pi-switch WebUI Guide — the thick reference

> **WebUI is now the primary interface.** This guide is the thick, single-source reference for the browser UI — architecture, build, runtime, security, the 4-step recipe and the full REST ↔ core map.
> The [README](./README.md) stays **thin**: quick start + overview + screenshots, with details deferred here.

pi-switch offers **three ways** to manage the same configuration — order reflects the new priority:

| Priority | Interface | Entry | Lives in |
|----------|-----------|-------|----------|
| **① Primary** | **WebUI** | `pi-switch webui start --daemon` → http://127.0.0.1:43110 | `internal/server` (gin) + `webui/` (React, `embed.FS`) |
| ② CLI | `pi-switch <cmd>` | `bin/pi-switch.js` → Go binary | |
| ③ TUI | `pi-switch tui` | `internal/tui/` (bubbletea) — secondary, terminal fallback | |

All three are **thin adapters over the same Go core** (`internal/config` +
`internal/gateway` + `internal/proxy` + `internal/server` + `internal/store`). No business logic lives in the UI
layers, so behaviour stays identical across them.

### Screenshots (also in README)

All screenshots are 1280×800, dark theme, captured with demo profiles:

- `assets/webui-home.png` — Home / Overview (profiles / exposed / current / proxy)
- `assets/webui-profiles.png` — Profiles list with Preset / Responses mode / exposed badges
- `assets/webui-gateway.png` — Gateway `Current vs Proposed` diff, pending banner and `Apply to Pi`
- `assets/webui-stats.png` — Stats with time-window, auto-refresh, provider & conversation tables

TUI screenshot remains at `assets/main.png` for terminal reference.

---

## Architecture — thin adapters

```
                 ┌──────────── shared Go core ────────────┐
   WebUI(gin)  ─►│  server.go   (REST + reads/shaping)    │
   CLI (node) ──►│  config.go   (ProviderProfile)         │──► ~/.pi-switch/config.json
   TUI (go)   ──►│  gateway.go  (gateway publish)         │──► ~/.pi/agent/models.json
                 │  proxy/      (forward/failover/limit)  │
                 └────────────────────────────────────────┘
        ▲                    ▲                      ▲
   webui/src           bin/pi-switch.js         internal/tui/  ← REST /api/* + embedded webui/dist
   React SPA            (Go binary)            (bubbletea)
```

The WebUI backend is a **gin server** that:
1. serves the React SPA (compiled into the Go binary via `embed.FS`), and
2. exposes `REST /api/*` where every route delegates to `config`/`gateway`/`daemon`/`store`.

It runs as a second **daemon-managed service** alongside the proxy (own pid/log/port),
using the generalized machinery in `internal/daemon`.

---

## Usage

### Build (frontend + Go)

```bash
# one-shot: builds webui/dist then embeds it into the Go binary
npm run build

# or step by step
npm run build:webui      # vite build → webui/dist
npm run build:go         # go build -ldflags "-s -w" -o bin/pi-switch ./cmd/pi-switch (embeds webui/dist)
```

> Stop any running webui daemon before rebuilding — it may hold the binary.

### Run

```bash
# foreground (Ctrl+C to stop)
pi-switch webui start --port 43110

# background daemon (recommended)
pi-switch webui start --daemon
pi-switch webui status
pi-switch webui stop
```

Then open `http://127.0.0.1:43110` in a browser. Defaults come from
`settings.web` in `~/.pi-switch/config.json` (host `127.0.0.1`, port `43110`).

### Dev workflow (hot reload)

```bash
# terminal 1 — Go API server
pi-switch webui start --port 43110

# terminal 2 — vite dev server (proxies /api to :43110)
npm run dev:webui        # http://localhost:43111
```

---

## Security

- **Loopback binds run open** (`127.0.0.1`/`localhost`/`::1`) — intended for local use.
- **Non-loopback binds require HTTP Basic auth** (user `admin`). A password is
  auto-generated on first start and stored in `~/.pi-switch/webui_password`; the
  browser prompts for it natively.
- For public exposure, put the server behind a TLS reverse proxy (Nginx/Caddy/Cloudflare).

---

## Stats — detailed data model (moved from README)

Every proxied request is appended to `~/.pi-switch/requests.log` as a JSON line. For streaming responses the upstream SSE stream is teed: each request's input/output/cached/reasoning token counts (when the upstream reports them) and conversation id are parsed on the side and the log line is written when the stream ends — the stream itself is never buffered. Reasoning tokens are a subset of output tokens (parsed from `completion_tokens_details.reasoning_tokens` / `output_tokens_details.reasoning_tokens` where the upstream reports them); they never inflate the total.

### Log schema

```jsonc
{
  "ts": "2026-08-31T07:46:09.977Z",   // RFC3339, used for time-window filtering
  "ok": true,                         // success flag
  "provider": "demo-openrouter",
  "model": "demo-openrouter/anthropic/claude-sonnet-4.5",
  "status": 200,
  "ms": 342,                          // latency
  "promptTokens": 1200,
  "completionTokens": 450,
  "cachedTokens": 600,
  "reasoningTokens": 50,              // subset of completionTokens
  "conversationId": "conv-demo-001",
  "conversationName": "Fix WebUI README",
  "costTotal": 0.0021
}
```

Only successful (`ok: true`, not `retry`) rows with both `promptTokens` and `completionTokens` count towards token totals; failover/retry intermediate rows and old log lines without token fields are excluded gracefully.

### WebUI dashboard

- **Tiles** — `TOTAL / OK / FAILED / SUCCESS / CACHE RATE` then `INPUT / OUTPUT / CACHED / REASONING / TOTAL` and `COST`. `Cached ⊆ Input`, `Reasoning ⊆ Output`; missing or zero values show `-`.
- **By provider / By conversation** — aggregated Input / Output / Cached / Total / Cache rate / Cost, cache rates < 50% rendered in red.
- **Time window** — `Today` (local calendar day from 00:00), `Last 24h` / `Last 7d` (rolling), `Custom` (start day 00:00 → end day 24:00, both dates required). Default `Today`. The picker converts the window to `from`/`to` epoch-millis and calls `GET /api/stats?range=<today|last24h|last7d|custom>&from=<ms>&to=<ms>`; a bare request with no window parameters returns the full history.
- **Recent requests** — below the conversation card, newest first, capped at the most recent 100 (pageable via `?page&limit`): time, provider, model, status (with error for failures), Input / Output / Cached / Reasoning / Total plus per-request cache hit rate. Clicking a conversation cell copies the full conversation id.
- **Cache hit rate** = `cached input tokens ÷ total input tokens` (output excluded). When no cache data exists it shows `-`.
- **Conversation id** — `x-conversation-id` header first, `x-opencode-session` second (sent by pi/open-code clients), `conversation_id` body field as fallback (ADR-0002). Requests from spawned subagents fold into the parent conversation id. The two opencode attribution headers (`x-opencode-session` / `x-opencode-client`) are injected by the pi-side `conversation-id-inject` extension; disable via `settings.injectOpenCodeAttribution: false` in `~/.pi-switch/config.json` or in the WebUI Settings panel (restart pi afterwards).
- **Conversation name** — injected `x-conversation-name` is the session's explicit title, or falls back to the first user message. Non-Latin1 titles are percent-encoded on the wire so the header stays HTTP-safe, then decoded by the proxy and again defensively by the webui, so Chinese titles render readably. Pi's skill-injection messages (`<skill name="…">`) are skipped when picking the fallback title.
- **Auto-refresh tiers** — `Off` (default) / `5s` / `30s` / `5min`; on refresh failure the previous data is kept.

### Stats API pagination

- `GET /api/stats?range=&from=&to=&page=&limit=` — `recentRequests` are pageable; `recentRequestTotal` is returned alongside.
- `GET /api/stats/conversations` and `GET /api/stats/conversations/:id/requests` share the same window semantics and pagination.
- Window validation: `range=custom` requires both `from` and `to` (epoch-millis), `from < to`, otherwise `400`. `today` also requires bounds in current builds; plain `GET /api/stats` without window params returns the full history (All-time).

---

## Maintainability: adding a new operation (the 4-step recipe)

Because the UIs are thin, a new capability is added **once in the core** and then
wired into each adapter:

1. **Core** — implement the logic in `internal/config` (mutation) or
   `internal/server` (read/shape). This is the single source of truth.
2. **CLI** — add a command in `cmd/pi-switch` and expose it via `bin/pi-switch.js`.
3. **WebUI backend** — add one route in `internal/server/server.go` that calls the core fn.
4. **WebUI frontend** — add a method in `webui/src/api.ts` and use it in the
   relevant panel under `webui/src/components/`.

The TUI (`internal/tui/`) already calls the core directly, so it usually needs a
change only if the feature has a TUI screen.

### REST ↔ core map (current)

| Route | Core call |
|-------|-----------|
| `GET /api/state` | `server::get_state` |
| `GET /api/presets` · `/presets/:id` | `presets` |
| `GET /api/profiles/:name` | `config::get_profile` |
| `GET /api/models/gateway` | `gateway::get_gateway` |
| `GET /api/models/gateway/preview` | `gateway::preview` (dry-run, merges hand-written extra) |
| `PUT /api/models/gateway` | `gateway::apply` (validated write) |
| `GET /api/doctor` · `/config/validate` | `doctor` |
| `GET /api/backups` · `/stats` | `store::backups` · `stats` |
| `POST /api/profiles` · `PUT /api/profiles/:name` | `config::upsert` |
| `DELETE /api/profiles/:name` | `config::remove` |
| `POST /api/profiles/:name/{duplicate,test,fetch-models}` | `config::*` |
| `PUT /api/profiles/:name/{models,expose,spoof}` | `config::*` |
| `GET /api/profiles/:name/credits` | `credits` (5s 超时, 仅主上游) |
| `POST /api/proxy/{start,stop}` | `daemon::proxy` |
| `PUT /api/proxy/failover` | `config::set_failover` |
| `PUT /api/settings` | `config::update_settings` |
| `POST /api/config/{export,import,restore}` | `sync` |
| `POST /api/init` | `config::init` |

### Gateway explicit publish (supplier-gateway isolation)

Supplier mutations (`ProfilesPanel`, `SettingsPanel`, `ModelsModal`, `ProxyPanel` failover) only write `~/.pi-switch/config.json` and never auto-write `~/.pi/agent/models.json`. They show a toast "已保存到本地，需到网关发布" and leave `GET /api/models/gateway/preview` to reflect the pending diff.

1. `GET /api/models/gateway/preview` — dry-run, returns `{ current, proposed, conflicts, pending_count }` without writing; `current` is the last published gateway, `proposed` is built from current `config.json`.
2. `GatewayPanel` shows `Current vs Proposed` and `pending_count`, plus `pending`/`mismatch` banner on first load when `pending_count>0`; it does not auto-apply.
3. On `Apply to Pi`, `PUT /api/models/gateway` validates and atomically writes `models.json` (merging hand-written `extra` fields), then notifies.

This keeps supplier as the single source of truth, gateway as a read-only derived view, and prevents `models.json` overwrites from discarding manual `headers`/`compat`/`cost`/`extra` fields (merged, not authoritative).

---

## Type sync (frontend ↔ Go)

`webui/src/types.ts` is a hand-written mirror of the Go structs in
`internal/config/config.go` (the source of truth). Keep them in sync when the config
model changes.
