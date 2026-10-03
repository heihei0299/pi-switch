# Spec: 配置 JSON 筐内标红 + Response 测试支持

Status: ready-for-agent

## 背景

- 点击“测试”对 `api=openai-responses` 的供应商提示“不支持response接口.”，实际应支持 `POST /v1/responses` 的上游可用性探测（与 `openai-completions` 的 `/v1/chat/completions` 对称）。
- 配置 JSON 的文件筐（`JsonEditor`）当前仅在下方文字提示校验结果，未在筐内对出错行标红；期望“出现语法错误的地方才变红”，语法错误整行淡红+行号红，语义错误不进红背景（预留行号黄点）。

## 目标

- **测试**：`src-rust/ops.rs::test_provider` 新增 `openai-responses` 分支，`POST {baseUrl}/v1/responses`，body `{model, input:"test", max_output_tokens:5}`，`Authorization: Bearer <key>`，10s 超时，2xx 即成功，其余按现有 `TestResult` 透出；`google-generative-ai`/`bedrock-converse-stream` 等仍返回 `Unsupported API type`。
- **筐内标红**：`webui/src/components/JsonEditor.tsx` 扩展为受控高亮编辑器，接收 `errorLine?: number | null`（1-based），出错行整行 `bg-red-500/10` 且行号 `text-red-400`，其余行保持原样；`GatewayPanel`/`ModelsModal`/`ProfileForm` 的 `JsonEditor` 通过 `validate*` 的 `error` 中解析出的行号（`JSON.parse` 的 `at position` 或 `Unexpected token at line X`）传入，`JSON.parse` 成功但 `validate*` 失败时不标红（仅下方文字提示，预留语义黄点）。
- **格式化**：左下“格式化”按钮（`JsonEditor` 外）保持 `try { setValue(JSON.stringify(JSON.parse(value), null, 2)) }`，可在任意时刻格式化，格式化后重算 `errorLine`。

## 非目标

- 不引入 Monaco/Codemirror，仅在现有 `JsonEditor`（行号+textarea）上叠加行背景高亮。
- 不改变 `validateGatewayJson`/`validateModelsJson` 的语义规则，仅将其错误映射到行号的“是否标红”策略。

## 交互

- 网关 `配置 JSON`、供应商 `ModelsModal` 的 `models json`、 `ProfileForm` 的 `profile json` 三处均使用 `JsonEditor`，`value` 为 `rawText`/`text`，`onChange` 直通，`errorLine` 由父容器计算。
- 点击“格式化”后若 JSON 合法则重排并清除错误行；非法则保持错误行标红。
- 测试按钮：`ProfilesPanel` 的 `ProfileCardMenu → 测试` 对 `openai-responses` 不再直接返回 `Unsupported API type`，而是走 `POST /v1/responses` 探测，上游 2xx 显示 `✓ Connected successfully`，否则透出 `✗ HTTP {code}`。

## 后端

- `src-rust/ops.rs::test_provider` 在 `match profile.api.as_str()` 中新增 `"openai-responses" => { body = json!({"model": ..., "input": "test", "max_output_tokens": 5}); url = format!("{}/responses", base); }`，其余逻辑复用 `reqwest` 与 `Authorization`。
- 保持 `load_config`/`backup` 等不变。

## 前端

- `JsonEditor.tsx` 新增 `errorLine?: number | null` prop，行号 `div` 与 `textarea` 的对应行 `div` 根据 `errorLine` 切换 `bg-red-500/10 text-red-400`。
- `GatewayPanel.tsx`/`ProfilesPanel.tsx`（两处）新增 `errorLine` 计算：`useMemo(() => { try { JSON.parse(text); return null; } catch (e) { const m = String(e).match(/at position (\d+)/); if (m) { const pos = Number(m[1]); return text.slice(0, pos).split("\n").length; } const m2 = String(e).match(/line (\d+)/i); if (m2) return Number(m2[1]); return 1; } }, [text])`，语义错误时返回 `null`（不标红）。
- 保留 `validate*` 的下方文字提示，`errorLine` 仅用于语法错误。

## 验收标准

- [ ] `api=openai-responses` 的供应商点击“测试”不再显示“不支持response接口.”，而是走 `POST /v1/responses`，mock 上游 200 时显示成功。
- [ ] 配置 JSON 筐内：`{"a":}` 等语法错误时出错行整行淡红+行号红，`{"api":"x"}` 等语义错误时筐内不红（仅下方文字）。
- [ ] “格式化”按钮在任意时刻可点击，合法 JSON 被重排，非法时保持标红。
- [ ] 三处 `JsonEditor`（gateway/models/profile）行为一致，`vitest` 与 `cargo test` 全绿。

## 子代理编排

- `T1` 后端 `test_provider` 由 `general-purpose` 子代理按 TDD 实现（红：`cargo test` 中 `test_provider` 对 `openai-responses` 的 mock 测试失败 → 绿：新增分支 → `cargo test` 通过）。
- `T2` 前端 `JsonEditor` 标红由 `general-purpose` 子代理按 TDD 实现（红：`vitest` 中 `JsonEditor` 对 `errorLine` 的断言失败 → 绿：行背景高亮 → `vitest` 通过）。
- 主代理负责 `spec` 澄清、`seams` 确认与最终 `cargo test`/`vitest` 全量、`code-review`（限长）与 `commit`，子代理间无文件写冲突（分属 `src-rust` 与 `webui/src/components`）。
