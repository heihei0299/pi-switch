# 02: 加入聚合校验与原子失败

**What to build:** 用户遇到 unsupported API、全局重复裸 model id、非法 gateway JSON 或第三个伪造 pi-switch provider 时，preview 能给出可定位的诊断，publish 会被后端拒绝，旧配置和当前编辑内容都保持不变；非 pi-switch provider 仍可共存。

**Blocked by:** 01: 实现固定双 provider 的正常发布路径
**Status:** resolved
- [x] unsupported API 不生成第三 provider，并在 preview 中显示跳过原因。
- [x] 任意 Supplier/Channel 间重复裸 model id 都显示完整冲突来源；preview 与 publish 均拒绝发布。
- [x] 校验失败返回明确的 400 错误，且 `models.json` 内容保持不变。
- [x] 非法 JSON、非法 provider 结构或非法 model 结构不会写文件。
- [x] 第三个使用 pi-switch proxy 标识的 provider 被后端拒绝。
- [x] GatewayPanel 保留 JSON editor 内容并显示错误。
- [x] 用户维护的非 pi-switch provider 不被校验流程误删。

## 实施总结
- 提交：`99ee2f1` — `feat(gateway): reject invalid aggregated providers`；`22e284b` — `fix(gateway): split validation regression seams`
- 实现的 seams：unsupported API preview 诊断、重复裸 model id 来源校验、固定 provider contract 校验、第三 proxy provider 拒绝、失败原子性。
- 验收标准：以上 7 项全部通过。
- 测试结果：Go 相关包 156 tests passed；WebUI 29 files / 245 tests passed。
- typecheck：通过；WebUI 与 Go build：通过。
- 文档对齐：README 已补充 gateway validation 行为。
- 遗留 / 后续建议：无。
