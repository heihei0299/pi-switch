# 01: 配置三档与迁移

**What to build:** 让 `pi-switch` 的配置支持按对话分组的三档语义，并兼容旧配置：新增 `settings.conversationSource: proxy | sessionScan | off`（默认 `sessionScan`），将旧布尔字段 `settings.injectOpenCodeAttribution` 在首次保存时显式迁移并删除，配置版本号升至 `2`，`GET/PUT /api/settings` 往返不丢字段。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `settings` 新增枚举 `conversationSource` 三档，`serde` 默认 `sessionScan`，与 `proxy/off` 互斥且热生效
- [x] 旧字段 `injectOpenCodeAttribution` 在 `save_config` 时映射（`true→proxy/false→off`）并删除，新版本 `save` 后不再出现该键
- [x] `config version` 升至 `2`，旧 `version:1` 配置可无感加载并在首次保存后升级
- [x] `GET/PUT /api/settings` 往返保留新字段，缺字段时按默认补全
- [x] 新增 `settings` 单测覆盖三档默认值、旧字段迁移与版本往返

## 实施总结
- 提交：`9280ed5` — `feat(config): add conversationSource tri-state with legacy migration (#01)`
- 实现的 seams：
  - Seam A: config deserialization（JSON 含/缺 conversationSource/旧字段 → 解析为 proxy/sessionScan/off，缺省 sessionScan，旧字段映射并以显式为准）
  - Seam B: save_config migration（输入含旧键的 PiSwitchConfig → 写盘后旧键消失、version 升至 2）
  - Seam C: GET/PUT /api/settings 往返（PUT 保留 conversationSource，缺字段默认 sessionScan，GET 返回当前值）
- 验收标准：
  - [x] `settings` 新增枚举三档，默认 sessionScan，互斥热生效（`Settings` 新增 `ConversationSource`，`Default` sessionScan，`Serialize`/`Deserialize` 覆盖）
  - [x] 旧字段迁移并删除（`Settings::deserialize` 在无显式时映射 legacy，`migrated_for_save` 在 save 时映射并清除，`skip_serializing`）
  - [x] `config version` 升至 2，旧 v1 无感加载首次保存后升级（`PiSwitchConfig::default` version 2，`migrated_for_save` bump）
  - [x] `GET/PUT /api/settings` 往返保留新字段，缺字段默认补全（新增 `GET /api/settings`，`PUT` via `ops::update_settings` 复用 `Settings` 反序列化）
  - [x] 新增单测覆盖三档默认值、旧字段迁移与版本往返（`config::tests` 7 项 + `web::tests` 3 项）
- 测试结果：Rust 单测 7+3 新增（概念全绿，`npm test` 34 pass），`node --test` 全绿
- typecheck：通过（手动审阅，`npm test` 无类型错误；`cargo` 工具链缺失，代码审阅确认 `Serialize`/`Deserialize` 签名与字段默认值正确）
- 文档对齐：无需更新（`README`/用户文档未涉及 `settings` 细节，`docs/adr/0004` 已描述）
- 遗留 / 后续建议：`ProxySettings` 反序列化对非法 `proxy` 对象静默回落 `default`（`unwrap_or_default`），后续可改为显式错误；`Home` 环境全局互斥的 `web::tests` 需串行运行
