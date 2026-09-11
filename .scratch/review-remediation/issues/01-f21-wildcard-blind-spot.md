# 01: F21 通配绑定漏报——判定改为「代理面是否超出 loopback」

Status: resolved (2026-09-11)

**缺陷（已实测）**：`publishedProxyAuthWarning` 判的是**发布出去的 baseUrl**，而
`BuildProposedGatewayEntry`（`gateway.go:177-181`）把 `""`/`0.0.0.0`/`::` 改写成 `127.0.0.1`。
配置 `proxy.host=0.0.0.0` 时：`gateway publish` stderr 为空；models.json 里是
`http://127.0.0.1:PORT/v1` + `apiKey: pi-switch-proxy`；而 proxy 以 0.0.0.0 + 密码启动后，
`Authorization: Bearer pi-switch-proxy` → **401**。server 侧判的是**管理监听**的绑定地址，与「发布出去的
provider 能不能用」无关（webui 在 loopback、proxy 在通配时同样沉默）。

**修**：判定改为「代理面是否超出 loopback」——`cfg.Settings.Proxy.Host` 原值（空/通配都算超出，
因为通配绑定必然要求密码，`ValidateBindAuth` 会拒绝无密码启动）**或** 发布条目自身 baseUrl 的 host
非 loopback。判定与文案一并移到 `internal/gateway`（server 与 CLI 都已 import 它；kernel rule 禁止
gateway 域反向依赖 server），消除两处近逐字重复的文案。

**验收**
- [ ] 配置 `proxy.host=0.0.0.0` → CLI `gateway publish` 有 warning，server 两条 publish 响应有 warnings
- [ ] loopback 时仍保持原响应（不新增噪音）
- [ ] 文案只有一份，两个调用方共用
- [ ] 既有 W2 测试里把 `0.0.0.0` 当作 loopback 的断言被改正
- [ ] README 与 progress 里写反的「只会误报、不会漏报」改正为准确表述
