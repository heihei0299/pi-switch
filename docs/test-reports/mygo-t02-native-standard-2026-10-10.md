# T02 Native 常规页面 — 实施中记录（2026-10-10）

## 状态与范围

- 当前分支：`mygo`；恢复时 HEAD：`a148f495049a05954db7c2949e058492d4ac4417`（T01）。
- **T02 尚未完成真实 GUI 验收；本次按用户“直接提交 t02”的明确指令提交实现和局部验证记录。T03～T06 未启动。** 提交不代表 Ticket 验收通过或独立审查批准；后文“未提交”等表述是提交前的阶段记录。
- 当前主代理串行实施和自审；未使用子代理。当前工具没有可核实的实际模型/思考等级信息，不宣称已确认指定型号。
- 保留 WebUI/WebView 默认入口及所有旧 UI，Native 常规页面可从 File / tray 的 `Show Native Overview` 打开，也可在新进程使用 `--native` 跳过旧管理路由/协议注册并直接创建 MyGo widget 主窗口。正式默认入口切换属于 T04；旧 WebUI 包仍链接并在 init 时计算嵌入资源指纹，不宣称已移除 WebView 依赖。
- 保留既有 `.idea/.gitignore`、`.idea/vcs.xml` 暂存，以及其他无关工作区内容；未修改原有 `docs/plans/mygo/HANDOFF.md`。

## 本轮代码进展

1. 原生窗口增加 Overview、Profiles、Providers、Proxy、Settings、Backups、Doctor 导航；域状态分别放在对应页面类型中。
2. Profiles/Providers 首版支持列表过滤、创建、编辑、重命名、删除、切换，以及基于现有 profile/channel/model/retry 核心规则的只读校验。全部 provider 字段、渠道和模型元数据通过 MyGo TextArea 编辑，不借 HTTP handler。
3. 已有 API Key、header 值和全部已存 URL 以保留标记呈现（URL path 也可能带凭据）；替换 API Key 使用 Password 输入。标记指向 revision 校验过的原始字段，不依赖当前草稿的渠道名称/顺序，支持重命名/重排；伪造标记和未知替换渠道拒绝。旧 flat/单一未命名渠道只在编辑器中复用核心规则物化为 main，打开编辑器不迁移磁盘文件。取消清空替换 Key，失败保留草稿。
4. Settings 使用同一 settings schema，可读写现有字段；保存检查并发修改、retry、端口范围，返回实际持久化的规范化 revision。非法 JSON 保留原文并禁用保存。
5. 本地备份存放在 `<config path>.backups`：目录必须是非 symlink、私有目录，备份文件 `0600`，保存 exact pre-write bytes。确认后的恢复使用现有跨进程配置锁和原子写入；损坏的当前 JSON 也可以恢复，恢复前仍保留损坏原文。恢复检查原始文件 revision、文件名、regular-file 和配置 capability gate。
6. Doctor 与 HTTP 诊断共用 `application.DiagnoseConfig`，字段保持 `ok/msg` 与 `level/path/message`，空问题列表保持 JSON array；原生页面不显示原始配置解析错误中可能出现的敏感值。完整 config 写入和恢复共用既有 capability gate。
7. 原生 Stop Proxy 增加确认；Proxy 更新优先投递到原生窗口，不需要借 WebView 更新业务状态。隐藏/退出行为与守护进程身份规则不变。
8. `Upstream.MarshalJSON` 保留 `exposedModels` 缺省、显式 `[]`、非空数组三种不同状态，避免显式取消暴露在一次保存后变为缺省。

## 与旧 WebUI 的差异 / 剩余工作

| 功能 | 当前 Native 状态 | 后续要求 |
| --- | --- | --- |
| Overview / Proxy | 通过 application 读取配置来源（含缺文件 defaults 标签）；Proxy 额外显示实际监听地址、已识别 PID、检查时间，失败明确 unknown；写入期间的刷新合并到旧任务结束后执行 | 真实启停/退出后进程、非托管占用以及窗口/托盘验收 |
| Provider CRUD / 切换 / 校验 / 复制 | 已接通；复制调用共享 domain copy gate，包含已有凭据但不复制未保存草稿、不切换 current、不覆盖已有目标，先确认和备份 | 最终两轴 Review；真实 GUI 从空数据完成操作 |
| 预设 / 连接测试 | 预设目录从 server 移至 application，旧 HTTP/CLI 入口委托同一目录；只提供 CanProxy 预设，草稿空 Key、零暴露。连接测试复用核心，显示成功/失败和耗时，不显示回源正文 | 真实 GUI 及中文输入验收 |
| 模型池 / exposure / 回源 | 在渠道 JSON 中编辑；共享 fetch/enrich/merge 核心只添加新 ID，保留已有 metadata、其他渠道和 exposure。Native 抓取只更新带保留标记的草稿，用户确认 Save 才写 config | 抓取与 HTTP 的即时持久化有意不同；仍可能刷新既有 models.dev 元数据 cache，不声称所有文件零写入 |
| Supplier usage | 显式读取保存过的 provider 主渠道；真实 rolling/weekly/monthly、有效 reset 时间，缺 percent 显示 unknown，真实 0 保留为 0，失败可重试，不显示任意回源字符串 | 使用当前保存值而非未保存编辑值；不自动回源，不伪造余额；真实 GUI 验收 |
| Settings | 全字段原生 JSON 编辑、确认、冲突、备份、失败保留；显示当前 Native binary 的 Go build identity，缺失字段明确 unknown | Native build identity 不冒充 WebUI 资源 fingerprint；后者仍可从迁移期 WebUI 检查 |
| Backups | 本地创建/显式恢复已接通；列表按修改时间倒序排列，同时间按文件名排序；选择后显示真实名称、时间、字节数，创建消息返回名称；旧 WebUI 仍显示未实现并返回 501 | 静态校验/权限错误已提供可追踪且不回显原始输入的提示；真实 GUI 选择/确认/失败恢复仍待验证；不增加加密导入导出/云同步 |
| Doctor | 共用诊断；原生加载错误脱敏；空态明确 | 完整对照/真实 GUI 操作验收；继续审查敏感内容 |
| IME / 焦点 | 聚合 provider 名称、密码、渠道、复制名称、JSON 输入的 composition；动作行拦截 MyGo 失焦清除 preedit 后的同一次 pointer press，防止禁用按钮在释放前启用 | 这是控件模拟证据，不是真实 Wayland/niri IME 证据；T03 编辑器需在自身 Ticket 中覆盖同一问题，本轮未修改复杂页面 |
| 正式 GUI 与入口独立性 | 旧 WebView 默认入口未改，Native 页面无 HTTP handler 业务依赖；`--native` 启动分支不调用旧 UI registrar，WindowOptions 使用 Content 而非 URL | 独立分支已由局部测试/调用链证明，但真实 GUI 新进程启动仍未验证；不能借此提前切换 T04 默认入口或宣称已去除旧依赖 |
| 系统主题 / 缩放 / 托盘 | 无真实 GUI 实测 | Linux Wayland/niri 实机门禁 **BLOCK** |

最终纯 Native 目标未缩小。功能入口已补充，但不能把局部控件测试当作七页完整交互、启动独立性和真实 GUI 门禁通过；T02 仍不可交付或推进 T03。

## 实际检查

执行环境：Linux amd64，`go version go1.27.1-X:nodwarf5 linux/amd64`；Go 模块与构建缓存使用既有本地缓存。
测试均使用 `mktemp -d /tmp/pi-switch-t02-*.XXXXXX` 创建临时 HOME；配置、备份、Key、models/sessions/database 使用各测试临时目录与假数据。未读取/修改真实配置或 Key。

基础命令环境：

```bash
HOME="$(mktemp -d /tmp/pi-switch-t02-test.XXXXXX)" \
GOMODCACHE=/home/shial/go/pkg/mod GOCACHE=/home/shial/.cache/go-build \
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test <受影响包>
```

初次基线检查还显式设置临时 `XDG_CONFIG_HOME`；当前会话检查发现 XDG_CONFIG_HOME / PI_SWITCH_CONFIG / PI_SWITCH_CONFIG_DIR / PI_AGENT_ROOT 均 unset。未联网安装依赖、未改锁文件。

| 检查 | 结果 | 证据范围 |
| --- | --- | --- |
| `go test ./internal/application ./internal/config ./internal/profile ./internal/server`（恢复后基线） | PASS | 原 T02 草稿涉及领域与 HTTP 回归；不是最终目标全量验收 |
| `go test ./internal/application`（新增/修改后） | PASS | 凭据遮蔽/按渠道名恢复、Key 替换、CRUD 冲突、取消/非法草稿零写入、exact backup、私有权限、symlink/traversal 拒绝、备份失败不覆盖、损坏配置恢复、capability 拒绝、Settings revision/默认值 |
| `go test ./internal/config` | PASS | 既有配置回归；exposure 三态与其他 upstream 字段 round-trip |
| `go test ./internal/server` | PASS | 诊断/capability 抽取后的管理 API 回归 |
| `go -C desktop test ./...` | PASS | 七页导航/空态、已存凭据和密码输入默认遮蔽、确认取消保留草稿、丢弃草稿清空 Key、非法 JSON 禁止保存、Stop 确认、加载错误不显示 raw secret，加上既有桌面局部测试 |
| `go -C desktop vet ./...` | PASS | 桌面模块静态检查 |
| `go vet ./internal/application ./internal/config ./internal/server` | PASS | 受影响 Go 层静态检查 |
| `git diff --check -- desktop internal/application internal/config/config.go internal/config/config_test.go internal/server/profile_handlers.go` | PASS | 已跟踪改动无 whitespace 错误；新 Go 文件均 gofmt |
| Native/application 引用检查 | PASS（本轮新增文件） | 新页面无 `internal/server`/HTTP handler 业务依赖；未声称最终全仓架构审计通过 |
| Wayland/niri、真实中文 IME、系统主题/缩放、托盘 | BLOCK | 当前会话 `DISPLAY` / `WAYLAND_DISPLAY` 为空，`XDG_SESSION_TYPE=tty`；无可用的真实 GUI 实测证据 |
| 全量 root 检查、CI、Linux 交付构建/安装、arm64 交叉编译、发布 | 未执行 | 非本轮局部检查范围；需按 T06 和实际操作授权推进 |

过程中遇到的失败均为本轮引入并已修复：MyGo 控件名应为 `TextInput` 而非 `TextField`；测试 fixture 的 channel 缺少 required api；新配置测试缺 json import；Settings 测试误把非法显式空 conversationSource 当成可缺省值。对应修复后的检查已 PASS，不冒充既有失败。

## 主代理串行自审（阶段性，非 Ticket 最终 Review）

- **规范/架构/安全**：检查页面状态隔离、后台结果投递、持久化锁/原子写入、源 revision、秘密遮蔽、路径与权限。通过回归用例核对源 revision 内的字段引用，避免渠道重命名/重排交叉绑定凭据；自审发现并修复未知替换渠道被忽略、损坏配置无法恢复、备份目录 symlink/权限以及 Settings 保存后 revision 不一致等问题。旧入口未删除；没有新事实来源，backup 仅为恢复快照。
- **功能/Spec**：原生 CRUD/复制/预设/连接测试/回源模型/usage/Settings/本地备份/Doctor 及 Native 独立启动分支均有局部证据；备份标识和安全错误提示已完成本轮自审。仍需完整实际交互、真实 GUI 验收及据此完成最终两轴 Review。**T02 不能验收通过**。
- **已知限制**：JSON-first 表单还需交互实测；备份含明文凭据，不能当成加密同步；退出 GUI 对底层同步文件/网络 I/O 的强制取消仍沿用 T01 的已知限制。本轮不执行真实 Proxy 管理操作。

## 恢复入口

1. `git status --short` 核对当前草稿和保留的无关暂存项；不要清理、reset 或覆盖。
2. T02 的 Native 启动分支、备份标识和静态错误反馈已有代码/局部证据；使用 `desktop/README.md` 的隔离启动方法完成实际交互验收，按上表解决剩余验收缺口，不把 T03 的复杂页面实现混进来。
3. 在真实 Linux GUI 环境完成 T02 的空数据→创建/切换→提交/取消→Proxy→退出后 Proxy→诊断/备份恢复门禁，再做两轴最终自审及一个精确文件范围的 T02 交付 Commit。
4. T02 全部门禁通过后才按当前目标进入 T03；默认入口切换、旧 UI 具体删除清单和 Linux 最终交付授权仍遵守 T04～T06。

## 续工实际证据（供应商操作 / 凭据引用 / 刷新 / IME）

上一回合属于 progress（实际增加页面、服务、测试），本回合继续修改上述权威文件。HEAD 未变，仍未提交 T02。

本轮新增或涉及：
- application：provider_operations、provider_usage、provider_presets、overview 与测试；修改 provider_draft，使保留标记绑定原始字段引用。
- profile：DuplicateInConfig、FetchChannelModelEntries、MergeChannelModels 从现有实现抽取，HTTP/CLI 仍使用同一 gate/metadata 规则；增加 read-only legacy 输入不变测试。
- config/server：User-Agent 继承规则共享；usage 与 preset HTTP 适配器改为委托服务，usage 错误不再回显响应正文。
- desktop：供应商操作、实际 Proxy 身份/监听快照、Native build identity；配置源读取移至 application；刷新请求合并；IME pointer press 保护。

所有 test 命令使用临时 HOME 与 XDG_CONFIG_HOME；使用既有本地 GOMODCACHE/GOCACHE，并设置 GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off。回源使用 httptest 和新鲜的临时 catalog fixture，没有请求真实供应商或使用真实 Key。

| 实际命令 / 操作 | 结果 | 范围 |
| --- | --- | --- |
| `go test ./internal/application ./internal/config ./internal/profile` | PASS | 新供应商操作、usage、默认值/UA、源字段引用、旧 shape 兼容；部分未受后续修改影响的结果复用 |
| `go test ./internal/application`（最后服务修改后） | PASS | 重命名/重排凭据、legacy editor 零写入、Overview 来源/错误脱敏，加上所有 application 局部回归 |
| `go test ./internal/profile ./internal/server ./cmd/pi-switch -run 'Test(Duplicate\|DoorParity_Duplicate\|Credits\|ChannelFetch\|FetchEnrich\|ProviderPresets\|FetchChannelModelEntries\|HandlePresets\|HandleProvider_(Duplicate\|FetchModels)\|ProviderCLIProbeAndFetch\|BuildOutbound)'` | PASS | 共享实现的直接 HTTP/CLI 消费者，原错误分类/UA/exposure/metadata 语义 |
| `go -C desktop test -run '^(TestNativeProviderCompositionBlocksSaveAndDuplicate\|TestNativeSettingsPreeditBlurCannotTriggerSave)$' -count=1` | PASS | 先复现 disabled action 因 IME blur 同次点击启用的失败，再加动作行 pointer guard 验证修复 |
| `go -C desktop test ./...`（最后桌面修改后） | PASS | 原有桌面测试 + 新预设/复制确认取消、usage unknown/zero、实际 Proxy listener、build identity、IME、刷新合并；不是 GUI 实测 |
| `go vet ./internal/application ./internal/config ./internal/profile ./internal/server` | PASS | 受影响 Go 核心/适配器 |
| `go -C desktop vet ./...`（最后桌面修改后） | PASS | 桌面模块静态检查 |

本轮临时失败：抽取 DuplicateInConfig 时漏写一个括号，gofmt 已定位并修复；新 IME 回归测试确实复现失焦/禁用按钮问题，修复后 targeted test 和完整 desktop test 均通过。未将这些失败归为继承失败。

主代理阶段性自审：
- 规范/架构/安全：复用 domain gate 而非复制规则；新页面不 import server；已有 URI、Key、header 不返回编辑器原值；伪造引用/冲突/回源失败不写 config。对旧 shape 的编辑器物化不写盘；复制仍允许 advisory legacy shape，以匹配原 core gate。
- 功能/Spec：复制、预设、测试、模型抓取、usage 操作有代码和局部证据；此阶段的 Native 启动独立性尚未核实，后续证据见下一节。真实 GUI 及剩余实际交互仍未通过。没有作独立 Review、Ticket 交付 Commit、Push、T04 切换或 T05 删除。

## 续工实际证据（独立验证入口 / 备份标识 / 错误反馈）

- `prepareDesktopShell` 在 `--native` 分支不调用旧管理 UI registrar；原生主窗口使用 MyGo Content surface，URL 为空，复用原主窗口关闭策略（无托盘退出、有托盘隐藏）。默认 WebView、single-instance、Proxy 生命周期保持原行为。第二次启动参数可能转发到已初始化 WebUI 的既有进程，因此独立启动证据必须使用新进程。
- 旧 server 包仍链接；已读其 init 路径，当前只计算嵌入 WebUI build fingerprint，而非执行 React 或注册旧路由。保留依赖是迁移事实，不等于 T04/T05 已完成。
- 备份列表新增文件系统 ModifiedAt，最新优先、同时间按名称稳定排序；选中项显示名称/时间/大小，创建成功返回文件名。取消恢复保留选择且不产生写入，已有 revision/锁/backup/capability/原子写入路径未削减。
- 非法 JSON、备份名/目录、端口、retry/capability 校验均使用静态安全错误；重复名称、缺失渠道、权限失败提供明确提示，不显示底层原始输入/路径或任意上游正文。

所有检查仍使用临时 HOME/XDG_CONFIG_HOME、既有本地 Go 缓存与禁止下载设置：

| 实际命令 / 操作 | 结果 | 范围 |
| --- | --- | --- |
| `go -C desktop test -run 'Test(NativeStartup\|DefaultStartup\|NativePrimaryClose\|StartsHidden\|MenusKeep\|WindowClose)' -count=1` | PASS | Native 不调用旧 registrar，默认入口/错误传播不变；新主窗口关闭/最小化策略 |
| `go -C desktop test ./...` | PASS | 新增备份名称/时间/大小与恢复取消、已知错误脱敏，及桌面局部回归 |
| `go test ./internal/application` | PASS | 时间排序与稳定 tie、静态安全校验 hint，及 application 局部回归 |
| `go vet ./internal/application`；`go -C desktop vet ./...` | PASS | 本轮修改涉及的两个包/模块 |
| 相关 `git diff --check`；新 Go 文件 `gofmt -l` | PASS | 跟踪修改无 whitespace 问题；新增 Go 文件无格式差异 |
| `desktop/README.md` 新 Native 隔离启动片段 `bash -n` | PASS | 仅检查说明中的 shell 语法，未执行 `go run`、启动 GUI 或管理 Proxy |
| GUI 环境检查 | BLOCK | DISPLAY/WAYLAND_DISPLAY 为空、XDG_SESSION_TYPE=tty；`/run/user/1000` 未发现 `wayland-*` socket，当前用户未见 niri/weston/Xorg/Xwayland/gnome-shell/kwin_wayland/sway 进程；未启动 GUI |

本轮初次 application test 因新增测试多余的 config import 编译失败；移除后再次检查 PASS。这是本轮引入并修复的失败，不是既有失败。后续仅改说明文档，未重复已通过且未受影响的测试。

主代理串行两轴阶段自审（假定单用户桌面、CLI/HTTP 可同时写配置）：
- **规范/架构/安全**：沿用 core 配置锁/原子写入与实际文件 revision；Native 新页面不借 HTTP handler；启动参数不改变默认入口；备份元数据不新增持久化 schema；静态错误不泄露原始输入。当前范围没有新增必须修复的代码问题，不代表全仓审计或独立批准。
- **功能/Spec**：独立启动分支、备份标识和已知错误反馈的代码/局部测试通过。无真实 GUI 时，七页完整工作流、中文 IME/焦点/键盘、主题/缩放/托盘以及显式 Proxy 生命周期仍为 **BLOCK**。这是未满足的强制门禁，不作 Ship/验收通过结论，不提交 T02、不进入 T03。

下一步：在真实 Linux GUI 使用隔离路径与假 Key，完成 Ticket 的逐项 GUI 验收并记录可观测证据；再完成最终两轴自审和精确文件范围的交付 Commit。未执行依赖安装、完整交付 build、系统安装、交叉编译、CI、发布、Push、真实 Proxy 操作或 T05 旧入口删除。

## 续工实际证据（Doctor 校验消息脱敏）

上一目标回合为 progress：修改了 Native 启动分支、备份反馈及验证文档。本回合先核对实际 HEAD/工作区与门禁；GUI 环境仍无显示连接或 compositor，未重复执行未受影响的检查。

代码审查随后找到 T02 错误反馈的具体缺口，并用两个失败回归复现：
- `%q` 输出的带引号/换行 API Key 没有匹配原始字符串替换，凭据仍出现在校验错误中。
- Doctor 的 `modelsDevProvider`/retry 校验消息未经现有凭据遮蔽；假数据将已存 URL/Key 误放入无效字段时，消息回显了 URL 密码/query 或 Key。

修复范围仅为共享 application 服务与测试：抽出已有的消息遮蔽函数，覆盖完整已存 URL、原始/Go-quoted 凭据形式，并在 provider Doctor issues 上复用；全局 retry 诊断使用静态安全 hint。保留诊断 level/path、非敏感字段信息与核心校验判定；不改变配置/备份格式，不写磁盘。Native Doctor 与旧 HTTP diagnostics 消费同一安全消息，不新增一套规则。

| 实际命令 / 操作 | 结果 | 范围 |
| --- | --- | --- |
| `go test ./internal/application -run '^Test(DiagnoseConfigHidesCredentialsInValidationMessages\|ValidationErrorRedactsQuotedCredentials)$' -count=1`（修复前） | FAIL，已修复 | 两个回归分别复现 Doctor URL 泄漏及 quoted Key 泄漏；非继承测试故障 |
| `go test ./internal/application`（修复后） | PASS | 新脱敏/字段路径/诊断零写入回归与所有 application 局部测试 |
| `go test ./internal/server -run '^(TestValidate_(ReportsKnownButUnproxyableAPI\|ReportsProfileShapeIssues\|ReturnsLevelPathMessage)\|TestApiContract_08_S3_ValidateOnlyHint)$' -count=1` | PASS | 直接 HTTP diagnostics 消费者的 level/path/message 与原能力/shape 校验契约 |
| `go vet ./internal/application`；修改 Go 文件 `gofmt -l`；相关 `git diff --check` | PASS | 修改范围静态/格式检查；未运行全仓检查 |

检查仍使用隔离临时 HOME/XDG_CONFIG_HOME、假 Key、本地 Go 缓存、GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off；未访问真实供应商或用户数据。主代理串行两轴阶段自审：规范/安全侧确认修复位于 application、没有削减 core gate 或文件写保护；功能侧确认错误仍有字段定位、非敏感既有 HTTP 契约通过。当前尚未证明真实 GUI 工作流，不作最终 Ticket 验收或独立 Review 声明。

HEAD 保持 T01，未提交 T02、未推进 T03～T06；`.idea/.gitignore` 和 `.idea/vcs.xml` 暂存仍保留。下一步仍是按上一节与 `desktop/README.md` 在实际 Linux GUI 完成门禁。备份明文凭据及底层同步 I/O 取消限制仍存在；本轮未执行 GUI、真实 Proxy 操作、依赖安装、发布、交叉编译或旧入口删除。
