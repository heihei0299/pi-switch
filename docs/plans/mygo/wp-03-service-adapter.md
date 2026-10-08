# WP-03：共享服务与桌面适配器

> 状态：实施中（desktop 已复用管理 REST router，并接入取消与 origin/host 限制；验收未完成） · 前置依赖：WP-02 · 验收关联：AT-02, AT-03, AT-04, AT-05, AT-06, AT-07, AT-10
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 先复用 internal/server 的管理 HTTP 契约；评估将逻辑抽成最小共享 application service 与 MyGo Bind。
- 逐项追踪 desktop action → application service → config/gateway/store，杜绝 UI 直接操作 config.json/models.json/requests.db。
- 为网络可取消请求和桌面调用添加上下文取消、错误映射及授权测试；兼容旧 API Schema decoder，新增契约测试。
- **验收**：AT-02～AT-07、AT-10；桌面与 HTTP 在相同夹具下产生相同事实/结果；浏览器页面无法越权调用 GUI 私有操作。

## 可逐项执行的任务

- [ ] 列出已有 WebUI API、CLI 调用与业务实现的调用链，标出可以复用的 application service。
- [x] 优先复用现有 REST 契约；确需 Bind 时抽取小接口，禁止桌面层直接读写 config/models/SQLite。
- [x] 建立统一的 DTO/错误/校验契约与取消语义，防止 HTTP 与桌面 RPC 出现两套判断。
- [x] 验证外部网页无法调用受信 MyGo Bind，回环/非回环管理 API 的授权和 CSRF/origin 策略保持有效。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-02 | Go contract + desktop | 新建/更新/复制/删除供应商，含无渠道、多个渠道、非法 API、空暴露集；磁盘 config 语义与旧服务一致；desktop 不直接修改 config |
| AT-03 | Go contract + desktop | 获取/测试模型、手动模型、暴露/取消暴露；未知与歧义路由仍拒绝；任何界面操作不会“默认全部暴露” |
| AT-04 | 单测 + temp paths | Gateway generated/draft 预览、metadata 0 值、diff、conflict、只选部分；发布前 models.json 字节不变，显式发布后原子更新、第三方 provider 保留；失败不产生半成品 |
| AT-05 | HTTP integration | Pi 从 :43112 调用裸模型 ID；Responses、Chat Completions、Anthropic 的适用模型按现有 protocol 能力路由；旧斜杠形式拒绝；错误结构匹配合同 |
| AT-06 | HTTP streaming | SSE 分块顺序、tool call、非流式、上游错误、取消/断流、超时；不会重复返回或丢失已接收用量事实 |
| AT-07 | SQLite + GUI | 每个请求只写一条历史；usage/cost/unknown、时间窗、会话归属/分页与旧版一致；历史事实不被迁移重写；session 目录只读 |
| AT-10 | 安全测试 | 外部 URL/普通浏览器不能调用私有 MyGo Bind；本机管理 API 的回环/非回环鉴权合同保留，拒绝恶意 origin 与未授权写入；Key 不进入日志/状态快照/错误弹窗 |

## 完成条件

REST/桌面采用同一事实来源，所有安全边界与请求取消测试通过，无重复业务逻辑。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
