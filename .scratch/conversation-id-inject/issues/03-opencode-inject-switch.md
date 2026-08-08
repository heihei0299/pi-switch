# 03 — opencode 归因头注入开关（settings.injectOpenCodeAttribution）

**What to build:** 为 conversation-id-inject 扩展新增显性配置 `settings.injectOpenCodeAttribution`（`~/.pi-switch/config.json`），控制是否注入 `x-opencode-session`（值=会话 id）与 `x-opencode-client`（值=pi）两个 opencode 归因头。默认 `true`（保持现有行为）；设为 `false` 时仅这两个头不注入，`x-conversation-id` / `x-conversation-name` 注入、Magic Context 剥离、子代理归并逻辑全部不变。

**Blocked by:** 02 — 扩展接线与对话归组端到端生效

**Status:** ready-for-agent

- [ ] 扩展：`parseOpencodeAttributionConfig(raw)` 纯函数——`undefined` / 坏 JSON / 键缺失 / 非布尔 → `true`；显式 `false` → `false`；显式 `true` → `true`
- [ ] 扩展：`loadOpencodeAttributionConfig()` 在扩展加载时读一次 `~/.pi-switch/config.json`（readFileSync + try/catch，复用 `src/core.js` 的 `CONFIG_PATH`）
- [ ] 扩展：`makeBeforeProviderHeadersHandler(getSession, options?)` 支持 `options.injectOpenCodeAttribution`（缺省 true）；false 时跳过归因头注入（不注入也不覆盖既有头）
- [ ] `src/core.js`：`defaultConfig()` settings 段与 `loadConfig()` 兜底均含 `injectOpenCodeAttribution: true`
- [ ] `src-rust/config.rs`：`Settings` 加 `injectOpenCodeAttribution: bool`（serde default=true, rename），防止 webui PUT /settings 整体替换 settings 段时静默丢字段；round-trip 测试通过
- [ ] webui：Settings 类型加字段；SettingsPanel General 区块 checkbox；i18n 中英文案；tsc 0 错误、vitest 全绿
- [ ] 单测：`npm test` 全绿（parse 6 用例 + handler 3 用例 + 既有用例）
- [ ] 端到端验收（人工）：config.json 置 `"injectOpenCodeAttribution": false` → 重启 pi → 经代理请求无 `x-opencode-session` / `x-opencode-client` 但有 `x-conversation-id`；置 `true` / 删除该键后恢复注入

## 实施总结

- 提交：`（待提交后回填）` — feat(extensions): add settings.injectOpenCodeAttribution switch for opencode attribution headers
- 实现的 seams：S1 `parseOpencodeAttributionConfig` 纯函数（保守默认：非显式 false 一律 true）；S2 `loadOpencodeAttributionConfig`（扩展加载时 readFileSync 读一次 ~/.pi-switch/config.json，复用 src/core.js CONFIG_PATH）；S3 `makeBeforeProviderHeadersHandler` 新增可选 `options.injectOpenCodeAttribution`（缺省 true），false 时跳过归因头注入；S4 `src/core.js` defaultConfig/loadConfig 加字段（loadConfig 对非布尔值归一化为 true）；S5 Rust `Settings` 加 serde 字段（default=true），防 webui PUT /settings 整体替换丢字段；S6 webui Settings 类型 + General 区块 checkbox + i18n
- 测试结果：`npm test` 56/56 全绿（parse 6 用例 + handler 3 用例）；cargo test 208 全绿（含 PiSwitchConfig 级 round-trip 与默认 true 用例）；webui vitest 96/96 + tsc 0 错误
- 评审修复：core.js 非布尔值归一化（`??=` 只兜底缺失，`"no"`/`0` 会被 webui 当 false 回显并写回）；README 补“pi 核心直连时仍注入”限定语；扩展局部变量改名避免遮蔽同名函数；Rust round-trip 测试改为经 PiSwitchConfig 嵌套路径（贴近真实丢字段场景）；webui checkbox 加 `?? true` 防御旧二进制缺字段
- 遗留 / 后续建议：
  - **人工端到端验收未执行**（需真实 pi 会话）：config.json 置 `false` → 重启 pi → 经代理请求无 `x-opencode-session` / `x-opencode-client` 但有 `x-conversation-id`；置 `true` / 删除该键后恢复注入。checklist 端到端项保持未勾选
  - 无关噪音（AGENTS.md、.pi/skills/）保持工作区悬置未提交
