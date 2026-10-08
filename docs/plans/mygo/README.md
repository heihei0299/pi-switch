# pi-switch → MyGo 桌面化/原生 UI 重写：执行与验收计划

> 状态：**计划基线（尚未开始实现）**；分支：**mygo**；制定日期：2026-10-08。
>
> 任务范围：本次提交**只记录执行与验收计划**，不改业务代码、不变更现有发布渠道、不创建 PR、不合并 main。
>
> 基线：pi-switch main = **18f877e60ef62e294bea8ad65bfebc5838ee96c1**（2026-10-03）；MyGo 上游为 [egoist/mygo](https://github.com/egoist/mygo)，调研时可用发布版本 **v0.3.3**（2026-10-08）。**版本是调研快照，不构成已锁定或已验证的依赖**。

## 0. 目标、边界、路线

### 0.1 项目目标

1. 为 pi-switch 增加 Windows、macOS、Linux 桌面入口：独立窗口、单实例、托盘菜单、系统通知、安装包和受控退出。
2. 保留已存在且经验证的 **Go 业务核心**，不重写 Proxy/Responses/Chat/Anthropic 转换、Gateway 发布、模型元数据、Token/费用、SQLite 统计、会话匹配、配置持久化。
3. **CLI / TUI / Headless Proxy 在没有图形环境时仍可独立运行**；桌面应用绝不能成为 Pi 模型调用的必要依赖。
4. 首先实现“桌面端与现有 WebUI 功能等价”；其后通过独立的原生 UI 技术验收决定是否把 React/Vite/WebView 全部替换为 MyGo ui。**完全原生是有条件的后续目标，不是未经验证的前提**。
5. 新旧版本读取同一用户数据，不产生第二套配置或互相覆盖；需要改变数据格式时必须单独制定可回滚迁移方案。

### 0.2 确定不做

- 不更改对外 API 请求格式、模型裸 ID 路由规则、两套固定 Gateway provider（pi-switch-res / pi-switch-chat）及显式发布语义。
- 不恢复已休眠的 failover 调度，不拓展供应商/协议功能，不顺手做无关重构。
- 不直接删除 CLI、TUI、npm 包、浏览器 WebUI 或已有发布工作流。
- 不自动接管用户已有的 daemon、修改用户私有 session 文件或收集额外遥测。
- 不以“UI 能打开”代替协议、数据、运行时、升级及安全验收。

### 0.3 路线与决策门

- **里程碑 M1 / 验证封装**：MyGo + 现有 React 页面（WebView）+ 现有 Go 服务；证明平台依赖、生命周期与管理数据访问可用。该里程碑是实验验证，不能称为完成重写。
- **里程碑 M2 / 可发布桌面版（优先）**：桌面外壳、系统功能、服务边界、构建、升级、诊断具备验收证据；复杂页面允许暂时保留 React。老 WebUI、CLI/TUI 保持可用。
- **里程碑 M3 / 可选完整原生重写**：所有可见页面（尤其 JSON 编辑器、Gateway diff、Stats 图表/会话浏览器）经过能力、输入法、可访问性和等价测试后，才能提出移除 React/Vite/WebView 的独立工作包。若任何关键条件不满足，停在 M2，不为追求“纯 Go”牺牲功能。

每个里程碑必须有“通过/未通过/豁免”记录；豁免需明确影响、替代路径和批准人，不能以“基本可用”作为门槛。

## 文件结构与执行顺序

这里**一个 WP 文件只定义一项可独立验证、独立提交的工作包**。共享技术事实与验证规则集中在下列参考文件，不再维护一份重复的大计划：

- [architecture.md](./architecture.md)：已有业务边界、数据契约、MyGo/Go 依赖和目标架构。
- [acceptance.md](./acceptance.md)：AT-01～AT-18 全量验收用例的权威定义。
- [verification.md](./verification.md)：执行命令、CI 门槛、验收记录模板。
- [release-gates.md](./release-gates.md)：暂停条件、回滚方式、正式交付准入标准。

| 工作包 | 独立计划 | 依赖 | 验收编号 |
| --- | --- | --- | --- |
| [WP-00](./wp-00-baseline.md) | 基线与继承失败隔离 | 无 | AT-01, AT-17 |
| [WP-01](./wp-01-spike.md) | MyGo 技术可行性原型 | WP-00 | AT-11, AT-12, AT-13, AT-14, AT-17 |
| [WP-02](./wp-02-desktop-shell.md) | 桌面外壳与代理生命周期 | Gate A | AT-08, AT-09, AT-11, AT-13 |
| [WP-03](./wp-03-service-adapter.md) | 共享服务与桌面适配器 | WP-02 | AT-02, AT-03, AT-04, AT-05, AT-06, AT-07, AT-10 |
| [WP-04](./wp-04-webview-parity.md) | React 页面承载及功能等价 | WP-03 | AT-02, AT-03, AT-04, AT-05, AT-06, AT-07, AT-15 |
| [WP-05](./wp-05-native-basic.md) | 简单页面原生 UI 迁移 | Gate B | AT-11, AT-12, AT-14, AT-15 |
| [WP-06](./wp-06-native-complex.md) | 复杂页面原生 UI 迁移与决策 | WP-05 | AT-04, AT-07, AT-11, AT-12, AT-14, AT-15 |
| [WP-07](./wp-07-distribution.md) | 构建、分发与升级 | Gate B（M3 可后续追加） | AT-01, AT-10, AT-12, AT-13, AT-14, AT-16, AT-17, AT-18 |
| [WP-08](./wp-08-retirement.md) | 旧 WebUI 退役（条件性） | Gate C + Gate D | AT-01, AT-15, AT-16, AT-18 |

## 执行约束

**工作约束：一个独立工作包/issue 对应一次交付 commit；一个明确 bug 单独 commit；先验收再 commit；不要把多个未验证修复混成一个 commit；commit 不等于合并。** 每个 PR 附运行命令、结果、未验证项和风险；每工作包独立 code review，最后执行一次全量验证。

## 完成与合并原则

- 不跨越 WP 的前置 Gate；WP-08 只在 Gate C 和 Gate D 全部通过后实施。
- 任一 WP 必须先通过相应 AT 用例、code review 和测试，再形成**一次交付 commit**；修复新发现的独立 bug 使用另一个独立 commit。
- 最后执行一次完整 CI + 跨平台真实 GUI 验收，随后**另外决定**是否合并；不自动 push 额外分支、创建 PR、打 tag 或发布。
- 文档改动本身不代表已测试 GUI/Proxy，也不代表已解决基线 CI 失败。
