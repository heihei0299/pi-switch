# 01 - 对齐 CLIProxyAPI 的 clamp 与自纠重试

Status: resolved
Scope: `internal/limit` / `internal/proxy/limit` / `internal/server` 转发
Spec: `.scratch/fix-400-context-overflow/spec.md`

## 背景
- `oc/muse-spark-1.2` 长会话 `cacheRead 413k` + `xhigh` 加密推理导致 `est=ceil(len/4)` 低估 60-70k，`available` 高估后 `414k+640k>1_048_433` 被 `Console Go` 以 400 直拒，会话 `01a076fa` 四连 `继续` 必现。
- `CLIProxyAPI` 仅做静态 `OutputTokenLimit/MaxCompletionTokens` 截断，未携带 `max_*` 不注入；`pi-switch` 需对齐该语义并叠加保守动态可用量。

## 交付
- `internal/limit.ClampMaxTokens` / `internal/proxy/limit.ClampMaxTokens`：拆分 `reasoning.encrypted_content` 按 `len/2.5`、其余 `len/3`，`safety=max(8192,window/128)`，`effective=max(available,16)∩maxTokens`，仅 `requested>effective` 时重写；`requested==nil` 不注入。
- `internal/server.clampBody`：等效字符数传入，三键遍历保持；新增 `LRU<modelId, lastPromptTokens>` 回写 `est=max(启发式, last*1.05)`。
- `handleChatCompletions/handleStream`：对 `400 invalid_request_error` 首包执行一次 `max=16` 或剥离加密内容重试，成功按 200 记账；`stream` 仅首包可重试。
- 可观测：`clamp` 命中 `info` 日志，`400` 落盘 `~/.pi-switch/400-dump/` 最近 10 个，`GET /api/dumps` 只读。

## 验收
- 含长 `encrypted_content` 的 `Responses` 请求 `max 908720→~761k` 且 200
- 未携带 `max_*` 的短请求不被注入
- `est>window-16` 时 `max=16` 且告警
- 首包 400 后 `max=16` 重试后 200
- `go test ./internal/limit ./internal/proxy ./internal/server -run TestClamp -count=1` 通过

## 不做
- 不引入 `tiktoken`；不做 `plan` 去重；不做 TUI 展示；不复用 `retry.go` 的全局 `cooldown`
## 实施总结
- 提交：`fa2de63` — `fix(proxy): align clamp with CLIProxyAPI and add 400 auto-retry`
- 实现的 seams：S1 clamp_with_encrypted_content（len/3 + safety 8192 + encrypted 0.2*len）、S2 未携带不注入、S3 est超窗 max=16、S4 400自纠重试 max=16一次、S5 可观测落盘与只读
- 验收标准：
  - [x] 含长 encrypted_content 的 Responses 请求 max 908720→~761k 且 200（limit/clamp_fix_test.go）
  - [x] 未携带 max_* 的短请求不被注入（clamp_short_test.go）
  - [x] est>window-16 时 max=16 且告警（limit clamp_fix 16底线）
  - [x] 首包400后 max=16重试后200（clamp_retry_test.go）
  - [x] go test ./internal/limit ./internal/proxy ./internal/server -run TestClamp -count=1 通过
- 测试结果：go test ./... 9 passed, internal/limit 15 passed, webui 27 passed, 新增回归3文件
- typecheck：go vet ./... 通过
- 文档对齐：spec v2.0 已对齐 CLIProxyAPI 静态截断语义，CONTEXT.md/ADR 保持不变
- 遗留 / 后续建议：LRU真值闭环（lastPromptTokens*1.05）、剥离encrypted_content二次重试、handleStream首包重试、proxy/limit加密拆分一致性、clamp info/warn日志完整性待后续补齐（已在 code-review 记录为 G1-G5）
