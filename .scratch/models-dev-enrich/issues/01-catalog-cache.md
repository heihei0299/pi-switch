# 01 — 目录缓存基础设施

**What to build:** 提供模型目录 `https://models.dev/api.json` 的拉取、24h TTL 本地缓存与降级能力，供后续 enrich 复用；用户在 Fetch 时无需每次拉 4.4MB 全量，离线或失败时可回退到缓存或跳过。

**Blocked by:** 无 — 可立即开始

**Status:** done

- [x] `GET https://models.dev/api.json` 10s 超时，成功后原子写入 `~/.pi-switch/cache/models-dev.json`（与 `backups` 同级，`config_dir()` 推导）
- [x] 暴露 `get_or_refresh_catalog()`：TTL 24h 内命中直接读缓存，过期或无缓存时尝试网络，失败回退到过期缓存并 warning，无缓存且失败则跳过 enrich
- [x] 目录 payload 全量缓存不切片，约 4.4MB，读写原子性与并发安全
- [x] 测试以 fixture `api.json` 片段注入，不依赖真实网络，覆盖命中/过期/网络失败降级分支
- [x] 尊重 ADR 0005 与 CONTEXT.md 术语（模型目录 / 模型元数据）

commit: e606941
