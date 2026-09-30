# 供应商探测与模型拉取遗漏 headers
Status: resolved

## Requirements
- provider test / fetch-models（HTTP 与 CLI）使用主 channel 的有效 URL/API key 及合并后的 profile/channel headers。
- header 名大小写不敏感，channel 值覆盖 profile，显式 Authorization 覆盖自动 Bearer。
- channel 定向 fetch 同样继承 profile 的 credentials/headers，复用已有合并策略；只合并新增模型，不改其它 channel。
- 只读 test/fetch 不修改 config 或写请求统计；真实 proxy 继续使用同一 header 合并规则。

## Validation
- public HTTP/CLI 红测原 403 / empty baseURL，修后主 channel probe/fetch 与定向 fetch 成功。
- config/profile/server/cmd 包通过；只读 config 字节不变、请求统计为零、原 proxy header 路径继续成功。
- Standards / Spec 审查均未发现遗留问题。
