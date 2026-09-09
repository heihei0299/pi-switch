# 03: 完成旧 provider 迁移与手动字段保留

**What to build:** 用户首次从旧的 Supplier/Channel provider 结构发布时，旧 pi-switch provider 会被清理，模型和手动配置按已确认的优先级迁移到固定 provider；非 pi-switch provider 保留，后续再次发布不会重复迁移或覆盖用户在固定 provider 上的编辑。

**Blocked by:** 01: 实现固定双 provider 的正常发布路径

**Status:** resolved

- [x] 旧 Supplier/Channel pi-switch provider 被清理，`pi-switch-res` 与 `pi-switch-chat` 固定 key 被接管。
- [x] 非 pi-switch provider 原样保留。
- [x] 旧 model-level fields 按全局唯一 model id 迁移到对应固定 provider。
- [x] 固定 provider 已有的手动字段优先于旧 provider 的迁移字段。
- [x] 生成字段始终以当前 Supplier/Channel 配置为准；无法唯一归属的旧 provider-level 字段不迁移。
- [x] exposed model 删除会从固定 provider 中删除。
- [x] JSON editor 中的第三方 provider 仍可编辑和发布。
- [x] 迁移失败时不产生半成品文件。

## 实施总结
- 提交：`ab23d62` — `feat(gateway): migrate legacy provider fields`
- 实现的 seams：旧 provider 清理、model-level 字段迁移、固定 provider 字段优先、第三方 provider 保留、删除模型不回迁。
- 验收标准：以上 8 项全部通过。
- 测试结果：Go 相关包 159 tests passed；WebUI 29 files / 245 tests passed。
- typecheck：通过；WebUI 与 Go build：通过。
- 文档对齐：README 已补充 legacy provider migration 规则。
- 遗留 / 后续建议：无。
