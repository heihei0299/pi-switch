# 04: `proxyStartError` 去掉死返回 + 改正过度声称

Status: resolved (2026-09-11)

**两处**：(1) 返回 `(int, gin.H)` 而两臂都是 500、唯一调用点是 `c.JSON(proxyStartError(err))` → `int` 是死返回；
(2) 注释称「两种响应形状完全照旧」——**不成立**：`unmanaged listener owns …` 的文本不含
`already in use`，旧代码对它是 `{"error":…}`，现在多了 `message`。

**修**：只返回 `gin.H`，状态码写在调用点；注释改为准确描述（并说明这条小变化是有意的：类型化后
「端口被他人占用」的两种来源都得到同一个可操作提示）。

**验收**
- [ ] 签名不再含死返回；调用点显式写 500
- [ ] 注释与真实行为一致（含存在的那处小的 body 变化）
- [ ] `proxy_start_honesty_test.go` 的按类型分类断言（正反控制）保持绿
