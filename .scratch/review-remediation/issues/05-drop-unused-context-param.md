# 05: `gatewayHealthPayload` 去掉未使用参数

Status: resolved (2026-09-11)

**问题**：`gatewayHealthPayload(c *gin.Context)` 内部从未使用 `c`（只用 `configPath()` 与
`gateway.ModelsPath()`）；`gofmt`/`vet` 看不见这类问题。

**修**：去掉参数，`handleGatewayHealth` 直接 `c.JSON(200, gatewayHealthPayload())`，
与同文件里已经不带 `c` 的 `enrichProposedModels`/`buildGatewayPlan` 一致。

**验收**
- [ ] 函数不再接收未使用参数；行为与响应体逐字节不变（票 03 的健康测试保持绿）
