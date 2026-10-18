# 01: 导航去重与维护入口分组

**What to build:** 用户通过总览、供应商、网关、用量与会话四个主要页面，以及独立的设置与维护入口，找到所有现有功能，不再面对四排导航按钮。本票只整理入口和页面组织，不等待总览内容或供应商编辑布局的后续调整。

**Blocked by:** None (can start immediately).

**Status:** done

- [x] 一级导航只保留四个主要页面和分开的设置与维护入口，不再平铺四排按钮；本票不要求额外完成全应用汉化。
- [x] Profiles 与 Providers 合并为一个供应商入口，原有查找、添加、编辑与切换能力仍可到达。
- [x] 代理日常控制可从总览使用，不再占用独立一级导航入口；本票不改变代理状态和操作逻辑。
- [x] 设置、软件包、备份与恢复、诊断、高级 JSON 编辑在维护区按用途分组，均有清晰可达的二级入口，不堆进一个无分组的“更多”菜单。
- [x] 导航往返不静默丢弃未保存的供应商或设置草稿，也不意外触发保存、网关发布或代理启停。
- [x] 主入口与二级入口可通过键盘访问，当前页面明确，切换后的焦点和布局可用。
- [x] 不删除现有能力，不改配置或数据库格式，不新增 UI 依赖，不混入无关工作树修改。
- [x] 直接受影响的导航、功能可达性和草稿保留回归检查通过；在实际 Native 窗口检查导航与维护区并留存截图。缺少运行环境时记录未验证，不能用源码或 HTML 草图替代通过。

## Comments

**实现（2026-10-18，分支 `mygo`，改动未提交）**

- `desktop/native_standard.go`：新增 `pageOverview` / `pageProviders` / `pageGateway` / `pageUsage` / `pageMaintenance` 与维护区二级页面常量；`standardView` 改为单行导航（四个主要页面 + `ui.Spacer` + `Maintenance`），当前页用加粗 + 强调色文字标记（`ui.Button(...).Bold().TextColor(theme.Accent)`），不占用强调色填充按钮，因此每个页面仍只有一个强调色的主操作；维护区二级页面（如 Settings）由“Maintenance 保持当前标记 + 页面标题”共同识别；`maintenanceView` 按 `Configuration`（Settings、Advanced JSON）/`Recovery`（Backups）/`Extensions`（Packages）/`Diagnostics`（Doctor）分组；新增 `navigate()`，只做打开页面所需的数据加载（Gateway 预览、Usage 首次刷新、Settings 首次加载、Backups 加载、Doctor 运行），不写配置、不发布网关、不启停代理。
- 删除原来的四排导航、独立 `Proxy` 页和 `Profiles` 入口；`nativeOverviewController.proxyData()` 因此成为死代码，一并删除。
- 供应商入口合并后，查找、添加、编辑与切换全部在 Providers 页面可达（见票 03）。

**测试**

- 新增 `desktop/native_nav_test.go`：单行导航与去重（五个入口同一行、`Providers` 只出现一次、旧入口消失、`Proxy` 只是总览的一节）、维护区分组与五个二级入口的内容、导航往返保留供应商/设置草稿且不产生配置写入、代理未被动过、仅键盘（Tab/Space/Enter）打开主入口与二级入口。
- `TestNativeStandardNavigationAndEmptyStates` 改为覆盖新导航集合；`TestNativeStopProxyRequiresConfirmation` 不再依赖已删除的 `Proxy` 页。

**验证**

- 串行回归：`GOMAXPROCS=2 go test -p 1 -parallel 1 -count=1 ./...`（desktop 模块）通过；唯一失败是既有问题 `TestNativeJSONSyntaxHighlightUsesThemeDanger`，在 HEAD `845a956` 的临时干净检出（验证后已删除）上同样失败，非本票引入。`go vet ./...` 无输出。
- 实际 Native 窗口（本地构建，容器 `ubuntu-test` 的 Xvfb `:99` + openbox，隔离 run `/home/gui-test/pi-switch-gui/sandbox/runs/actions-gui-20261018212935`）：`shots/10-maintenance.png`（维护区分组与四个用途标题）、`shots/11-settings.png`（二级入口打开后 `Maintenance` 仍保持当前标记）、`shots/13-keyboard-nav.png`（仅键盘 Tab Tab Return 打开 Gateway）、`shots/05-providers-list.png`（合并后的供应商入口）。
- 当前页提示调整后重新构建并复测：`shots/16-overview-nav-cue.png`（`Overview` 加粗强调色、`Start Proxy` 是唯一强调填充按钮）、`shots/17-providers-nav-cue.png`（`Providers` 当前标记、`Add provider…` 是唯一强调填充按钮）、`shots/18-maintenance-nav-cue.png`（`Maintenance` 当前标记、维护区文案已改为“never writes config or publishes the Gateway”）、`shots/20-provider-edit-nav-cue.png` 与 `shots/21-keyboard-focus-add-provider.png`（键盘 Tab 焦点环与当前页标记可区分）。
- 环境限制：容器内该实例运行约 1 分钟后指针点击不再送达应用（Xvfb/openbox 仍在运行、XTEST 键盘正常），因此 `20-provider-edit-nav-cue.png` 与 `21-keyboard-focus-add-provider.png` 改用键盘 Tab/Return 操作；指针点击相关的证据来自该实例冻结之前的截图。
- 另在本地 niri/Wayland 会话以 XWayland 客户端（2x 缩放）复现同一导航布局与代理区，截图 `.scratch/gui-action-hierarchy/screenshots/niri-wayland-01-overview.png`、`niri-wayland-02-proxy-running.png`。
- 未验证：真实输入法下的 IME 组合输入（本轮未复测）；容器内系统托盘不可用（缺少 libayatana-appindicator3），托盘入口未验证；Wayland 原生（非 XWayland）与真实高 DPI/分数缩放未验证；Windows/macOS 未验证。
