# 09: Go 骨架与构建基座

**What to build:** 从零搭建 Go 重写的最小可跑闭环：`cmd/pi-switch` 入口 + `internal/config` per-request JSON 加载（含 `version/current/profiles/settings` 兼容与 `injectOpenCodeAttribution→conversationSource` 迁移）+ `gin` 双服务（`127.0.0.1:18080` proxy 占位 + `127.0.0.1:18081` mgmt 占位 + `/healthz`）+ `embed.FS` 空 `webui/dist` 占位，`go build -ldflags "-s -w"` 与 `npm run build:webui && go build` 两阶段可执行，`go test` 冒烟通过。对应原型 `prototype/go-skeleton-gin-sqlite:main.go` 的 `defaultConfig/loadConfigPerRequest/initStore` 提升。

**Blocked by:** 无（可直接开始）

**Status:** ready-for-agent

- [ ] `go run ./cmd/pi-switch -- --help` 与 `go build` 在 `linux/darwin/windows×amd64/arm64`（`modernc/sqlite` 纯 Go 无 CGO）均可执行
- [ ] `~/.pi-switch/config.json` per-request 读取：手改文件后下一次请求即生效，无需重启；旧字段 `injectOpenCodeAttribution` 自动迁移到 `conversationSource`（`true→proxy/false→off`），`version<2` 升至 2
- [ ] `GET /healthz`（双端口）与 `GET /`（`embed.FS` 占位 HTML）可访问；`GET /api/config` 返回 `source` 与完整 `config`
- [ ] `Settings` 默认值符合 spec：`providerPrefix=pi-switch`、`writeMode=gateway`、`gatewayApi=openai-completions`、`proxy 127.0.0.1:43112`（原型用 18080 避冲突）、`web 127.0.0.1:43110`（原型 18081）、`conversationSource=sessionScan`
