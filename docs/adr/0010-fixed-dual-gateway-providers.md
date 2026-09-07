# 0010 固定双 provider 的网关聚合视图

Status: accepted

网关不再按 Supplier/Channel 生成多个 provider，而是将 exposed models 按客户端 API contract 聚合到固定的 `pi-switch-res`（Responses）与 `pi-switch-chat`（Chat）两个 provider 中。这样可以避免真实上游数量直接泄漏到客户端配置，同时保持裸 model id、正确的 API 声明和本地 proxy 路由；该决策 supersede ADR-0009。

## Considered Options

- 按每个 Channel 生成 provider：API 表达准确，但 provider 数量随上游增长，并将内部路由结构暴露给客户端，否决。
- 所有模型放进一个 provider：无法同时准确声明 Responses 与 Chat API，否决。
- 允许重复 model id 并按入口 provider 消歧：需要改变现有 proxy 路由语义，增加运行时耦合，否决。
- 保留旧 provider 并额外添加两个固定 provider：会产生重复模型和陈旧配置，否决。

## Consequences

- `models.json` 中 pi-switch 管理的 provider 数量稳定为最多两个，非 pi-switch provider 仍可共存。
- exposed model 的裸 id 必须全局唯一，重复 id 不再通过 provider 隔离。
- 旧按 Channel 组织的 provider 需要在首次发布时清理，并按唯一 model id 迁移可识别的手动字段。
- session affinity 必须按模型下沉，不能在聚合 provider 级别统一设置。
