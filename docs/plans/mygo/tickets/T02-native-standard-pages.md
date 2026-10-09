# T02 — 常规功能全面 MyGo Native 化

- **源需求**：SPEC §4、§6 WP-03
- **优先级**：P1
- **前置**：T01 完成并验证
- **交付**：常规管理流程均可在 MyGo 原生 GUI 中使用；本 Ticket 一个交付 Commit

## 目标与范围

实现并接通正式原生页面：`Overview`、`Profiles`、`Providers`、`Proxy`、`Settings`、`Backups`、`Doctor`。复用现有 Go 服务与数据模型，不通过 React 页面完成其中某个必要操作。Native 可重新设计布局，但不能丢失既有关键能力。

## 实施步骤

1. 在 T01 的 Shell/Controller 上建立统一原生导航、加载/空态/错误态、弹窗确认、焦点与键盘规范；页面 state 按域隔离，不新增一个总状态巨型 struct。
2. Overview 展示配置/当前 Profile、Proxy 身份和健康、近期状态，刷新必须反映真实事实，不用成功默认值掩盖错误。
3. Profiles 与 Providers 完成既有的查看、添加/编辑、切换和校验，保留模型/渠道/能力判定规则；凭据只按现有安全语义呈现，保存前校验及备份。
4. Proxy 完成显式启动/停止、刷新健康、地址/错误信息与非托管端口占用提示。GUI 退出或窗口隐藏不隐式停服务。
5. Settings、Backups、Doctor 覆盖现有设置、备份/恢复、诊断入口；重要变更需确认；错误可追踪且不泄露敏感值。
6. 对照旧 WebUI 功能制作本 Ticket 的常规页面差异记录，消除阻塞项；保留 WebUI 作为迁移期对照，不删除代码。

## 验收（全部必须）

- [ ] 七类常规页面仅依赖 MyGo Native widget 即可完成其现有关键操作，不需启动或嵌入 React 页面。
- [ ] Profiles/Providers 的新增、编辑、切换、失败校验与凭据遮蔽行为与共享核心一致。
- [ ] Proxy 只有显式操作才停止受管且身份匹配的进程；占用端口非本程序时禁止接管/误杀。
- [ ] Settings、备份恢复、诊断的错误/空态明确，取消操作与失败路径不破坏数据。
- [ ] 键盘导航、焦点、复制粘贴、主题/缩放、中文输入（涉及文本输入处）可以实际操作。
- [ ] 不写入第二份 provider 数据；不修改持久化格式；现有 WebUI 在迁移阶段仍可用于回退。

## 验证与证据

- `go -C desktop test ./...`；受影响领域包运行对应 `go test`，对危险写入使用临时目录、假 Key 和备份/恢复夹具。
- 在实际 Linux GUI 上演示：空数据 → 创建/切换 Profile → 提交/取消编辑 → 启动/停止受管 Proxy → 退出 GUI 后检查 Proxy → 诊断/备份恢复。
- Wayland/niri 中文 IME/窗口缩放及托盘若此阶段无法执行，明确 BLOCK，在 T06 最终验收前不得宣称通过。

## 不在范围内

Gateway JSON/Diff、Stats 图表、Sessions 和 Packages 复杂交互（T03）；WebView 默认入口切换（T04）；旧 UI 删除（T05）。

