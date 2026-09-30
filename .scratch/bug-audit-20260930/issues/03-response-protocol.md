# 非流式响应协议错误
Status: resolved

## Requirements
- Chat 客户端→Responses upstream、Messages 客户端→Chat upstream 的返回值必须转换为客户端协议。
- 保留文本、结束原因、模型和 usage；响应转换必须与请求转换分开。
- 所有跨协议解析/转换失败在首次和 retry 路径均返回 502 conversion_error，不能静默透传。
- 同协议 passthrough 与现有 retry/header 策略保持。

## Validation
translator/server 回归通过。两轴审查已完成；Standards 指出的 incomplete 原因映射经回归修复并复核通过，Spec 无未解决发现。
