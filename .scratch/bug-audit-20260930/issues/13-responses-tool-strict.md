# Responses 转 Chat 工具定义丢失 strict
Status: resolved

## Requirements
- 沿用 HTTP 代理入口与真实 upstream 接收请求的回归边界，覆盖首次及 max-token retry。
- Responses function 定义中的 strict:true、strict:false 与显式 null 保留到 Chat function 定义；字段缺失时不添加默认值。
- 保留既有 name/description/parameters、工具选择与历史转换，不改变能力校验或重试策略。
- 遵循 system-contract §2.4.8，复用现有工具转换入口，不增加抽象或依赖。

## Validation
- `GOFLAGS='-run=^TestResponsesFunctionStrictReachesChatUpstream$' scripts/test-limited.sh go`：true/false/null 的首次/retry 六场景实际 Red（字段丢失）；修复后连同字段缺失对照共八场景 Green。
- 同脚本选跑 Tool、Responses/SSE 与前三项新增 HTTP 回归，均通过。
- `scripts/test-limited.sh go`：四项修复的 Go 全量测试共 18 个包通过。
- Standards 与 Spec 提交前双轴审查均无发现。
