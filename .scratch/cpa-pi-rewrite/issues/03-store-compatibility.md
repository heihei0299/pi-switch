Labels: wayfinder:grilling
Type: grilling
Status: resolved

## Question

“完全照搬” CPA 的 `store` 三后端（PG/git/object-store）是否在本次重写内？`pi-switch` 的单文件 `config.json` + `requests.log` 追加式不可变语义（`ADR-0004`）是冻结还是允许 `config schema v2 + 自动迁移`？`watcher` 热重载是仅白名单 `settings.proxy/circuitBreaker` 还是全量？

Blocked by:

## Answer

- **Store**：冻结单文件 `config.json` + `requests.log` 追加式，不引入 CPA 的 PG/git/object-store 三后端（YAGNI，符合 ADR-0004 单用户本地定位）。
- **Watcher**：仅白名单热重载（`settings.proxy/circuitBreaker` 等），其余字段需重启生效。
- **迁移**：允许 `config schema v2 + 自动迁移`（首次保存时迁移旧字段，如 `injectOpenCodeAttribution→conversationSource` 已有先例），但 `requests.log` 追加式语义不变。

决议已与用户在 03 票 grilling 中确认（2026-09-01）。
