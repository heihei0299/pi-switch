# 02 — Profile 映射字段扩展

**What to build:** 使自定义名称的 profile（如 `my-openai`）能显式映射到模型目录的 provider（如 `openai`），未配置时按 preset 自动推断，推断失败静默跳过 enrich，不阻塞保存。

**Blocked by:** 无 — 可与 01 并行

**Status:** done

- [x] `ProviderProfile` 新增可选字段 `modelsDevProvider?: string`（`src-rust/config.rs`，`Option<String>`，`skip_serializing_if`），`webui/src/types.ts` 同步
- [x] 映射优先级：`modelsDevProvider` 显式值优先，未填时按 preset→目录 key 推断（openrouter/anthropic/deepseek/openai/siliconflow 等），推断失败跳过 enrich 不报错
- [x] `validate_provider_profile` 中对 `modelsDevProvider` 仅 warning（未知 key 跳过，不阻断保存，避免目录新增 provider 时硬编码滞后）
- [x] 现有 profile 无该字段时反序列化兼容（`None`），round-trip 不丢字段
- [x] 术语与文案统一使用“模型目录 / 模型元数据”

Commit: 517c269af2fa45876f677f0d8fadef79f227ac40
