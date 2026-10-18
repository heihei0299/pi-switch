# 03: 供应商列表与编辑任务分离

**What to build:** 用户先查找、选择供应商，再进入添加或编辑任务；不再同时面对列表、预设、大段 JSON 和所有辅助按钮。编辑以保存和取消为主，其他操作按目的分组；保留现有 JSON 编辑器，不另建表单编辑器。本票可以在现有供应商页面独立完成。

**Blocked by:** None (can start immediately).

**Status:** done

- [x] 列表状态提供查找、选择、添加与进入所选项编辑的清晰入口，不同时铺开完整编辑工具。
- [x] 预设归入添加流程，添加空白供应商及从预设添加两种现有能力均可用，不常驻挤占列表区域。
- [x] 现有 JSON 编辑器只在添加或编辑任务中出现，编辑时突出保存与取消；不新建另一套表单或改变草稿数据格式。
- [x] 测试连接、获取模型、查询用量、复制及手动校验按目的分组、按需展开，仍可发现和使用；明确区分针对草稿和针对已保存配置的操作，不因展开区域而自动触发网络请求。
- [x] 删除放入单独的危险操作区，保留后果说明及确认；取消危险操作不修改配置。
- [x] 保存前验证、无效 JSON 提示、凭据遮蔽及输入法组合输入保护保持有效；不能因收起手动校验按钮而跳过验证。
- [x] 导航往返、收起工具区域或切换编辑任务不静默丢失未保存修改；取消编辑沿用明确弃稿语义，不隐式保存。
- [x] 进行中状态、错误和操作结果保持可见；保存供应商不自动发布网关，不改变已有业务契约。
- [x] 对直接受影响的列表、添加与预设、编辑保存及取消、辅助操作可达性、危险操作和草稿保留补充必要回归检查；实际 Native 窗口检查列表及编辑布局、键盘路径与输入法保护并留存截图，缺少环境的项目明确记录未验证。

## Comments

**实现（2026-10-18，分支 `mygo`，改动未提交）**

- `desktop/native_providers.go`：`nativeProvidersPage` 增加 `adding` / `advancedOpen` / `deleteOpen`，`view()` 改为分派到 `listView` / `addView` / `editView` 与确认对话框。列表页只有标题、说明、搜索、列表和 `Add provider…`（主操作）、`Edit selected provider`、`Switch selected profile`、`Refresh providers`；添加页提供 `Add blank provider`、`Cancel add`、`Provider preset` 选择与 `Add from preset`；编辑页以名称/渠道/上游输入和 `Provider / channels / models JSON` 编辑器为主，`Save provider…` 主操作 + `Cancel provider draft`，随后是 `More provider actions` 折叠区与独立的 `Danger zone: delete provider`；消息与忙碌状态移到顶部。
- `desktop/native_provider_operations.go`：辅助操作按目的分组为 `Against the draft`（渠道输入、`Validate provider`、`Test draft connection`、`Fetch models into draft`）与 `Against the saved provider`（复制名称、`Refresh saved provider usage`、`Duplicate saved provider…`、用量行）；未保存时后者返回 `Save the provider to use saved-provider actions.`，展开区域不会自动发起网络请求。
- 保留原有 JSON 编辑器与草稿数据格式，未新增表单编辑器；`blockComposingLive()`（`desktop/native_standard.go`）在事件发生时才读取组合输入状态，使位于输入框上方的保存/取消行不会因 IME 组合输入触发写入。危险区的 `Delete provider…` 也包在同一守卫里（它移到折叠区后曾丢掉该保护），`cancelDraft()` 同时清空 `composing`。

**测试**

- 新增 `desktop/native_providers_test.go`：列表隐藏编辑工具、添加流程同时提供空白与预设、编辑突出保存/取消、辅助操作分组且按需展开、删除留在危险区并带确认、取消编辑显式回到列表、无效草稿阻止保存且保留草稿。
- `desktop/native_standard_test.go`：预设、网络操作与 IME 组合输入用例改为先进入 `Add provider…` / 展开 `More provider actions`，并按无头测试需要设置 `ReduceMotion`（折叠面板动画在无头帧中不会推进）。IME 用例扩展为 `TestNativeProviderCompositionBlocksSaveDuplicateAndDelete`，覆盖 `Delete provider…`；去掉危险区守卫后该用例确实失败（`IME preedit triggered write confirmation: … button=Delete provider…`），恢复守卫后通过。
- 测试中的页面名改用 `pageProviders` / `pageSettings` / `pageDoctor` / `pageBackups` 等常量，避免拼写错误静默落进 `standardView` 的 `default` 分支。

**验证**

- 串行回归：`GOMAXPROCS=2 go test -p 1 -parallel 1 -count=1 ./...`（desktop 模块）通过；唯一失败为既有问题 `TestNativeJSONSyntaxHighlightUsesThemeDanger`（HEAD `845a956` 同样失败）。`go vet ./...` 无输出。
- 实际 Native 窗口（容器 `ubuntu-test` Xvfb `:99` + openbox，隔离 run `/home/gui-test/pi-switch-gui/sandbox/runs/actions-gui-20261018212935`）：`shots/05-providers-list.png`（列表页只提供查找/选择/添加/进入编辑）、`shots/06-provider-edit.png`（编辑页顶部，凭据以 `__PI_SWITCH_KEEP_CREDENTIAL__` 标记遮蔽）、`shots/07-provider-edit-actions.png`（`Save provider…` 主操作 + `Cancel provider draft`，`More provider actions` 与 `Danger zone: delete provider` 分别收起）、`shots/08-provider-edit-more.png`、`shots/09-provider-edit-groups.png`（`Against the draft` 与 `Against the saved provider` 两组）。
- 当前页提示调整后复测（单一强调按钮层级）：`shots/17-providers-nav-cue.png`（列表页只有 `Add provider…` 一个强调填充按钮）、`shots/20-provider-edit-nav-cue.png`（编辑页顶部与凭据遮蔽，`Save provider…` 仍是编辑页唯一强调填充按钮，见 `shots/07-provider-edit-actions.png`）、`shots/21-keyboard-focus-add-provider.png`（键盘焦点环与当前页标记可区分）。
- 未验证：真实输入法下的 IME 组合输入保护未在本轮复测（只有无头测试与既有 Fcitx5/Wayland 记录）；容器内系统托盘不可用；真实高 DPI 与 Wayland 原生未验证。

**复核修复（2026-10-18 第二轮，分支 `mygo`，改动未提交）**

- 缺陷：`desktop/native_provider_operations.go` 的 `Against the draft` 行使用 `blockComposingPress(p.composing)`，而该行在 `Saved provider copy name` 输入框之前渲染，捕获的是尚未包含该输入框组合状态的旧值；在复制名称框里组合输入时点击 `Validate provider` / `Test draft connection` / `Fetch models into draft` 仍会发起请求。
- 修复：该行与 `Against the saved provider` 行都改用 `blockComposingLive(func() bool { return p.composing })`，在按下事件派发时读取最新组合状态；既有禁用条件、业务调用、保存/复制/删除保护与全局输入处理均未改动。
- 测试：新增 `TestNativeProviderCompositionBlocksDraftNetworkButtons`，三个动作各一个子用例——复制名称框组合输入中点按不得启动任务，提交候选词后同一按钮仍可用。为让“页面是否发起请求”可观察，`desktopShell` 增加仅测试使用的 `taskRequested` 钩子（`runStandardTask` 在窗口检查之前回调），无窗口时任务照旧被拒绝，断言不再依赖恒为 false 的 `busy`。
- 红/绿：临时把该行改回 `blockComposingPress(p.composing)`，三个子用例全部失败（`a preedit in the copy name started work through …`）；恢复后通过。顺带把 `TestNativeProviderNetworkControlsBlockInvalidDraft` 与 `TestNativeProviderAuxiliaryActionsAreGroupedAndLazy` 中只断言 `p.busy` 的空断言改为同时断言 `taskRequested` 未触发。
- 验证：`GOMAXPROCS=2 go test -p 1 -parallel 1 -count=1 ./...`（desktop）通过，唯一失败仍是既有 `TestNativeJSONSyntaxHighlightUsesThemeDanger`；`go vet ./...` 无输出。
- 未验证：真实输入法下“组合输入中点按草稿网络按钮”仍未在实体窗口复测。本轮 X11 测试会话（Xvfb `:99`）没有输入法；既有 Fcitx5/Wayland 会话只验证过输入与候选提交，容器内也无法向嵌套 Wayland 客户端注入指针点击。
