# provider CLI 评审修复

**来源**：`code-review`（Standards + Spec 两轴）对 `b6a38b2...HEAD`（provider CLI 接线批次）的评审结论。
本批次只修评审发现的缺陷，不新增功能、不做架构调整。

## 目标

1. 收掉上一批次自己承认但未收敛的 ticket 04 缺口：`provider fetch-models --channel`。
2. 消灭「静默吞掉参数」这一类假成功（本 spec 存在的理由就是消灭它）。
3. 让错误分类不依赖错误文本，并恢复被无意改动的 HTTP 400 文案。
4. 让 help / README 与实际可用集合一致，并把该一致性落到测试上（不再靠人肉核对）。

## 判据

1. **不另写一套逻辑**：CLI 与 HTTP handler 共用同一实现（沿用上一批次已确立的 `CreateProfile`/`SetExposedModels` 模式）。
2. **不改服务端 JSON 契约**：除恢复 `as required` 外，任何 handler 的状态码与 body 文案保持等价。
3. **拒绝而非忽略**：CLI 子命令遇到未知 flag、缺失值、多余参数一律非零退出并说明原因；绝不静默把 flag 当 model id 或静默丢弃。
4. **测试走公开接口**：CLI 测试用 `provider show` 断言落盘效果，不直接读 config JSON；handler 测试用真实 HTTP。
5. **help/README 一致性由测试机械保证**，而不是靠 review 发现。

## 不在本批次范围

- A6（config 读取缓存）、A12（前端单体）——仍不可执行。
- 网关 Bearer 与 Basic 鉴权不匹配（ticket 02 的 F21 设计缺口）——需要产品决策。
- `remove`/`rm`/`ls` 别名是否要成为正式命令（本批次只把它们写进 help，不改行为）。
- `provider add` 与 WebUI 建出的 profile 形状分叉——涉及前端改动，另行处理。
