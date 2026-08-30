# 0006 供应商与网关彻底逻辑独立，仅显式发布

供应商（`config.json: profiles`）为唯一事实来源，网关（`models.json: providers[prefix]`）为只读派生视图，二者在存储、逻辑与运行时彻底隔离，任何供应商或设置变更均不自动写网关，仅经网关发布（`PUT /models/gateway`）显式落盘；同进程内通过独立文件锁/原子写、独立 Router/mod 与独立健康检查实现双向错误隔离，为未来独立进程预留接口但本次不真拆进程。

## Considered Options
- 保留 `update_settings` 与启动时自动同步：减少用户操作但破坏单一发布路径可审计性，遂否决
- 同步真拆为双进程：彻底隔离但引入双守护进程运维与 IPC 复杂度，待多上游加权调度确需时再拆，遂本次仅预留 `health_check/start_placeholder/gateway.notify` 占位
- 网关立即扇出多上游：提前改变契约但当前仍以 `primary_*` 聚合即可满足，遂保留聚合口径不变

## Consequences
- 启动与设置变更仅产生 pending + mismatch 横幅，不落盘；需用户到 GatewayPanel 显式“应用到 Pi”
- 网关失败永不阻断供应商 CRUD，反之亦然；两者各有独立 health 与错误转换
