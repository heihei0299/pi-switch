# W26 Native GUI 环境验收（当前 Ubuntu 测试机复测与历史记录）

## 最新证据 — 当前源码在已有 Ubuntu 测试机隔离复测

- 日期：本地 **2026-10-14 PDT**；远端 UTC 为 **2026-10-15**。本节记录本轮实际远端操作、磁盘断言与已查看的截图；旧证据独立归档。
- 当前目标：`gui-test@10.10.10.244`，**Ubuntu 24.04.4 LTS**，不是 Arch。本轮源码来自 `/home/shial/Project/pi-switch` 的 `main`，提交 `d784ea402cc5490541ac155a45d9942be65f51eb`，保留原有用户修改。
- **总状态：BLOCKED**。本轮 Native GUI、IME、Provider、Gateway、JSON、Stats/Sessions、菜单及 GUI/Proxy 生命周期的软件路径已实际通过；真实 `3200×2000 / scale 2`、物理显示/键盘/GPU/seat 和 Noctalia/托盘门禁缺少环境。另有 **Alt+F10 合成输入路径 FAIL**，不得被总体 BLOCKED 掩盖。
- 下方旧 `64c469b`、旧 worktree 与开发容器尝试均为独立历史记录；不将历史 PASS 当成本轮证据，也不将开发容器无 Wayland 推断为远端不可测。

### 本轮环境、构建身份与隔离

| 检查 | 实际结果与边界 |
|---|---|
| SSH 主机身份 | 首次严格校验 exit `255`，未执行远程命令。随后 `ssh-keyscan` 写入临时 known_hosts，核对 ED25519 指纹与用户确认值 `SHA256:GGay+nN2Gs9Q2aKCWQrfCGy0PsZY/4bi1nagWaro7nw` 完全一致；正式 SSH/scp 均 exit `0`，未关闭主机校验 |
| Niri/Wayland | Niri `26.04`，PID `11017`，嵌套运行于 `Xvfb :100`；实际 winit 输出 `1280×800`、scale `1`、`physical_size=null`。Wayland socket 为 `/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/runtime/wayland-1` |
| Native 客户端 | `GDK_BACKEND=wayland`，GTK4/MyGo Native；当前入口直接使用 `mygo.App.Run()`，拒绝 `--web`。本轮执行文件为新 Native 二进制，运行期间 `/proc/24316/maps` 未见 webkit/webview；没有用浏览器/WebView 代替 Native |
| 物理设备与桌面组件 | 缺少 `/dev/dri`、`/dev/input`、`/dev/tty0`、`/dev/seat0`；未安装 `noctalia-shell`、`noctalia`、`qs`。在隔离运行资源中补齐仓库 icon 后，tray 实际错误为缺少 `libayatana-appindicator3`，不再将 icon 缺失当作最终原因 |
| 构建工具链 | 开发机 Go `1.27.2-X:nodwarf5`，`go -C desktop build` 新编译 exit `0`；远端 Go 为 `1.24.2`，未安装或修改远端工具链 |
| 新隔离 run | `/home/gui-test/pi-switch-gui/sandbox/runs/w26-current.2eUIwP`；独立 HOME、XDG config/data、config、agent/sessions、models.json、bin、logs；独立 `TMPDIR=/tmp/pi-w26.*`，使单实例锁不触碰旧 app |
| 会话路由与数据安全 | 保留共享 Wayland/DBus/`XDG_RUNTIME_DIR`，只提取所需会话路由变量，不复制真实用户数据；unset `PI_AGENT_SETTINGS`、`PI_CODING_AGENT_DIR`。凭证全为 dummy，上游仅 `127.0.0.1:9` 且从未调用；config/DB 权限 `0600` |
| 本轮测试数据 | 新 DB 中仅 1 条明确标记的合成请求；初始 profile 为 `w26-seed`。本轮交互、models 和数据库断言均针对这个新 run，不沿用历史测试数据 |

构建产物身份：

- Native SHA256：`6a76948657e6e77d83a6095f247f491b027428425849da55e20ccefd3e731da3`。
- GUI `go version -m`：`vcs.revision=d784ea4`（完整提交见上）、`vcs.modified=true`。
- CLI SHA256：`37541d1d163b4f613b23c658c550c340325fefe66d5e25934175aac58f133a17`。
- CLI `--build-info`：`commit=d784ea4`、`dirty=true`、`version=20261011.0.0`。dirty 来自保留的原有用户修改，不能宣称为 clean commit 构建。

### 本轮逐项实测矩阵

下列 PASS **仅限本轮嵌套 Wayland / 合成输入的软件路径**；截图为 `1280×800`，不证明实体键盘、物理 DPI 或硬件显示。

| 验收项 | 本轮状态 | 实际证据/限制 |
|---|---|---|
| Native 窗口启动、无白屏 | PASS（软件路径） | 新 GUI PID `19298`、Niri window `11` 出现；max 列动画结束后截图稳定、可见内容 |
| Fcitx5 中文候选/提交 | PASS（软件路径） | `fcitx5-remote -n` 返回 `pinyin`；通过 xdotool 在 Providers 搜索框输入 `nihao`，候选出现，Space 提交为“你好”；候选及提交截图均已查看 |
| GUI Provider Add/保存 | PASS（软件路径） | 实际点击 Add，填写 `w26-current-added` 和 dummy password，Save 确认；磁盘断言新 key 存在且 dummy 凭证保留 |
| GUI Provider Edit/重命名/保存 | PASS（软件路径） | 实际 Edit 改为 `w26-current-edited` 并确认 Save；断言旧名消失、新名存在，masked 凭证保存后仍保留原 dummy 值 |
| Provider 重启后重载 | PASS（软件路径） | 重启同配置 GUI PID `24316`、window `12`；列表同时显示 `w26-current-edited` 和 `w26-seed`，截图已核验 |
| Gateway 发布 | PASS（软件路径） | 选择 seed `gpt-4o-mini`，Publish 确认；隔离 `models.json` 中含 `pi-switch-chat` 模型 |
| JSON Editor 加载/编辑/校验/发布 | PASS（软件路径） | Load safe draft 后通过 GUI TextArea 将 `proxy=false` 改为 `true`；Validate 显示 Valid / 1 pending change，Publish 确认，磁盘断言 `proxy=true` |
| Stats/Sessions | PASS（软件路径） | Stats 显示 1 条成功请求、200 tokens、42 ms；Sessions 显示 `W26 current synthetic GUI validation`、1 request；均为本轮合成数据并有截图 |
| 窗口 X 关闭及重开 | PASS（软件路径） | 实际点击窗口 X 后 PID `19298` 退出；随后以同隔离配置重开 PID `24316`、window `12` |
| File 菜单/F10/Show/Quit | PASS（软件路径） | F10 实际展开 File；Show 后 GUI 仍存活；最后选择 File > Quit 后 PID `24316` 退出 |
| Alt+F10 合成输入路径 | **FAIL** | 应用提示 Alt+F10，但本轮发送该组合后未展开菜单，证据 `screens-alt-f10.png`；F10 则成功。既有定位显示 `shell.go` / `setTrayError` 提示 Alt+F10、但未设置对应 Accelerator；根因及实体快捷键结果仍 **UNVERIFIED**，本工单仅报告、不改业务代码 |
| 独立 Proxy start/status/health | PASS | 本轮隔离 Proxy PID `19282`、端口 `43145`；start/status/health 成功 |
| GUI 退出后 Proxy 存活 | PASS（本轮实际 GUI 生命周期） | 窗口 X 关闭 PID `19298` 后，以及 File > Quit 退出 PID `24316` 后，Proxy `19282` 均仍存活，CLI status exit `0`、health exit `0`；不是仅凭独立 CLI start 推断 |
| 仅清理本轮 Proxy | PASS | 最后 `proxy stop` exit `0`；停止后 status exit `1`、curl exit `7` 为预期，`ss` 确认 `43145` 无监听、`proxy.pid` 不存在 |
| 不干扰旧实例 | PASS（观察范围） | 只停止新隔离 Proxy `19282`；原 Niri `11017`、Fcitx `11916`、GUI `11981` 仍存活，旧 window `3` 仍在 |
| 真实 3200×2000 / scale 2 | **BLOCKED** | 当前实际输出仅 1280×800 / scale 1、无物理尺寸；不得使用嵌套会话或截图冒充硬件 HiDPI 通过 |
| 物理 GPU/显示/键盘/seat | **BLOCKED** | 所需设备节点缺失；软件 IME/合成输入不能替代实体设备验收 |
| Noctalia / 系统托盘 | **BLOCKED** | Noctalia/qs 未安装，且当前 Native tray 缺 `libayatana-appindicator3`；没有真实托盘交互通过证据 |

### 本轮截图与日志

以下截图均位于新远端 run `/home/gui-test/pi-switch-gui/sandbox/runs/w26-current.2eUIwP/`，均已实际查看；不能与旧 run 截图混用：

```text
screens-provider-added-list.png
screens-provider-edited.png
screens-provider-reloaded.png
screens-gateway-published.png
screens-json-loaded.png
screens-json-edited.png
screens-json-validated.png
screens-json-publish-dialog.png
screens-json-published.png
screens-stats-first.png
screens-stats-sessions.png
screens-ime-candidate.png
screens-ime-committed.png
screens-menu-f10.png
screens-menu-reopen-f10.png
screens-menu-quit-selected.png
screens-reopen-overview.png
screens-alt-f10.png
```

本地截图副本：`/tmp/pi-switch-v2-acceptance.HfeKa3/w26-remote-*.png`。同目录保留新增环境日志 `w26-remote-environment.log` 以及 setup、ime、provider-add、provider-edit、gateway、json、close-reopen、reload、final-lifecycle 各阶段日志；末次 git status/diff 命令 exit `0`。本次仅更新本报告，没有修改产品代码或覆盖既有测试日志。


### 本轮命令退出状态与公共验收边界

| 实际命令/检查 | 退出状态 |
|---|---|
| `go -C desktop build -mod=readonly -o /tmp/pi-switch-v2-acceptance.HfeKa3/pi-switch-native-current .` | `0`，本轮重新构建 |
| `go version -m`、本地/远端 SHA256 比对、远端 `ldd`、CLI `--build-info` | `0` |
| 严格主机身份验证后的 SSH/scp、Niri focus/screenshot、xdotool 交互命令 | `0`；命令成功不等于功能成功，Alt+F10 的实际失败单列于矩阵 |
| 新隔离数据创建、Provider 添加/编辑/重载、JSON 发布及 SQLite 内容/权限的 Python 断言 | `0` |
| `proxy start --daemon --host 127.0.0.1 --port 43145`、运行时 `proxy status`、`curl -fsS .../healthz`、`proxy stop` | `0`，GUI 两次正常退出后的 status/health 也均为 `0` |
| 停止后的 `proxy status` / health curl | `1` / `7`，预期结果；后续断言端口释放及 PID 文件不存在为 `0` |
| `bash scripts/check-secrets.sh`、`git diff --check` | `0` |

此前最终公共验收命令记录在 `/tmp/pi-switch-v2-acceptance.HfeKa3/final.log`：根模块及 Desktop test/vet、架构检查、MyGo vet、构建、功能回归、Node 测试、七个关键包的 race 命令均 exit `0`；架构输出为 `architecture dependencies: clean (2 modules)`。本次仅改 W26 报告，不重复未受影响的 Go 全量检查。根测试仍有 16 个既有 legacy retry/failover/supplier SKIP，保持 **UNVERIFIED**，不是 PASS；不将这些已有覆盖空缺归因于本轮 GUI 操作。

## 历史 A — 旧 Ubuntu-test follow-up（64c469b / 旧 worktree）

以下保留原有记录；其中 PASS、PID、输出尺寸、哈希和“最终状态”均只适用于当次旧 run，不是上方本轮结果。

- 日期：2026-10-14
- 工作树：`/home/shial/Project/pi-switch-mygo-v2-20261014094522`
- 分支：`codex/mygo-v2-20261014094522`
- GUI/CLI build identity：`64c469b`（测试时 HEAD；随后仅更新本报告，未改 Go/Desktop 源码）
- 实际目标机：`gui-test@10.10.10.244`，Ubuntu 24.04.4 LTS
- 总状态：**BLOCKED**（Ubuntu/Niri/Wayland 软件 GUI 子集已通过；Noctalia、HiDPI 与物理设备门禁不可验证）
- 本次目标 OS：Ubuntu 24.04；Arch 环境不是本次必达项。

### 结论

按用户后续指示，改在 `ubuntu-test` 主机执行隔离 Native GUI 验收。该主机运行 Ubuntu 24.04.4、Niri 26.04、Wayland socket `wayland-1` 和 Fcitx5 5.1.7；MyGo 以 `GDK_BACKEND=wayland` 原生 Wayland 客户端启动。中文拼音候选及提交、Provider 添加/保存/重载、Gateway 发布、JSON Gateway draft 修改/校验/发布、Stats/Sessions 隔离数据展示、窗口关闭重开以及 GUI 退出后 Proxy 继续运行均有实际交互证据。

此会话是运行在既有 Xvfb `:100`（1600×1000×24）上的嵌套 Niri/Wayland，会使用 llvmpipe。它证明 Wayland 客户端、IME 软件路径和 UI 交互，但不是实体显示器/实体键盘/硬件加速验收。Noctalia 未安装，桌面没有 `/dev/dri` 或 `/dev/input`，3200×2000/scale 2 与物理输入/显示仍未验证；所以不把整个 W26 标为 PASS，也不把这个软件会话冒充物理实机结果。

此前首次尝试的本地 `arch-lxc` shell 没有 Wayland 环境，已记录为 BLOCKED。该尝试未启动 GUI/Proxy；本报告现以用户要求的 ubuntu-test follow-up 结果补充并更新状态。

### 环境与隔离

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

### 逐项验收

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

### 命令结果与证据

- SSH 首次使用默认 known_hosts 失败（exit `255`）；没有关闭主机密钥校验。用 `ssh-keyscan` 取 ED25519 公钥并核对已确认指纹后，以临时 known_hosts + `StrictHostKeyChecking=yes` 重连成功。
- 主机检查：`/etc/os-release`、`/usr/local/go/bin/go version`、`/usr/local/bin/niri --version`、Fcitx5 查询均成功；Noctalia 命令检查为未安装。
- `bash scripts/build-go.sh "$OUT/pi-switch"`：exit `0`。
- `go -C desktop build -mod=readonly -o "$OUT/pi-switch-mygo-spike" .`（开发机 Go 1.27.2）：exit `0`。
- Proxy start/status/health：成功；GUI 关闭后 status/health 仍成功；`proxy stop` 成功。
- Proxy 停止后 `proxy status` 返回 `1`、curl health 返回 `7`，这是预期的 stopped/connection-refused 结果；端口检查确认空闲。
- GUI 存活、窗口关闭后退出、重新打开存活均已检查；隔离配置、模型文件和请求记录只在本 run 内读写。JSON Editor 初次合成输入曾产生 invalid JSON；定位为 xdotool 命令链把延迟参数附加进文本，重载安全 draft 后分开执行输入事件，再通过 UI 校验并发布成功。

#### 菜单回退与清理证据

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

#### Provider 添加补测

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

#### Alt+F10 只读定位

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

### 本次文档收尾检查

本次 amend 仅更新本报告；补测未改源代码、系统或真实用户数据。按 W26 目标，隔离 run 仅修改了 `config.json` 及其 pre-write backup，并产生 GUI 日志/PID/截图；隔离 SQLite、models、Agent 会话和原有服务未被修改。未重复执行此前已通过且未受文档修改影响的 Go/Desktop 测试。

- `bash scripts/check-secrets.sh`：exit `0`。
- `git diff --check`：exit `0`。
- `git diff --cached --check`：exit `0`。

### 最终状态/后续

Ubuntu-test 上的 Wayland 软件交互、File 菜单回退、Provider 通过 GUI 添加/保存/重载、隔离 Proxy 生命周期已通过；Provider Add 证据明确来自外层 Xvfb→Niri→Wayland 合成输入路径，不冒充实体键盘。总体仍为 **BLOCKED**，因为 Noctalia/系统托盘、真实物理 HiDPI/键盘/seat/GPU 在此容器不可运行。合成输入、F10 软件菜单和 1600×1000 嵌套会话不能替代物理门禁；Alt+F10 未展开，保持 UNVERIFIED。没有停止或接管原有 W19 GUI/Fcitx 进程，没有安装软件或更改系统级工具链，没有 Push。

## 历史 B — 开发容器本地尝试（2026-10-14，bd4e69f）

This section records only the earlier local development-container attempt at HEAD `bd4e69f`. Its missing Wayland session blocked GUI actions in that container, not on the existing remote Ubuntu test machine. The latest remote results are recorded at the top of this report; neither the local block nor the older Ubuntu PASS results substitute for them.

- **Branch:** `main` (HEAD `bd4e69f`); no branch refs were changed, recreated, deleted, reset, or pushed.
- **Local-attempt status:** **BLOCKED** for every GUI interaction in this development container. No real Wayland session or Wayland socket is present, so no GUI action is labeled PASS and no Xvfb, nested Wayland, browser, or simulated GUI substitute was used.
- **Exact environment:** `uname -a` reported `Linux arch-lxc 7.2.9-zen1-1-zen #1 ZEN SMP PREEMPT_DYNAMIC Sat, 03 Oct 2026 11:43:26 +0000 x86_64 GNU/Linux`; `XDG_SESSION_TYPE=`; `WAYLAND_DISPLAY=`; `XDG_CURRENT_DESKTOP=`; `go version go1.27.2-X:nodwarf5 linux/amd64`.
- **OS:** `/etc/os-release` reports Arch Linux rolling (`BUILD_ID=rolling`, `IMAGE_VERSION=2026.08.01`).
- **Session/socket/seat evidence:** `loginctl` shows only the `shial` manager session with `Type=unspecified`, `Class=manager`, and no seat/display; the runtime directory contains only `bus`, with no `wayland-*` socket. Named-process inspection found no `niri`, `fcitx5`, `Noctalia`, Xwayland, or other compositor process. `/dev/dri`, `/dev/input`, and `/run/seatd.sock` are absent.
- **Noctalia/Fcitx5:** command checks for `noctalia`, `noctalia-shell`, `fcitx5`, and `fcitx5-remote` all returned not found; these GUI/IME checks are **BLOCKED**, not PASS.
- **Native entrypoint:** current source and a current `desktop` build use `mygo.App.Run()` directly; `--web` is rejected as unsupported and the source contains no WebView fallback. The current Native binary was built outside the repository at `/tmp/pi-switch-v2-acceptance.HfeKa3/pi-switch-native`.
- **Local-only GUI matrix:** Native window launch/visible/no-crash, 3200x2000 scale 2, Fcitx5 composition/candidate/commit, Provider edit/save/reload, GUI Provider add/save/reload, Gateway publish, JSON editor validate/publish, Stats/Sessions display, File menu/Show/Quit, Alt+F10, Noctalia/tray, window close/reopen, and GUI-exit Proxy survival were each **BLOCKED** for that local attempt because the development container had no real GUI session. This does not describe the later remote test run.
- **Isolated CLI/Proxy:** temporary HOME/XDG/config/data paths were used; inherited Wayland/DBus/runtime variables were not replaced, agent variables were unset, and no real user config or credentials were read. The current CLI was built outside the repository. Port `127.0.0.1:43145` was free, so it was selected. Isolated `proxy start --daemon`, `proxy status`, `GET /healthz`, owned `proxy stop`, stopped status, and post-stop port-release check completed; the stopped status exit `1` and refused health connection are expected. Only the owned isolated Proxy was stopped.
- **Evidence logs:** `/tmp/pi-switch-v2-acceptance.HfeKa3/w26-environment.log` records the ArchLXC environment, absent GUI dependencies/socket/seat, and the resulting GUI block. `/tmp/pi-switch-v2-acceptance.HfeKa3/w26.log` records the isolated Proxy start/status/health/stop: no isolated `proxy.pid` remains, `43145` is free, post-stop curl exit `7` is the expected refusal, and unknown `43112` remained listening and untouched.
- **Public A-D:** `/tmp/pi-switch-v2-acceptance.HfeKa3/w26-public-corrected.log` records all four readonly/count-1 public commands, each with exit `0`; the earlier isolated-HOME Go cache lookup failures are preserved in `/tmp/pi-switch-v2-acceptance.HfeKa3/w26-isolated-cache-failures.log` and were diagnosed as environment/cache availability, not code failures. No code fix was made.
- **Inherited coverage boundary:** the reconstructed `/tmp/pi-switch-v2-acceptance.HfeKa3/w24.log` and fresh W24 rerun completed with exit `0`; its 16 inherited legacy retry/failover/supplier `SKIP` cases remain **UNVERIFIED**, never PASS. The mandated `LegacyLog_` suite was 9/9 PASS. These are not expanded into W26 GUI or business/test repair claims.
