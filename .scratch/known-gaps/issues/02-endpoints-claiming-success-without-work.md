# 02: 三个端点返回成功但什么都没做

Status: open —— 部分需先确认「是否有意为之」，未修复

**一句话**：这三处都在返回成功载荷，但它们宣称的状态/动作与真实情况无关，脚本或前端据此判断会得出错误结论。

## (a) `POST /api/proxy/start` 空 body → 「proxy started (stub)」

- `internal/server/settings_handlers.go:127` → `c.JSON(200, gin.H{"running": true, "message": "proxy started (stub)"})`。
- 走到这条支路的条件是：body 里没有 `daemon`、query 没有 `?daemon`、且 `host == "" && port == 0`
  （`:98-113`）。即**恰恰是"什么参数都没给"的那种调用**，会被回以"代理已启动"。
- 该支路没有 `daemon.Start`、没有任何监听、没有启动任何进程 → 纯字面量。
- 真实启动走 `:114-125`（有 `daemon.Start`，返回 pid/host/port/startedAt）。

**判断所需**：这个 `stub` 支路是有意保留的兼容占位（消息里带 "stub" 已经算诚实提示），还是应该改成
400/501？若保留，至少不应回 `running: true`——调用方无法区分它与真实启动。

## (b) daemon 启动路径把失败推迟到健康检查之后

- 同文件 `:115` → `daemon.Start(daemon.Proxy, host, uint16(port))` 是**同步**的。
- `internal/daemon/daemon.go:450` → `healthy := managedHealth(info, 15)`；
  `:294-299` → `checkHealthEndpoint(host, port, 15)`；
  `:127-142` → 每次尝试探测 `/healthz` 与 `/health` 两个路径，各带 500ms client 超时，每次尝试之间
  `time.Sleep(200ms)`。

**真实等待时长（按机制计算，不是"~15 秒"这个流传的说法）**：15 次尝试 ×（2 路径 × 500ms 超时 +
200ms 间隔）。连接被拒时单次几乎立即失败 → 约 **3 秒**；端口在但无响应时最坏 → 约 **18 秒**。
失败的错误在 `daemon.go:466` 才产出，也就是说 HTTP 请求被挂住这么久才有结果，期间调用方拿不到任何进度。

- 附带同类问题：`settings_handlers.go:117` 用**错误文本**判断"端口被占用"
  （`strings.Contains(strings.ToLower(err.Error()), "port already in use")`），与本批次刚从 profile 域
  移除的按文本分类是同一种做法。`daemon.go:461-465` 确实会读日志生成含该字样的消息，
  所以通常能命中；但这是靠措辞对齐，不是靠类型。

**判断所需**：是否给启动加异步/进度反馈；是否把"端口占用"改成有类型的错误。

## (c) `GET /api/gateway/health` 与 `POST /api/gateway/start` 的恒定载荷

- `internal/server/gateway_handlers.go:217-220`：恒返回
  `{"running": true, "mode": "logical-isolation", "gateway_id": "pi-switch", "has_models_file": true, "last_notify": nil, "message": "ok"}`。
  其中 `running`、`has_models_file`、`last_notify`、`message` 全是**字面量**，从不与文件系统核对；
  只有 `upstreams_total: len(cfg.Profiles)` 是真数据。
- `:221-223`：`POST /api/gateway/start` 恒返回 `{"running": true, "mode": "logical-isolation", "gateway_id": "pi-switch"}`，
  不启动任何东西。

**注意（避免误判为缺陷）**：`mode: "logical-isolation"` 看起来是有意设计——网关是"发布 models.json"这个
逻辑概念，没有常驻进程可启动，所以 start/health 是空操作本身合理。**真正的缺陷是"报告未检查的状态"**：
`has_models_file: true` 在 `~/.pi/agent/models.json` 不存在时同样是 true，而前端/脚本很可能拿它当作
"已发布"的依据。

**判断所需**：确认这三个端点是否被前端当真值使用；若是，`has_models_file` 必须改为真实检查
（`os.Stat`），否则应删掉该字段而不是谎报。

## 验收（三条各自独立）

- [ ] (a) 空 body 的 `proxy/start` 不再声称 `running: true`（或明确它是有意的兼容占位并有测试固定）
- [ ] (b) 启动失败的等待上限有明确理由，且「端口占用」不再靠错误文本判定
- [ ] (c) `has_models_file` 反映真实文件状态，或该字段被移除

**处理情况（2026-09-11）**：(a) 非 daemon 分支改为 501 + 指明 `daemon:true`；(b) 子进程退出即结束健康等待（同场景实测 18037ms → 17ms），并且端口占用改为 `daemon.ErrPortInUse` + `errors.Is`；(c) `has_models_file` 改为真实 `os.Stat`。`gateway start` 的恒定载荷经查证是有意设计（`mode: logical-isolation` 无进程可启动），故不改。
