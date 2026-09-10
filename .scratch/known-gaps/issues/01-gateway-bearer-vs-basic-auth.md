# 01: 网关发布的 Bearer 凭据与守卫只认 Basic 不匹配（F21）

Status: open —— 需要产品决策，未修复

**一句话**：具名 LAN 绑定且设了密码时，网关发布给 Pi 的那个 provider **无法通过鉴权**。

## 证据链（全部已核到行号）

1. 网关发布 provider 时写死一个哨兵 apiKey：
   `internal/gateway/gateway.go:236` → `"apiKey": "pi-switch-proxy"`。
2. 同一处发布的 baseUrl 指向代理自己：`internal/gateway/gateway.go:184`
   → `baseUrl := "http://" + host + ":" + itoa(port) + "/v1"`。
3. 代理侧守卫**只接受** HTTP Basic：
   `internal/server/server.go:486-490` → `if !strings.HasPrefix(auth, "Basic ") { reject(c) }`；
   `:471` 的挑战头是 `Basic realm="pi-switch"`。
4. Pi 会把 models.json 里的 `apiKey` 以 `Authorization: Bearer …` 发出（这一点是**未核实**的
   外部行为，来自 P0 票 02 的记录；本仓库无法自证）。

后果：管理面/代理面在**非 loopback 绑定 + 有密码**时，任何携带 `Bearer pi-switch-proxy` 的客户端
得到 401 JSON，而它正是网关自己配置出去的那个客户端。

## 为什么现在没暴露

- 通配绑定被改写成 loopback：`gateway.go:178-181` 把 `""`/`0.0.0.0`/`::`/`[::]` 一律改写为
  `127.0.0.1`。于是发布的 baseUrl 看着是本地地址，问题被掩盖。
- loopback 绑定本来就不设防（`proxy_auth_test.go:32` 固定该行为），所以默认部署下不会撞上。

## 相关但独立的第二处不一致（本次复核新发现）

**鉴权决策取自实际绑定地址，发布的 baseUrl 却取自配置**：

- 守卫的判定基于 CLI 传入的实际绑定地址（P0 批次已确立：`--host` 决定是否设防，`cfg.Settings.Web.Host`
  不再被信任）。
- 而发布用的 host 读的是 `cfg.Settings.Proxy.Host`（`gateway.go:178`）。

因此「用 `--host <具名 LAN 地址>` 启动但设置里仍是 127.0.0.1」时，两者会给出不一致的画面。
这一条与 F21 同源（两个来源各自决定一件事），修 F21 时应一并确定以谁为准。

## 修之前必须先定的

1. **要么**文档化「暴露代理需要支持 Basic 的客户端」，并让发布路径给出可用的凭据形态；
2. **要么**在 spec 层重议「仅 Basic」，让守卫也接受 Bearer。

**明确不做**（P0 票 02 已判定）：把共享密码写进 `models.json`——那等于把共享密钥发布到
`~/.pi/agent/models.json`。

## 修复时会碰到的既有测试（当前行为的固定网）

`internal/server/proxy_auth_test.go`：`LoopbackStaysOpen`(:32)、`RejectionIsTheMiddlewareAnswer`(:73，
断言 401 带 `WWW-Authenticate`)、`NonLoopbackWithoutPasswordIsRejected`(:92)、`UsesTheSharedCredential`(:113)、
`HealthProbeStaysReachable`(:133)。加 Bearer 支持必须同时改这些，否则它们会正确地拦住你。

## 验收（未定，取决于上面两条出路中的哪一条）

- [ ] 具名 LAN 绑定 + 密码下，网关发布出去的那个客户端能真正用起来（或文档明确它不支持）
- [ ] 鉴权决策来源与发布 URL 来源的一致性有明确结论并被测试固定
- [ ] `models.json` 中不出现共享密码

**处理情况（2026-09-11）**：按 A 案落地（明确告知 + 文档化，不发布任何凭据）。两条 publish 路径（`PUT /api/models/gateway`、`POST /api/gateway/publish`）在非 loopback 绑定时于成功响应中带 `warnings`；CLI `gateway publish` 同样把警告打到 stderr，判断取自**即将写入的条目**自身的 baseUrl，避免与 gateway 包的 host 推法漂移；两个 README 记录了该限制。测试同时断言响应与 `models.json` 都不含共享密码。**根治项（可发布的代理专用 token）仍未决，需另立票。**
