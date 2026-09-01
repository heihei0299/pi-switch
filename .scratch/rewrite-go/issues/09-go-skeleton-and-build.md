# 09: Go 骨架与构建基座

**What to build:** 从零搭建 Go 重写的最小可跑闭环：`cmd/pi-switch` 入口 + `internal/config` per-request JSON 加载（含 `version/current/profiles/settings` 兼容与 `injectOpenCodeAttribution→conversationSource` 迁移）+ `gin` 双服务（`127.0.0.1:18080` proxy 占位 + `127.0.0.1:18081` mgmt 占位 + `/healthz`）+ `embed.FS` 空 `webui/dist` 占位，`go build -ldflags "-s -w"` 与 `npm run build:webui && go build` 两阶段可执行，`go test` 冒烟通过。对应原型 `prototype/go-skeleton-gin-sqlite:main.go` 的 `defaultConfig/loadConfigPerRequest/initStore` 提升。

**Blocked by:** 无（可直接开始）

**Status:** resolved

- [x] `go run ./cmd/pi-switch -- --help` 与 `go build` 在 `linux/darwin/windows×amd64/arm64`（`modernc/sqlite` 纯 Go 无 CGO）均可执行
- [x] `~/.pi-switch/config.json` per-request 读取：手改文件后下一次请求即生效，无需重启；旧字段 `injectOpenCodeAttribution` 自动迁移到 `conversationSource`（`true→proxy/false→off`），`version<2` 升至 2
- [x] `GET /healthz`（双端口）与 `GET /`（`embed.FS` 占位 HTML）可访问；`GET /api/config` 返回 `source` 与完整 `config`
- [x] `Settings` 默认值符合 spec：`providerPrefix=pi-switch`、`writeMode=gateway`、`gatewayApi=openai-completions`、`proxy 127.0.0.1:43112`（原型用 18080 避冲突）、`web 127.0.0.1:43110`（原型 18081）、`conversationSource=sessionScan`

## Answer

已实现并验证：
- `go.mod`（gin v1.10.0 + modernc/sqlite v1.38.0，纯 Go，`go 1.23`）`go test ./...` 4+2 通过，`go vet` 0，`go build -ldflags "-s -w"` 产物 7.4M
- `internal/config`：`DefaultConfig`/`LoadConfigAtPath` 支持 per-request 热重载、`injectOpenCodeAttribution` 迁移、`version<2→2`，4 单测覆盖
- `internal/server`：`NewProxyRouter`/`NewMgmtRouter`（gin，`PI_SWITCH_CONFIG` 环境覆盖便于测试），`GET /healthz` 双端口、`GET /` 占位 HTML、`GET /api/config`，2 集成测试
- `cmd/pi-switch`：`--help/--version/proxy/webui/doctor` 子命令，`--` 前缀兼容 `go run -- --help`
- `webui/dist/.gitkeep` 占位，`npm run build:webui && go build` 链路就绪

提交：`feat/rewrite-go@a1c7c26`。Code Review 双轴通过（5 smells 已修 2，剩余为判断型；Spec 1 partial `embed.FS` 为 09 可接受占位）。

下一票：`10 单供应商代理闭环` 已 unblocked。
