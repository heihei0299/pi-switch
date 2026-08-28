# 04 — 可观测与降级 polish

**What to build:** 让用户在 Fetch 成功时感知 enrich 结果与降级情况，频繁 Fetch 命中缓存，离线或失败时有明确提示。

**Blocked by:** 03

**Status:** done

- [x] enrich 结果 `enriched=N skipped=M failed=K` 合并到现有 Fetch 成功 toast 与 `web.rs` 响应中，`skipped` 为目录未覆盖数，`failed` 为网络/解析失败数，失败原因写入日志
- [x] 24h 内多次 Fetch 命中本地缓存，无重复网络请求；过期或网络失败时回退到过期缓存并报告 warning，无缓存且失败时跳过 enrich 并提示
- [x] 离线场景（有 24h 内缓存）仍能完成 enrich，无缓存且无网络时保留原值并提示失败原因
- [x] 文案与日志统一使用 CONTEXT.md 术语（模型目录 / 模型元数据），与“上游模型列表”区分

commit: 64c05c1 (64c05c1df6c6244d7fa4cea7c8a170415343ad43)
