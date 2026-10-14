# W26 Native GUI 环境验收（Ubuntu-test follow-up）

- 日期：2026-10-14
- 工作树：`/home/shial/Project/pi-switch-mygo-v2-20261014094522`
- 分支：`codex/mygo-v2-20261014094522`
- GUI/CLI build identity：`64c469b`（测试时 HEAD；随后仅更新本报告，未改 Go/Desktop 源码）
- 实际目标机：`gui-test@10.10.10.244`，Ubuntu 24.04.4 LTS
- 总状态：**BLOCKED**（Ubuntu/Niri/Wayland 软件 GUI 子集已通过；Noctalia、HiDPI 与物理设备门禁不可验证）
- 本次目标 OS：Ubuntu 24.04；Arch 环境不是本次必达项。

## 结论

按用户后续指示，改在 `ubuntu-test` 主机执行隔离 Native GUI 验收。该主机运行 Ubuntu 24.04.4、Niri 26.04、Wayland socket `wayland-1` 和 Fcitx5 5.1.7；MyGo 以 `GDK_BACKEND=wayland` 原生 Wayland 客户端启动。中文拼音候选及提交、Provider 添加/保存/重载、Gateway 发布、JSON Gateway draft 修改/校验/发布、Stats/Sessions 隔离数据展示、窗口关闭重开以及 GUI 退出后 Proxy 继续运行均有实际交互证据。

此会话是运行在既有 Xvfb `:100`（1600×1000×24）上的嵌套 Niri/Wayland，会使用 llvmpipe。它证明 Wayland 客户端、IME 软件路径和 UI 交互，但不是实体显示器/实体键盘/硬件加速验收。Noctalia 未安装，桌面没有 `/dev/dri` 或 `/dev/input`，3200×2000/scale 2 与物理输入/显示仍未验证；所以不把整个 W26 标为 PASS，也不把这个软件会话冒充物理实机结果。

此前首次尝试的本地 `arch-lxc` shell 没有 Wayland 环境，已记录为 BLOCKED。该尝试未启动 GUI/Proxy；本报告现以用户要求的 ubuntu-test follow-up 结果补充并更新状态。

## 环境与隔离

| 检查 | 结果 | 证据 |
|---|---|---|
| SSH 主机验证 | PASS | 临时 known_hosts 中 ED25519 指纹与已确认指纹 `SHA256:GGay+nN2Gs9Q2aKCWQrfCGy0PsZY/4bi1nagWaro7nw` 一致；连接使用 `StrictHostKeyChecking=yes` |
| OS | PASS | `/etc/os-release`: Ubuntu 24.04.4 LTS (`VERSION_ID=24.04`) |
| Niri/Wayland | PASS | Niri `26.04`；现存 Niri 进程环境 `XDG_SESSION_TYPE=wayland`, `XDG_CURRENT_DESKTOP=niri`, `WAYLAND_DISPLAY=wayland-1`; GUI 日志确认走 Wayland |
| Fcitx5 | PASS | Fcitx5 `5.1.7`; `fcitx5-remote -n` 返回 `pinyin` |
| Noctalia | BLOCKED | `noctalia`、`noctalia-shell` 均未安装/未找到 |
| 显示/输入设备 | BLOCKED | Niri 嵌套运行于 `Xvfb :100 -screen 0 1600x1000x24`; `/dev/dri`、`/dev/input` 均不存在 |
| Go 工具链 | UNVERIFIED | 主机 `/usr/local/go/bin/go` 为 Go `1.24.2`，低于 Desktop `go.mod` 要求的 Go `1.27.1`；未改系统工具链。使用开发机 Go `1.27.2-X:nodwarf5` 构建本次 CLI/Native 二进制后复制到隔离 run |
| 隔离目录 | PASS | `/home/gui-test/pi-switch-gui/sandbox/runs/w26-ubuntu-20261014` 下独立 `home/`, `xdg/`, `config/`, `agent/`, `models.json`, `bin/`, `logs/`；不使用真实用户配置/数据 |

构建产物：

- CLI SHA256：`77c1fc67052ae53eaaf03baeb616776a82d2b2be8fe7145b42e2d40bc8a90aab`
- Native GUI SHA256：`7695ff42707150dbeb13f0f2244198780a4ce4600e3eda585ea3e1f4ee91aab7`
- `--build-info`: `commit=64c469b`, `dirty=false`, `target=linux/amd64`, `version=20261011.0.0`
- 隔离 `config.json` 与 `requests.db` 权限为 `0600`。

## 逐项验收

| 验收项 | 状态 | 结果/限制 |
|---|---|---|
| Native 窗口启动、无崩溃/白屏 | PASS | 最新 W26 构建在 Niri Wayland 会话运行；日志显示 llvmpipe 软件渲染 |
| 3200×2000、scale 2 布局 | BLOCKED | 可用画面仅 1600×1000 Xvfb；物理 DPI/缩放不存在，未模拟替代 |
| Fcitx5 中文输入 | PASS | 输入 `nihao` 显示拼音候选，Space 提交为“你好”；证据是 GUI 候选/提交截图。输入事件经测试 Xvfb 合成，不代表实体键盘验收 |
| Provider 编辑/保存/重载 | PASS | 将隔离 profile 名改为 `w26-test-provider`，UI 报告已保存并创建 pre-write backup；关闭后重开，Overview 显示该 profile，配置 key 与之匹配 |
| 通过 GUI 添加新 Provider | PASS | 复用外层 Xvfb `:100` → Niri 输入路径，在纯 Wayland Native GUI 中 Add `w26-added-provider`，填入 dummy API key，GUI 保存确认、列表出现新 Provider；关闭后重启 GUI，Provider 仍出现在列表，隔离 config key 核对通过 |
| Gateway 发布 | PASS | 选择 fake `gpt-4o-mini` 并发布，仅写入隔离 `models.json` |
| JSON Editor 显示/修改/保存 | PASS | 加载隔离 Gateway draft，将 `proxy` 从 `false` 改为 `true`；UI 显示校验通过且有 1 个 pending change，确认发布；随后只读核对隔离 `models.json` 中 `proxy=true` |
| Stats/Sessions 显示隔离数据 | PASS | 在隔离 SQLite 中插入一条明确标记的合成请求；UI 显示 1 request 和 `W26 synthetic GUI validation` 会话 |
| 窗口关闭/重新打开 | PASS | GUI 窗口关闭后对应进程退出；用同一隔离环境重新启动，保存的 Provider 和 Proxy 状态可见 |
| Noctalia/系统托盘 | BLOCKED | Noctalia 未安装，系统托盘不可用；不能把托盘集成标为 PASS |
| File 菜单回退（软件交互） | PASS | F10 成功展开 File/Edit；菜单包含 `Show pi-switch`、`Refresh Proxy Status`、`Start Proxy`、`Stop Proxy`、`Quit`。选择 Show 后 GUI 继续存活；选择 Quit 后隔离 GUI PID `14255` 退出 |
| Alt+F10 快捷键 | UNVERIFIED | 前轮合成 `Alt+F10` 未展开菜单；该输入结果不能证明快捷键 PASS |
| 不依赖旧 WebView | PASS | 实际运行的是 `--native` MyGo Native 可执行文件；未启动旧 WebView 路径 |
| Proxy 启动/状态/健康/停止 | PASS | 隔离端口 `127.0.0.1:43145`：CLI start/status、`/healthz` 均成功，最终停止并确认端口释放 |
| GUI 退出后 Proxy 存活 | PASS | GUI 正常关闭后 Proxy PID `12603` 仍在；CLI status 与 `/healthz` 通过；之后仅停止此隔离 Proxy |
| 物理 GPU/键盘/seat | BLOCKED | `/dev/dri`、`/dev/input` 缺失；不作硬件能力推断 |

## 命令结果与证据

- SSH 首次使用默认 known_hosts 失败（exit `255`）；没有关闭主机密钥校验。用 `ssh-keyscan` 取 ED25519 公钥并核对已确认指纹后，以临时 known_hosts + `StrictHostKeyChecking=yes` 重连成功。
- 主机检查：`/etc/os-release`、`/usr/local/go/bin/go version`、`/usr/local/bin/niri --version`、Fcitx5 查询均成功；Noctalia 命令检查为未安装。
- `bash scripts/build-go.sh "$OUT/pi-switch"`：exit `0`。
- `go -C desktop build -mod=readonly -o "$OUT/pi-switch-mygo-spike" .`（开发机 Go 1.27.2）：exit `0`。
- Proxy start/status/health：成功；GUI 关闭后 status/health 仍成功；`proxy stop` 成功。
- Proxy 停止后 `proxy status` 返回 `1`、curl health 返回 `7`，这是预期的 stopped/connection-refused 结果；端口检查确认空闲。
- GUI 存活、窗口关闭后退出、重新打开存活均已检查；隔离配置、模型文件和请求记录只在本 run 内读写。JSON Editor 初次合成输入曾产生 invalid JSON；定位为 xdotool 命令链把延迟参数附加进文本，重载安全 draft 后分开执行输入事件，再通过 UI 校验并发布成功。

### 菜单回退与清理证据

证据来源为同一隔离 run 的既有 Ubuntu-test 执行结果：

```text
/home/gui-test/pi-switch-gui/sandbox/runs/w26-ubuntu-20261014/screens-menu-f10.png
/home/gui-test/pi-switch-gui/sandbox/runs/w26-ubuntu-20261014/screens-menu-show-action.png
/home/gui-test/pi-switch-gui/sandbox/runs/w26-ubuntu-20261014/screens-menu-quit-selected.png
```

本地副本已只读核验并查看：

```text
/tmp/pi-switch-w26-menu-f10.png
/tmp/pi-switch-w26-menu-show.png
/tmp/pi-switch-w26-menu-quit-selected.png
```

三个本地文件均为 `1600x1000` PNG。`screens-menu-f10.png` 可见 File 菜单及五个菜单项；远端 `screens-menu-show-action.png`（本地副本为 `/tmp/pi-switch-w26-menu-show.png`）记录 Show action 后 GUI 仍可见；`screens-menu-quit-selected.png` 记录 Quit 被选中。

菜单测试与清理结果：

- F10 软件菜单回退交互：**PASS**。
- 合成 Alt+F10：未展开菜单，**UNVERIFIED**，不宣称快捷键通过。
- Show action 后 GUI 存活：**PASS**。
- Quit action 后隔离 GUI PID `14255` 退出：**PASS**。
- 隔离 Proxy 最终为 stopped，`43145` 监听已释放：**PASS**。
- 未停止、接管或修改任何原有服务。

### Provider 添加补测

先核对既有证据：隔离 `config.json` 的 profile key 查询结果原先仅为 `w26-test-provider`；既有 `screens-provider-editor.png`、`screens-provider-saved.png`、`screens-providers.png` 和重载截图只能证明编辑/保存/重载，不能证明实际 Add。

此前将 `xdotool search --pid <Wayland GUI PID>` 当作输入路径探针是错误的：纯 Wayland 客户端本来没有对应的 X11 child window。按此前已验证的方法改为：

1. 在 `DISPLAY=:100`、既定 `XAUTHORITY` 下定位外层 Niri X11 window：`xdotool search --name niri` 得到 `6291458`；`xdotool getactivewindow` 名称为 `niri`，PID `11017`。
2. 启动同一隔离 run 的 Native GUI，Niri `msg windows` 得到 Wayland app window ID `9`、PID `15144`。
3. 先 `xdotool windowactivate --sync 6291458`，再 `niri msg action focus-window --id 9`；使用外层 Niri 相对坐标和 `xdotool mousemove/click/key` 发送鼠标/键盘事件。该路径成功进入 Provider Add 表单并产生截图反馈。输入是 Xvfb/Niri 合成输入，不代表实体键盘。

实际 Add：

- 点击 `Add provider` 后进入新 Provider draft。
- 输入名称 `w26-added-provider`。Fcitx5 未停止；本次使用逐键 ASCII key events 和延迟，避免候选转换/事件丢失。
- 输入 dummy API key `w26-dummy-key`；未执行 Test draft connection，未触真实 Provider。
- 点击 `Save provider…`，确认 `Save`；GUI 显示保存结果。
- 保存后截图显示 Provider 列表包含 `w26-added-provider`，并显示 `Provider saved with a pre-write backup. Gateway was not published.`
- 隔离 config 的 profile key 只读核对结果为：`w26-added-provider,w26-test-provider`。未读取或回显凭据值。

保存后重载：

- 关闭本次 GUI PID `15144`，确认该 PID 已退出。
- 使用同一隔离 run 重新启动 GUI，PID `18642`，Niri window ID `10`。
- 通过同一外层 Niri 输入路径进入 Providers 页面；重载截图再次显示 `w26-added-provider` 和 `w26-test-provider`。
- 最终关闭本次重启 GUI PID `18642`；隔离 Proxy 保持 stopped，`43145` 无监听。原有 PID `11981` GUI、Fcitx5、Niri 和其他 Proxy 未停止或接管。

证据截图：

```text
/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/home/Pictures/Screenshots/Screenshot from 2026-10-14 19-25-34.png
/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/home/Pictures/Screenshots/Screenshot from 2026-10-14 19-26-04.png
/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/home/Pictures/Screenshots/Screenshot from 2026-10-14 19-27-17.png
/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/home/Pictures/Screenshots/Screenshot from 2026-10-14 19-28-28.png
```

本地只读查看的关键副本：

```text
/tmp/pi-switch-w26-provider-add-fields-exact.png
/tmp/pi-switch-w26-provider-add-confirm.png
/tmp/pi-switch-w26-provider-add-saved.png
/tmp/pi-switch-w26-provider-add-reloaded.png
```

其中：

- `fields-exact` 显示 `w26-added-provider` 和掩码后的 dummy key；
- `confirm` 显示 `Save provider?` 确认对话框；
- `saved` 显示保存成功消息及新 Provider 列表项；
- `reloaded` 显示重启 GUI 后新 Provider 仍在列表中。

补测相关退出码：SSH keyscan/指纹核对/严格 SSH 连接 `0`；外层 Niri 搜索 `0`；外层 `windowactivate` `0`；Niri `focus-window` `0`；Add 表单、字段输入、Save 和确认点击命令均 `0`；Niri 截图动作 `0`；保存后 config profile-key 查询 `0` 且 `added_present=true`；重启 GUI 发现 window ID `10` `0`；重载截图命令 `0`；关闭 PID `15144`/`18642` 后各自存活检查为 `1`（目标已退出）；`43145` 监听检查为 `1`（保持 stopped）。

因此此前“没有输入注入工具所以无法测试”的结论已纠正。`xdotool search --pid` 的 `1` 只表示纯 Wayland GUI 没有 X11 child window，不表示外层 Xvfb→Niri 输入路径不可用。

### Alt+F10 只读定位

- `desktop/shell.go:92-98` 构造 File 菜单和 `Show pi-switch`，该菜单项没有设置 `Accelerator`；`desktop/shell.go:138` 只有 tray 不可用时的提示文本。
- `desktop/shell_test.go:54-75` 只验证菜单项/角色存在，不验证 Alt+F10。
- 已安装 `github.com/egoist/mygo v0.3.3` 的 `menu.go:73-77` 定义显式 `Accelerator` 字段；`prepareItem` 使用角色默认快捷键，而 `RoleAppMenu` 的默认 accelerator 为空（`menu.go:318-323`, `605-606`）。
- Niri 配置只读检查未发现 `F10` 或 `Alt+F10` binding；现有 Alt binding 是 `Super+Alt+L`、`Super+Alt+S`、`Alt+Print` 等。

现有证据支持“应用源码没有为 Show 显式声明 Alt+F10”；但不足以断定是 compositor 截获、GTK 默认行为差异或合成输入路径限制。因此 Alt+F10 继续保持 **UNVERIFIED**，与 F10 成功展开 File 菜单明确区分。

关键截图保存在远端隔离 run：

```text
screens-native-start.png
screens-provider-reopen-proof.png
screens-gateway-published.png
screens-json-validated2.png
screens-json-publish.png
screens-stats-sessions.png
screens-stats-bottom.png
screens-ime-candidate.png
screens-ime-committed.png
screens-reopened-final.png
screens-menu-f10.png
screens-menu-show-action.png
screens-menu-quit-selected.png
```

## 本次文档收尾检查

本次 amend 仅更新本报告；补测未改源代码、系统或真实用户数据。按 W26 目标，隔离 run 仅修改了 `config.json` 及其 pre-write backup，并产生 GUI 日志/PID/截图；隔离 SQLite、models、Agent 会话和原有服务未被修改。未重复执行此前已通过且未受文档修改影响的 Go/Desktop 测试。

- `bash scripts/check-secrets.sh`：exit `0`。
- `git diff --check`：exit `0`。
- `git diff --cached --check`：exit `0`。

## 最终状态/后续

Ubuntu-test 上的 Wayland 软件交互、File 菜单回退、Provider 通过 GUI 添加/保存/重载、隔离 Proxy 生命周期已通过；Provider Add 证据明确来自外层 Xvfb→Niri→Wayland 合成输入路径，不冒充实体键盘。总体仍为 **BLOCKED**，因为 Noctalia/系统托盘、真实物理 HiDPI/键盘/seat/GPU 在此容器不可运行。合成输入、F10 软件菜单和 1600×1000 嵌套会话不能替代物理门禁；Alt+F10 未展开，保持 UNVERIFIED。没有停止或接管原有 W19 GUI/Fcitx 进程，没有安装软件或更改系统级工具链，没有 Push。
