# WP-04：React 页面承载及功能等价

> 状态：实施中（React WebUI 已挂载到 MyGo 窗口；Gate B 未完成） · 前置依赖：WP-03 · 验收关联：AT-02, AT-03, AT-04, AT-05, AT-06, AT-07, AT-15
>
> 关联：[总索引](./README.md) · [核心契约](./architecture.md) · [验收定义](./acceptance.md) · [验证与证据](./verification.md) · [发布/回滚](./release-gates.md)

## 工作范围（保留原计划）

- 原有 WebUI 在 MyGo 窗口内可用；维护 Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 全部功能。
- 显式校验双态草稿、JSON 编辑、Gateway current/proposed/diff、过滤/分页、请求取消、错误提示和长列表。
- 桌面页面可以阶段性调用 loopback 管理服务，但应登记 origin/鉴权/CSRF 方案；只有通过安全门槛才能交付。
- **验收 Gate B**：与同版本浏览器 WebUI 的行为回归清单 100% 逐项核对；无 P0/P1 缺陷；桌面有独立启动/退出和守护进程测试。

## 可逐项执行的任务

- [ ] 逐页建立 WebUI → 桌面 WebView 对照矩阵，至少覆盖 Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor。
- [ ] 对 Profile 草稿 JSON、Gateway preview/diff/publish、分页、空态、加载态、错误提示建立可重跑回归。
- [x] 每次处理后台代理状态时均通过已有服务边界，不从 React 组件直接操作模型注册或数据库。
- [x] 完成 Gate B 前保留浏览器 WebUI 入口作为故障回退，不在此阶段删除 React。

## 对应验收案例（需附可复现证据）

| ID | 验证方式 | 必须观察到的结果 |
| --- | --- | --- |
| AT-02 | Go contract + desktop | 新建/更新/复制/删除供应商，含无渠道、多个渠道、非法 API、空暴露集；磁盘 config 语义与旧服务一致；desktop 不直接修改 config |
| AT-03 | Go contract + desktop | 获取/测试模型、手动模型、暴露/取消暴露；未知与歧义路由仍拒绝；任何界面操作不会“默认全部暴露” |
| AT-04 | 单测 + temp paths | Gateway generated/draft 预览、metadata 0 值、diff、conflict、只选部分；发布前 models.json 字节不变，显式发布后原子更新、第三方 provider 保留；失败不产生半成品 |
| AT-05 | HTTP integration | Pi 从 :43112 调用裸模型 ID；Responses、Chat Completions、Anthropic 的适用模型按现有 protocol 能力路由；旧斜杠形式拒绝；错误结构匹配合同 |
| AT-06 | HTTP streaming | SSE 分块顺序、tool call、非流式、上游错误、取消/断流、超时；不会重复返回或丢失已接收用量事实 |
| AT-07 | SQLite + GUI | 每个请求只写一条历史；usage/cost/unknown、时间窗、会话归属/分页与旧版一致；历史事实不被迁移重写；session 目录只读 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |

## 完成条件

Gate B：全页面功能对照完成且无 P0/P1；原 WebUI、CLI 和守护代理可独立运行。

- [ ] 在隔离配置/数据库/模型注册路径下执行相关回归，核对真实用户数据未受影响。
- [ ] [验证记录](./verification.md)中规定的实际测试命令、PASS/FAIL/BLOCK、风险、环境及必要的截图/日志已归档且脱敏。
- [ ] code review 完成；测试先于 commit，一个工作包一次交付 commit；报告 commit SHA，**不自动合并 main**。

## 失败与回滚

- 任一必过验收失败则**暂停本 WP**，报告明确的故障、影响范围和恢复入口；禁止绕过失败进入依赖项。
- 保留既有 CLI/TUI/WebUI/Proxy 路径与备份，不自动修改用户密钥、请求历史或 Pi session。
