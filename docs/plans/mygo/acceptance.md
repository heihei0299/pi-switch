# 全量验收用例 AT-01～AT-18

> 这是唯一的 AT 编号定义。各 WP 文件引用对应验收 ID，并补充工作包完成条件；不要分叉同一用例的判断标准。

## 4. 验收用例（逐项记录证据）

下列 AT-* 是必须维护的验收 ID。每个记录包含：环境（OS/架构/桌面/显示 scale/Go/MyGo 版本）、操作与夹具、预期、实际、日志或测试链接、pass/fail/block、操作者、日期。涉及 API Key 的截图/日志全部脱敏。

| ID | 测试方式 | 操作与可观察断言 |
| --- | --- | --- |
| AT-01 | CI + CLI | 原有 Go、WebUI、Playwright、smoke、跨平台构建通过；记录并单独关闭继承的 README 测试失败 |
| AT-02 | Go contract + desktop | 新建/更新/复制/删除供应商，含无渠道、多个渠道、非法 API、空暴露集；磁盘 config 语义与旧服务一致；desktop 不直接修改 config |
| AT-03 | Go contract + desktop | 获取/测试模型、手动模型、暴露/取消暴露；未知与歧义路由仍拒绝；任何界面操作不会“默认全部暴露” |
| AT-04 | 单测 + temp paths | Gateway generated/draft 预览、metadata 0 值、diff、conflict、只选部分；发布前 models.json 字节不变，显式发布后原子更新、第三方 provider 保留；失败不产生半成品 |
| AT-05 | HTTP integration | Pi 从 :43112 调用裸模型 ID；Responses、Chat Completions、Anthropic 的适用模型按现有 protocol 能力路由；旧斜杠形式拒绝；错误结构匹配合同 |
| AT-06 | HTTP streaming | SSE 分块顺序、tool call、非流式、上游错误、取消/断流、超时；不会重复返回或丢失已接收用量事实 |
| AT-07 | SQLite + GUI | 每个请求只写一条历史；usage/cost/unknown、时间窗、会话归属/分页与旧版一致；历史事实不被迁移重写；session 目录只读 |
| AT-08 | 进程集成 | 窗口关闭→已运行 Proxy 仍可 :43112/healthz 且能服务请求；“退出桌面”不杀代理；显式“停止代理”仅作用于正确身份；GUI 重启不重复创建 |
| AT-09 | 进程集成 | 端口被无关程序占用、旧 PID、损坏锁、崩溃、双实例、用户登录启动；不得误杀/接管，不以 PID 存在伪称 healthy |
| AT-10 | 安全测试 | 外部 URL/普通浏览器不能调用私有 MyGo Bind；本机管理 API 的回环/非回环鉴权合同保留，拒绝恶意 origin 与未授权写入；Key 不进入日志/状态快照/错误弹窗 |
| AT-11 | Linux 实机 | Arch+niri/Wayland：打开/隐藏、焦点、菜单、粘贴、fcitx5 中文组合与候选、3200×2000@scale2 的缩放/点击位置/滚动；无残影、文字截断、候选错位 |
| AT-12 | Windows/macOS 实机 | Windows 10/11 WebView2/IME/高 DPI/单实例；macOS WKWebView/托盘/睡眠恢复/签名状态。每个受支持目标都运行可交互 GUI smoke，缺设备标记 block，不拿交叉编译充数 |
| AT-13 | Linux 降级 | libayatana-appindicator3 缺失、无 system tray、WebKitGTK 缺失/版本不匹配、Wayland portal 不支持快捷键：应用不死锁，可找到替代入口，错误可读；托盘仅菜单模式 |
| AT-14 | 统计基准 | 同一硬件、同一配置、冷/热启动各 5 次；测窗口可交互耗时、空闲 60 s 的 CPU/RSS、典型 100/1000 项页面滚动；提交原始数据与对照，Gate B 前批准阈值，不凭单次测量声称性能提升 |
| AT-15 | GUI 回归 | Home、Profiles、Proxy、Gateway、Stats/Conversations、Packages、Settings/Backups/Doctor 逐屏核对：空态、读写、保存、错误、草稿/JSON 编辑、键盘、长列表、主题 |
| AT-16 | 安装与回滚 | 干净系统安装、旧版本升级、备份/恢复、卸载桌面、恢复旧 CLI；config/models.json/requests.db 未丢失、Key 未泄露、daemon 不遗留异常进程 |
| AT-17 | 依赖与秘密 | 固定依赖/锁文件、依赖许可证、敏感信息扫描、打包内容检查、构建身份与 commit 可追溯；未经签名的发行物明确标注不可正式分发 |
| AT-18 | CI/发布隔离 | main 的原六目标 CLI build 不退化；desktop 分离构建并在相应平台跑可执行 smoke；tag/npm 发布条件不被 desktop PR 误触发 |

**安全测试方式**：以临时文件隔离 PI_SWITCH_CONFIG、PI_SWITCH_CONFIG_DIR、PI_SWITCH_DB、PI_SWITCH_MODELS（四个变量同时设置），启动前验证读取的是临时空配置，完成后检查真实用户目录未变化。禁止将真 API Key、现有工作目录的 secrets 复制到 CI 或 public issue。
