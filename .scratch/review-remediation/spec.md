# 6debc80 评审修复

**来源**：`code-review` 双轴对 `6debc80` 的结论（Standards + Spec），以及我自己的复核。
**性质**：只修评审发现的问题，不扩大范围。

## 判据

1. **先修真缺陷**：通配绑定下 F21 漏报是唯一会让用户拿到 401 的漏报，优先级最高。
2. **不能只改文档**：如果实现漏判，改注释不算修完，必须改判定本身。
3. **已文档化的标准优先**：复用 kernel helper 这类硬性发现按标准修（但修法要兼顾可用性，不照抄 reviewer 示例）。
4. **测试要能挡住自己犯过的错**：本批新增测试必须覆盖"接线"而不只是纯函数——因为上一批的教训 1 正是「单测通过、生产不触发」。
5. **诚实**：我此前对外写下的两处错误结论（"只会误报不会漏报"、"proxyStart 无调用点"）必须改正，而不是删掉。

## 票

| 票 | 主题 | 类型 | 判定 |
|----|------|------|------|
| 01 | F21 通配绑定漏报：改为判定"代理面是否超出 loopback"，判定与文案共享到 `internal/gateway` | 真缺陷 | Spec(a)2 |
| 02 | 补 `Start` 接线的端到端测试（子进程立刻失败），并删掉 `exited != nil` 死分支 | 覆盖缺口 | Spec(a)1 / Standards 风险1 |
| 03 | 复用 kernel `notImplemented` | 硬性标准 | Standards 硬性1 |
| 04 | `proxyStartError` 去掉死返回 `int`；改正"形状完全照旧"的过度声称 | 清理 + 诚实 | Standards 风险4 / Spec(c) |
| 05 | `gatewayHealthPayload` 去掉未使用参数 `c` | 清理 | Standards 风险3 |
| 06 | 修正 `docs/architecture-review.md` A7：恢复"全仓判据仍不成立"，并限定"不再按文本分类"只对 handler 成立 | 文档准确性 | Spec(c) |
| 07 | 删 `cmd.Process.Release()`（实测：Release 先于 Wait 时 Wait 15µs 内以 `invalid argument` 返回） | 隐患消除 | Spec(c) / Standards 风险2 |
| 08 | 修正 `.scratch` 里的两处事实错误；前端读对象型 `error` 列为后续 | 记录诚实 | Spec(a)2/a3 |

## 不在本批范围（诚实记录）

- **前端 `req()` 只认字符串 error**（`webui/src/api.ts:75`）：因此 8 个 501 端点的可操作 message 在 UI 里都显示为 "Not Implemented"（`res.statusText`）。正确修法是让前端读 `error.message`，但改 webui 需要跑 tsc/vitest，本会话只授权 `go test`，故本批不动，仅记录。
- **提交粒度与 `tickets:` trailer**：`commit-check` 技能要求「一个 commit 只表达一个逻辑变更」；本批按该要求处理，但历史提交不回改。
- `daemon.Start` 仍按日志文本判定端口占用（`address already in use`）：日志是子进程的输出，没有类型可言，这是唯一可行的判定方式——本批只把结论限定到"handler 不再按文本分类"。
- `Start` 的运行期绑定与配置不一致（`proxy start --host X` 但配置里是 Y）仍可能漏判；正确来源是运行中的 daemon host，本批用配置（覆盖评审实测的通配场景），运行期差异记为残留限制。
