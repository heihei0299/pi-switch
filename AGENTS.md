# GUI 测试环境信息

核验日期：2026-10-11

- Incus 容器名：`ubuntu-test`（当前运行中）
- 固定 IP：`10.10.10.244`（`incusbr0` 上由 Incus DHCP 静态预留，接口 `eth0`）
- 实际系统：Ubuntu 24.04.4 LTS，x86_64；容器名虽曾为 `arch-test`，镜像并非 Arch。
- 测试环境已部署并运行：用户 `gui-test` 下的 Xvfb `:99`（1600×1000×24）、Openbox 和 llvmpipe；MyGo GUI 当前以 `--native` 运行。
- 当前 GUI 构建：`heihei0299/pi-switch` 的 `mygo` 分支，Desktop Actions Run `38119611724`，Commit `5041101b406a82b006512adb6948e69c06924b5b`，Artifact `desktop-test-linux-amd64`。
- 已通过基础 GUI Smoke：进程存活、窗口可见、Tab 输入成功并完成截图。运行中的应用：`/home/gui-test/pi-switch-gui/sandbox/home/.local/pi-switch-mygo-spike.app/pi-switch-mygo-spike`；截图：`/home/gui-test/pi-switch-gui/sandbox/screenshots/native-start.png`。

## SSH 接入核验

核验日期：2026-10-11

- SSH 登录：`gui-test@10.10.10.244:22`；已从开发环境成功认证。
- 已向用户确认并核对远端 ED25519 主机指纹：`SHA256:GGay+nN2Gs9Q2aKCWQrfCGy0PsZY/4bi1nagWaro7nw`。不要关闭 SSH 主机密钥校验；认证凭据不记录在本文件。
- SSH 内部主机名回报为 `pi-switch-web`；Incus 容器名仍为 `ubuntu-test`。
- 非交互 SSH shell 未设置 `DISPLAY`；图形会话使用 `:99`。远程图形工具需显式设置 `DISPLAY=:99` 和 `XAUTHORITY=/home/gui-test/.local/state/pi-switch-gui/Xauthority`（只引用该文件，不读取 cookie）。只读核验确认 `gui-test` 下 Xvfb、Openbox 和 `pi-switch-mygo-spike` 进程正在运行。

## 当前隔离复测实例

核验日期：2026-10-11

- 上述 Desktop Actions 构建和 `native-start.png` 是预装基线；另启动了当前未提交工作树的独立 `--native` 测试构建，commit `ae3b9ef934a6fc6cf8909c254f66b2fe7c13415d`（dirty），二进制 SHA256：`e356cc9f2e9b0b19fb290107728a37b9ef1ddc0a1b9c1def3f03d5819ecb3e83`。
- 隔离 run 目录：`/home/gui-test/pi-switch-gui/sandbox/runs/plan-ae3b9ef-20261011083843`；应用配置、数据库、模型、Agent 会话、导出和截图均在此 run 或其对应子目录，不使用真实用户数据。
- 最新核验时预装基线 PID `273` 仍在运行；当前工作树 GUI PID `7508` 正在运行，使用同一隔离 XDG runtime 的第二次启动已退出；此前 PID `7156` 通过窗口关闭退出。受控 Proxy PID `7292` 已停止；PID 均为当次观察值。
- 当前工作树 CLI 已放入隔离 run 的 `bin/pi-switch-worktree`，SHA256：`3b8ea5dddf6d28154c7ba210208c184cf4cad3fcdffb14c2db6a5ba87666f6b0`；GUI 通过 `PI_SWITCH_CLI_PATH` 指向该文件完成了复测。
- 当前隔离 config 仅有假 profile `supplier`（current=`supplier`），`settings.web.port=43110`、`settings.proxy.port=49093`；无 `proxy.pid`，端口 `49093` 已空闲。备份仍在该 run 的 `config/config.json.backups/`，权限 `0600`。
- `127.0.0.1:43112` 存在一个无法核验身份的监听进程；测试确认 Native 拒绝接管后，监听仍保持原状。不要停止或接管它。
- 本次 Wayland/IME 验证目标已按用户决定改为 Ubuntu 24.04；Arch 不再是这次测试的目标 OS。Ubuntu 上的 Niri/Wayland 与 Fcitx5/Pinyin 验证结果及硬件限制见下方 2026-10-14 记录。


## Ubuntu 24.04 Niri/Wayland/IME 验证（2026-10-14）

- 目标 OS：Ubuntu 24.04；该决定仅调整本次隔离 GUI 测试目标，不代表产品支持矩阵变化，也不构成 Arch 验证。
- 复用容器 `ubuntu-test` 和已有隔离 sandbox，未重装或重置容器；Niri `26.04` 安装于 `/usr/local/bin/niri`，配置在 `/home/gui-test/pi-switch-gui/sandbox/runs/desktop-env-20261014/session/xdg/config/niri/config.kdl`。
- Niri 运行于既有 Xvfb `:100` 上的嵌套 Wayland 会话（Wayland socket `wayland-1`），使用 llvmpipe；MyGo Native 进程通过 `GDK_BACKEND=wayland` 连接。运行和 IME 数据位于 `/home/gui-test/pi-switch-gui/sandbox/runs/wayland-e71dffc-20261014`。
- Niri 配置校验通过；Fcitx5 当前输入法为 `pinyin`。通过 GUI 搜索框输入 `nihao`，候选窗出现并提交为“你好”。截图位于隔离 run 的 `screens-wayland-ime-candidate.png` 与 `screens-wayland-ime-committed.png`。
- 容器没有 `/dev/dri`、`/dev/input`、`/dev/tty0`、`/dev/seat0`；因此 GPU 加速、实体键盘、真实屏幕缩放/高 DPI、物理 seat 与实体桌面会话均未验证。合成输入仅能证明该软件 IME/Wayland 路径。

## Agent skills

### Issue tracker

Issues are tracked as local markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Use the single-context layout. See `docs/agents/domain.md`.
