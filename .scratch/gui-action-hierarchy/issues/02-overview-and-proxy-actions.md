# 02: 总览聚焦与代理操作按状态呈现

**What to build:** 用户打开现有总览即可看清当前供应商、完成切换，并根据代理真实状态执行适用操作；完整连接细节和辅助操作不再争抢主要区域。本票可直接在现有总览上完成，不依赖导航改版。

**Blocked by:** None (can start immediately).

**Status:** done

- [x] 总览优先展示当前供应商摘要、切换入口及代理状态；切换后的当前标识和成功或失败反馈准确。
- [x] 配置路径、完整连接详情与调试说明按需查看，仍然可达，但不常驻挤占主区域。
- [x] 根据真实代理状态突出当前适用的启动或停止操作，不同时突出相反操作；状态未知时提供检查入口，不把未知状态当作已停止或正常运行。
- [x] 已有手动刷新能力作为次要操作保留，不新增后台轮询，不擅自改变刷新机制。
- [x] 错误、身份异常、端口占用及需要处理的事项保持明确可见；不因收起详情而隐藏状态或操作失败，不因查看总览自动发布网关。
- [x] 代理身份与健康核验继续有效，只能停止已核验身份的受管进程；保留停止前的后果说明和确认，不接管未知监听者。
- [x] 隐藏或退出 GUI 不隐式停止代理，其他既有代理生命周期和配置语义不变。
- [x] 直接受影响的供应商切换和代理状态回归检查通过，覆盖未知状态、身份异常、端口占用及停止取消；实际 Native 窗口检查总览、键盘操作与不同状态并留存截图。缺少环境时明确记录未验证，不以 HTML 草图代替验收。

## Comments

**实现（2026-10-18，分支 `mygo`，改动未提交）**

- `desktop/native_overview_controller.go`：`view()` 重排为标题行（`pi-switch` + `Refresh Overview` 次要按钮）→ 错误/忙碌横幅 → `Current provider`（名称、合并后的 `API: … · Upstream: …`、模型统计）→ `Switch provider`（搜索 + 列表 + `Switch to selected profile` 主操作 + 切换反馈）→ `Proxy` → `Connection details` 折叠区（配置来源、`Proxy:`、`Web UI:`）。配置路径与连接细节不再常驻主区域。
- `desktop/native_proxy.go`：新增 `nativeProxyAction`（check/start/stop）与状态摘要/详情；`controls()` 只渲染一个当前适用的主操作——未检查或检查失败时是 `Check Proxy Status`，已检查但未运行时是 `Start Proxy`，身份与健康核验通过且确实在运行时才是 `Stop Proxy`；`Refresh Status` 作为次要手动刷新保留，`Proxy details` 折叠区承载完整状态细节；未知状态既读不成“已停止”也读不成“正在运行”。停止仍走 `Stop managed Proxy?` 确认对话框，说明在途请求可能失败、未知进程不会被停止。
- 切换供应商后若紧随的总览重载失败，反馈改为 `Switched to X; the overview could not be reloaded, so the current marker may be stale.`，不再出现“切换成功”与旧 `(current)` 标记互相矛盾的无保留结论。
- 未新增轮询、未改动 `proxycontrol` 语义；查看总览不会发布网关或写配置。

**测试**

- 新增 `desktop/native_proxy_test.go`：动作随核验状态切换、未知状态只提供检查入口、任一时刻只有一个适用操作、状态细节默认收起、身份不匹配信息保持可见。
- `desktop/native_overview_test.go`：总览交互用例扩展为点击 `Connection details` 后检查细节内容，并新增“收起细节时错误仍可见”“切换反馈与当前标识一致”“渲染总览不写任何文件”。
- “不发布网关”的断言原来检查的是临时目录里网关根本不会写的路径，现已用 `t.Setenv("PI_SWITCH_MODELS", …)` 把 `filegateway.ModelsPath()` 指到同一临时目录后再断言，断言从永真变为有效（顺带防止测试意外写入真实用户路径）。
- 新增 `TestNativeTaskHelpersRefuseWithoutAWindow`：无窗口时 `runStandardTask` 与代理 `runTask` 在进入任务登记前就拒绝执行，说明“页面未进入忙碌/未启动请求”这类断言不是空断言。

**验证**

- 串行回归：`GOMAXPROCS=2 go test -p 1 -parallel 1 -count=1 ./...`（desktop 模块）通过；唯一失败为既有问题 `TestNativeJSONSyntaxHighlightUsesThemeDanger`（HEAD `845a956` 同样失败）。`go vet ./...` 无输出。
- 实际 Native 窗口（容器 `ubuntu-test` Xvfb `:99` + openbox，隔离 run `/home/gui-test/pi-switch-gui/sandbox/runs/actions-gui-20261018212935`，隔离 config 端口 `proxy 49093` / `web 43111`）：
  - `shots/01-overview.png` 未检查/未运行：状态行 + `Start Proxy` 主操作 + `Refresh Status` 次要操作；
  - `shots/02-overview-running.png` 点击 `Start Proxy` 后：`Managed Proxy identity and health verified.`、`Proxy daemon started (PID 4530) on http://127.0.0.1:49093`、主操作变为 `Stop Proxy`（同一时刻没有并排的相反操作）；
  - `shots/03-stop-confirmation.png` `Stop managed Proxy?` 确认对话框（`Cancel` / `Stop`）；
  - `shots/04-overview-stopped.png` 确认后进程退出、`proxy.pid` 被清理。
- 当前页提示调整后复测：`shots/16-overview-nav-cue.png` 中 `Start Proxy` 是总览唯一强调填充按钮（导航用文字级当前标记），`Refresh Status` 保持次要外观。
- 本地 niri/Wayland 会话以 XWayland 客户端复现同一状态切换：`.scratch/gui-action-hierarchy/screenshots/niri-wayland-01-overview.png`（未运行）、`niri-wayland-02-proxy-running.png`（运行中，PID 469385）。
- 端口占用/未核验监听者已在真实窗口验证：先在隔离环境用 `python3 -m http.server 49093` 占用代理端口，再启动 GUI；检查后总览显示 `Proxy identity and health: unknown (check failed).` 与 `Proxy port is occupied by an unverified listener; refusing takeover. Use ss -ltnp to inspect the listener.`，主操作变为 `Check Proxy Status`（既不是 `Start Proxy` 也不是 `Stop Proxy`），外部监听者保持运行、未被接管（`shots/14-foreign-listener.png`、`shots/15-port-in-use.png`）。
- 未验证：容器内系统托盘不可用，`--start-minimized`/托盘生命周期未验证；Wayland 原生与真实高 DPI 未验证；停止过程中的信号升级（`force killed`）沿用既有 daemon 行为，未单独复测。

**复核修复（2026-10-18 第二轮，分支 `mygo`，改动未提交）**

- 总览标题行下方新增一行网关摘要（`Gateway: …` + 次要 `Open Gateway`），文案由新增的 `nativeGatewayStatusText()` 从共享的 `nativeGatewayState` 推导，不计算第二份 diff。未检查、加载中、读取失败、已加载但最近一次失败（可能过期）、发布中、有冲突、有待发布、已同步共八种状态分别有不同结论；未知与过期都不会被表述成“没有待发布”。
- 入口只导航：点击 `Open Gateway` 走既有 `navigate(pageGateway)`，不发布、不写配置、不轮询，也不新增主操作按钮或第二排主按钮。
- 刷新链路：`desktopShell.refreshNativeOverview()` 现在同时触发 `refreshGatewaySummary()`，因此启动、进入总览/供应商页、手动 `Refresh Overview`，以及切换当前供应商（`switchProfile` 写入 `current` 后）、保存/复制/删除供应商、保存设置、恢复备份等既有刷新点都会更新摘要；`nativeGatewayState.refresh()` 在预览加载中收到的新请求会记下并在完成时补查一次（`gatewayRefreshPending` / `flushRefresh`），最新状态不会被丢弃。请求参数继续用 `nativeGatewaySelections()`，保留网关页尚未发布的选择，并区分“默认选择”（nil）与“显式空选择”（空切片）。
- 测试：`desktop/native_overview_test.go` 新增 `TestNativeGatewayStatusTextDistinguishesUnknownFromClean`（八种状态的文案与警告色，含 nil 状态）、`TestNativeOverviewGatewayHintNavigatesWithoutPublishing`（渲染文案 + 导航到网关页 + 未发布 + 未写 config/`filegateway.ModelsPath()`）、`TestNativeOverviewGatewayRefreshKeepsPageSelection`（默认/显式空/显式选择三种刷新参数）、`TestNativeGatewayRefreshRemembersTriggerDuringLoad`（加载中的请求被记住并在完成后补查）.
- 验证：串行 desktop 回归通过（唯一失败仍是既有 `TestNativeJSONSyntaxHighlightUsesThemeDanger`）；`go vet ./...` 无输出。实际 Native 窗口（容器 `ubuntu-test` Xvfb `:99` + openbox；隔离 run `/home/gui-test/pi-switch-gui/sandbox/runs/actions-gui-20261018212935` 与 `/home/gui-test/pi-switch-gui/sandbox/runs/gateway-hint-20261018`）：`.scratch/gui-action-hierarchy/screenshots/22-overview-gateway-uptodate.png`（打开即有 `Gateway: up to date; nothing to publish`，无需先进入网关页）、`23-overview-gateway-pending.png`（`Gateway: 2 model change(s) ready to publish; publishing stays explicit`）、`24-gateway-page-from-hint.png`（点击 `Open Gateway` 后进入网关页，`pending: 2`，`Publish selected models` 仍需显式确认；本次会话前后 `models.json` 与 `config/config.json` 校验和一致）。
- 版面代价：总览比之前高一行。1000×760 窗口下 `Start Proxy`、`Refresh Status` 与 `Proxy details` 仍在首屏内，托盘不可用的提示文字落到首屏下沿（滚动可达）；未改动画廊提示的位置，以免把与本票无关的提示挤走。
- 未验证：冲突态、读取失败态与“发布后收敛”只在无头测试中覆盖，未在实体窗口构造对应网关状态；窗口宽度小于 1000px 时的换行/裁剪未单独截图。

**审核修复（2026-10-18 第四轮）**

- 发布期间排队的刷新现在也会补查：新增 `completePublish`，成功后失败后都清发布状态、释放任务并 `flushRefresh`；`publishSelection` 与 `publishDraft` 都走它。补查经 `refreshGateway` 入口重放，排队的选择（含显式空选择）原样带回。
- 新增 `TestNativeGatewayPublishReplaysQueuedRefresh`（成功/失败两个子用例）：去掉补查调用时该测试失败，恢复后通过。
- 说明：总览提示行本身随页面改版落在第一个提交里（`native_overview_controller.go` 是整函数重写，无法拆出单独 hunk），本提交补齐网关侧的队列与重放。
