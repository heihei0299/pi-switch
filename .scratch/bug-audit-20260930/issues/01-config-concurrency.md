# 配置并发写入丢失更新
Status: resolved

## Requirements
- 所有配置局部变更跨进程串行执行 load → validate/mutate → atomic save，保留其它成功变更。
- CLI、HTTP、TUI 使用同一事务边界；整文件替换获取相同写锁，但保留替换语义。
- 不改变既有 profile 校验、响应格式或 rename 业务规则。
- 锁等待有界，进程退出自动释放；配置仍为 0600 原子写入。
- 回归覆盖并发创建、跨进程更新、回调失败零写入。

## Validation
Go 全量与配置/调用方回归通过；Windows、Darwin 配置包交叉编译通过。Standards 与 Spec 审查的错误优先级发现已修复，复核无遗留发现。
