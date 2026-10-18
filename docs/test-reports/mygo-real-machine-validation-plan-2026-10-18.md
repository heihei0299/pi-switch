# MyGo Native 宿主机真机验证方案

- 编写日期：2026-10-18。
- 文档类型：测试计划，不是已执行的验收报告。
- 当前状态：**NOT_RUN（待执行）**。下列用例的预期结果不是实测结论，不能据此标记 PASS。
- 目标分支：`mygo`；编写时 HEAD：`845a956e671193adf1d02f8d0ffbdff04714aaf1`，工作树包含未提交及未跟踪代码。
- 主验收环境：当前宿主机 Arch Linux、原生 Niri/Wayland 桌面、Fcitx5/Rime、物理键盘及实体显示器；不再以 Ubuntu 容器作为本轮测试环境。
- 边界：本轮方案目标已由 Ubuntu 容器切换为宿主机，但尚未完成 Arch 真机验收，不改变产品支持范围；Xvfb、嵌套 Wayland、远程合成输入不能替代真机结论。
- 执行授权：本次只调整测试方案，不执行测试。须在用户明确要求开始测试后，才按本方案启动应用、服务或进行桌面交互。

## 1. 目标与已有证据边界

### 宿主机环境基线

下表仅记录 2026-10-18 的只读环境核对，不是测试结果，也不代表任何用例已通过。执行前重新核对实际条件，不能将这些观察作为完整硬件或输入验收证据。

| 项目 | 方案采用的宿主机条件 |
|---|---|
| 系统与架构 | Arch Linux，x86_64 |
| 内核 | `7.2.9-zen1-1-zen` |
| 桌面会话 | Niri `26.04 (8ed0da4)` / Wayland，`Remote=no`、`Seat=seat0` |
| 实体输出 | `eDP-1`，3200×2000，当前刷新率 165 Hz，缩放 2，逻辑尺寸 1600×1000 |
| 图形设备 | Intel Arrow Lake-P / Arc Pro 130T/140T；应用实际渲染后端仍待验证 |
| 设备入口 | 存在 `/dev/dri/card1`、`renderD128`、`/dev/input` 和 `/dev/tty0`；无 `/dev/seat0` 节点不等于没有物理 seat |
| 输入法 | Fcitx5 当前输入法为 `rime`；本轮按现有 Rime 测试，不擅自切换为 Pinyin |
| 图形运行库 | 已发现 GTK4、EGL/GL、libayatana-appindicator3；库存在不代表托盘或 GPU 验收通过 |
| 会话与 locale | `XDG_SESSION_TYPE=wayland`、`WAYLAND_DISPLAY=wayland-1`、UTF-8 |

原 Ubuntu 容器及其截图、合成输入和软件渲染结果仅作历史参考，不计入本轮宿主机通过项。Pinyin 若需覆盖，单独安排并获准后测试，不为套用旧环境修改宿主机全局输入法。

本轮补齐真实桌面环境中的输入法、键盘、GPU、缩放、多屏、主题、托盘及 GUI/Proxy 生命周期验证。

根模块与 Desktop 测试、静态检查、构建，以及既有容器 GUI 冒烟，可作为本轮准备依据，但不能转记为本轮真机 PASS。历史环境与软件路径证据见 [W26 验收记录](mygo-rearchitecture-w26-2026-10-14.md)；该记录中的 Alt+F10 合成输入失败和实体高分屏、物理设备、托盘证据缺口，均需分别复测，不能假定已修复。

本计划分别形成三个结论：

1. **功能可用性**：受控测试数据下的业务操作是否正确。
2. **真机桌面验收**：原生会话、实体输入、显示与生命周期门禁是否通过。
3. **发布包验收**：本轮实际包的安装、升级、降级与卸载是否通过。

单项通过不能替代其他结论。

## 2. 固定候选版本

1. 从同一份源码快照构建 GUI、配套 CLI 和测试包；快照必须包含本轮相关的未提交改动及未跟踪源码。
2. 不得仅用 `git archive HEAD` 或旧 Actions 包代表 dirty 工作树。准备构建副本时，只纳入必要源码与资源，不复制 `.env`、凭据、真实配置或其他无关文件。
3. 参考 [Desktop 构建说明](../../desktop/README.md) 和 [Desktop 工作流](../../.github/workflows/desktop.yml)，携带匹配的 CLI、图标及构建身份资源；正式包验收不使用缺资源的裸二进制替代。
4. 构建在一次性源码副本或已有合适的隔离目录执行，不覆盖工作树内既有构建产物。
5. 测试期间冻结候选包；改动代码、资源或依赖后记录新身份，并重验受影响项目与直接回归范围。

执行时填写：

| 字段 | 本轮实际值 |
|---|---|
| 执行日期、测试人 | 待填写 |
| 分支、完整 HEAD、dirty 状态 | 待填写 |
| 源码快照标识及变更清单 | 待填写 |
| GUI / CLI / 安装包 SHA256 | 待填写 |
| CLI `--build-info` | 待填写 |
| GUI 构建身份、目标架构 | 待填写 |
| 宿主机 OS、内核、Niri、MyGo 版本 | 待填写 |
| GPU、驱动、实际应用渲染后端 | 待填写 |
| 实体屏幕分辨率、缩放、多屏布局 | 待填写 |
| Fcitx5/Rime、主题、托盘宿主；额外输入法场景另记 | 待填写 |

## 3. 安全与环境准备

### 3.1 真实会话与数据隔离

- 使用宿主机当前真实桌面会话，默认不创建账号或重新登录。应用配置、数据库、models、Agent Session、缓存、窗口状态及临时文件必须隔离；不连接真实用户数据，不操作已有 pi-switch 实例或身份不明的监听。
- 保留真实会话的 `HOME`、`XDG_RUNTIME_DIR`、`WAYLAND_DISPLAY`、D-Bus 和输入法路由；不启动 Xvfb 或嵌套合成器。MyGo 窗口状态位于用户配置目录，因此应用进程的 `XDG_CONFIG_HOME`、`XDG_DATA_HOME`、`XDG_CACHE_HOME` 单独指向本轮目录，避免写入真实应用状态。隔离后须核对实际主题和输入法表现；不一致时记录限制，不宣称等同原桌面配置。
- 不使用 `env -i` 清空真实桌面环境；只覆盖本轮应用数据路径。启动前确认没有会使应用访问真实数据的额外路径覆盖项，不要打印或保存完整环境变量。
- 使用 UTF-8 locale。记录物理会话、输出设备与实际渲染证据；仅凭会话变量无法证明非嵌套或硬件加速。
- 缺少依赖时记录 BLOCK，不自动安装系统包、修改合成器配置或更换桌面组件。

### 3.2 隔离启动模板

以下为获准开始测试后使用的启动模板，本次不执行。在宿主机当前真实桌面终端中，先将 `GUI_BIN`、`CLI_BIN` 设置为本轮包内实际可执行文件的绝对路径，并准备受控本地 Catalog 地址 `TEST_CATALOG_URL`；代码不会构建或安装软件。

```bash
: "${GUI_BIN:?请先设置 GUI_BIN 为本轮 GUI 的绝对路径}"
: "${CLI_BIN:?请先设置 CLI_BIN 为本轮 CLI 的绝对路径}"
: "${TEST_CATALOG_URL:?请先设置受控本地 Catalog 地址，禁止访问真实服务}"
test -x "$GUI_BIN" && test -x "$CLI_BIN" || exit 1

if [ "${XDG_SESSION_TYPE:-}" != wayland ] || \
   [ -z "${WAYLAND_DISPLAY:-}" ] || \
   [ -z "${XDG_RUNTIME_DIR:-}" ]; then
  printf '%s\n' 'BLOCK：请从真实 Wayland 桌面终端启动。' >&2
  exit 1
fi
case "$(locale charmap)" in
  UTF-8|UTF8) ;;
  *) printf '%s\n' 'BLOCK：当前会话不是 UTF-8 locale。' >&2; exit 1 ;;
esac

umask 077
RUN=$(mktemp -d "$HOME/pi-switch-realtest.XXXXXX") || exit 1
mkdir -p "$RUN"/{config,agent/sessions,logs,evidence,exports,tmp,cache,xdg/config,xdg/data,xdg/cache} || exit 1

export PI_SWITCH_CLI_PATH="$CLI_BIN"
export PI_SWITCH_CONFIG_DIR="$RUN/config"
export PI_SWITCH_CONFIG="$RUN/config/config.json"
export PI_SWITCH_DB="$RUN/config/requests.db"
export PI_SWITCH_MODELS="$RUN/models.json"
export PI_SWITCH_CATALOG="$RUN/cache/models-dev.json"
export PI_SWITCH_CATALOG_URL="$TEST_CATALOG_URL"
export XDG_CONFIG_HOME="$RUN/xdg/config"
export XDG_DATA_HOME="$RUN/xdg/data"
export XDG_CACHE_HOME="$RUN/xdg/cache"
export PI_AGENT_ROOT="$RUN/agent"
export PI_AGENT_SETTINGS="$RUN/agent/settings.json"
export TMPDIR="$RUN/tmp"

printf 'Run directory: %s\n' "$RUN"
printf 'session=%s wayland=%s\n' "$XDG_SESSION_TYPE" "$WAYLAND_DISPLAY" \
  > "$RUN/evidence/session.txt"
"$CLI_BIN" --build-info > "$RUN/evidence/build-info.json" || exit 1
sha256sum "$GUI_BIN" "$CLI_BIN" > "$RUN/evidence/binaries.sha256" || exit 1

GDK_BACKEND=wayland "$GUI_BIN" > "$RUN/logs/gui.log" 2>&1 &
GUI_PID=$!
printf '%s\n' "$GUI_PID" > "$RUN/evidence/gui-initial.pid"
```

只对启动的本轮进程设置上述路径，不修改系统配置。使用独立 `TMPDIR` 隔离本轮临时文件和单实例锁；本轮二次启动必须继承同一组变量。GUI 重启及 CLI 核验均保持相同数据路径；每次 GUI 启动保持 `GDK_BACKEND=wayland`。

模板中的 `PI_AGENT_SETTINGS` 仅用于本轮手工应用验收，不直接沿用于项目单元测试；外部显式路径可能覆盖测试内部设置的 `PI_AGENT_ROOT`。

### 3.3 测试数据与受控服务

- 仅使用假凭据、合成请求、假 Profile、模型、Package 和 Session 哨兵数据。
- 首次执行 Provider test、fetch-models 或推理前，先确认所有测试渠道指向受控的本地假上游；不能直接调用默认配置中的真实服务地址。
- 本地假上游需可记录请求，并能返回模型列表、普通响应、流式响应、错误、超时等受控结果；模型 Catalog 也使用本地假数据，不读取真实缓存或请求默认外网 Catalog。其进程与监听端口单独记录。
- Proxy、管理 API 和假上游均使用本轮确认空闲的 loopback 端口；启动失败时不抢占已有监听。
- 备份、导出和截图保存在本轮目录内；不记录真实 token、私钥、Cookie、Xauthority 内容或原始认证头。

## 4. 第一阶段：核心真机验收

以下项目均为获准执行后的必测模板，目前全部 NOT_RUN；没有环境或证据时标记 BLOCK，不能按不适用直接跳过整个门禁。本次不注入按键、不切换输入法、不调整缩放或主题、不启停 GUI/Proxy，也不执行安装、升级或卸载。

| 编号 | 项目 | 操作与通过标准 | 状态 |
|---|---|---|---|
| RM-01 | 原生启动与渲染 | 在实体 Niri/Wayland 会话启动；移动、缩放窗口并切换所有页面。无黑屏、崩溃、持续卡死；留存原生客户端及实际渲染后端证据 | NOT_RUN |
| RM-02 | 真实中文输入 | 使用宿主机 Fcitx5/Rime 和物理键盘，在搜索、Provider 名称及 JSON 编辑器中组合输入、选词、取消候选、切换中英文、删除和撤销。无丢字、重复提交或组合期间提前保存；Pinyin 补充场景另列 | NOT_RUN |
| RM-03 | 键盘与菜单 | Tab / Shift+Tab 焦点可见且顺序合理；Enter、Esc、复制粘贴和弹窗操作无焦点陷阱。物理按键专项复测 Alt+F10，并记录是否被合成器快捷键截获 | NOT_RUN |
| RM-04 | 缩放与主题 | 在硬件支持的 100%、150%、200% 缩放、明暗主题和最小窗口下检查布局。文本、按钮、错误及禁用状态可辨认，滚动和点击位置正确；不支持的子场景分别记录 BLOCK | NOT_RUN |
| RM-05 | Provider / Profile | 新建、复制、编辑、重命名、切换、删除假数据并重启。持久化正确；取消不写入；脱敏凭据不被意外清空或公开 | NOT_RUN |
| RM-06 | Gateway / JSON | 生成、预览、非法 JSON、取消发布、确认发布。验证和预览不写文件；发布只修改约定目标，结果与预览一致 | NOT_RUN |
| RM-07 | 并发修改 | 页面加载后，用同隔离环境的 CLI 修改相关数据，再保存或发布。旧 revision 被明确拒绝，不静默覆盖新内容 | NOT_RUN |
| RM-08 | 请求与统计 | 对本地假上游执行成功、流式、错误、重试、超时及取消。界面不被请求阻塞；错误、次数、取消状态与 Stats/Sessions、假上游记录相符 | NOT_RUN |
| RM-09 | 备份与恢复 | 创建备份、修改、取消恢复、确认恢复、制造并发变更。恢复正确且先备份当前内容；冲突不覆盖；相关文件权限符合约定；不暗中重启 Proxy | NOT_RUN |
| RM-10 | Packages / Doctor | 使用本地假 Package 与 Agent 设置验证登记、导入及诊断展示。状态符合实际，无真实用户路径访问、无隐式全局安装 | NOT_RUN |
| RM-11 | 托盘与生命周期 | 执行第 5 节完整序列，窗口、GUI 与后台 Proxy 状态符合当前实现 | NOT_RUN |
| RM-12 | 失败与保护路径 | 测试受控端口占用、非法端口、上游不可达、操作取消。错误明确；不启动重复进程、不接管或终止无关服务、不破坏已有测试数据 | NOT_RUN |

### 4.1 物理显示与 GPU 专项

- GPU：保留能将实际应用与渲染设备/后端关联起来的正向证据。存在 `/dev/dri`、能打开窗口或没有日志告警，都不能单独证明硬件加速。
- 若实际为 llvmpipe，明确记录“软件渲染”；主功能可单独通过，但不能将 GPU 硬件路径记为 PASS。拿不到正向证据时该项记 BLOCK。
- 高分屏：宿主机已具备 `3200×2000 / scale 2` 的对应输出条件，但应用布局和实体显示验收仍为 NOT_RUN。关闭该门禁需要本轮实体输出与应用交互证据；其他分辨率、虚拟输出或仅环境核对不能替代。
- 多屏：具备多屏时跨屏移动、切换焦点、调整布局，核验候选窗、弹窗和点击坐标。缺少硬件时记录该子场景 BLOCK。
- 不能用合成输入替代物理键盘与 IME 验收；候选与提交分别留证。

### 4.2 Alt+F10 专项判定

先复测物理 Alt+F10，再记录实际合成器绑定、应用焦点和菜单行为。若按键被合成器截获，记录环境冲突及可用替代入口；不能直接归因为应用缺陷，也不能据此宣布历史失败已修复。菜单的应用侧行为证据不足时，相关子项保留 BLOCK。

## 5. GUI / Proxy 生命周期序列

预期依据：[当前窗口逻辑](../../desktop/shell.go) 与 [Native Desktop 说明](../../desktop/README.md)。**关闭窗口、隐藏窗口或退出 GUI 不会自动停止 Proxy。**

| 编号 | 操作 | 预期结果 | 状态 |
|---|---|---|---|
| LC-01 | GUI 启动本轮受管 Proxy | 健康检查和身份核验成功，记录端口与本轮进程身份 | NOT_RUN |
| LC-02 | 再次点击启动 | 不生成第二个受管 Proxy，不错误覆盖 PID 状态 | NOT_RUN |
| LC-03 | 有可用托盘时关闭窗口 | 窗口隐藏，可通过托盘恢复；GUI 和 Proxy 均仍运行 | NOT_RUN |
| LC-04 | 无可用托盘时关闭窗口 | GUI 退出，不形成无法恢复的隐藏实例；Proxy 仍运行 | NOT_RUN |
| LC-05 | 分别在有/无托盘条件使用 `--start-minimized` | 有托盘时按预期隐藏；无托盘时窗口保持可见 | NOT_RUN |
| LC-06 | 同一会话、同一隔离环境再次启动 | 激活已有 GUI，不产生重复管理实例 | NOT_RUN |
| LC-07 | 显式退出 GUI | GUI 退出，Proxy 保持运行 | NOT_RUN |
| LC-08 | 重开 GUI | 正确识别原受管 Proxy，不误报其他服务为受管服务 | NOT_RUN |
| LC-09 | 显式停止受管 Proxy | 受管进程和监听退出，PID 状态清理正确 | NOT_RUN |
| LC-10 | 自己启动受控假监听服务，占用另一个测试端口 | 应用拒绝接管或停止它；假服务始终保留，最后由创建者清理 | NOT_RUN |

托盘正向验收需要实际可用的宿主和运行依赖；缺失时只能证明降级路径，托盘正向项必须记 BLOCK。创建无托盘条件时使用可恢复的测试会话配置，不破坏真实桌面服务或卸载全局依赖。

## 6. 第二阶段：安装包与系统事件

核心功能无未解决阻断后，使用第 2 节固定的完整包执行：

| 编号 | 项目 | 操作与通过标准 | 状态 |
|---|---|---|---|
| PK-01 | 安装与重装 | 获得单独执行授权并核对影响范围后，在宿主机按包支持的隔离方式安装、运行、重装；资源、入口和配套 CLI 正确，哨兵及现有用户数据不被覆盖 | NOT_RUN |
| PK-02 | 升级与降级 | 选定实际源码不同且兼容的旧包，记录双版本身份；升级后再降级，执行文件 SHA256 符合目标版本，哨兵与数据仍完整 | NOT_RUN |
| PK-03 | 卸载 | 只移除本轮包及其明确拥有的入口，按既定策略保留数据；不波及无关文件或其他实例 | NOT_RUN |
| PK-04 | 数据完整性 | 每一步核验 config、models、SQLite、Agent Session 哨兵；SQLite `PRAGMA integrity_check` 正常，敏感数据文件权限符合约定 | NOT_RUN |
| SY-01 | 锁屏与挂起恢复 | GUI、焦点、输入法和受管 Proxy 状态恢复正确，无重复实例或误报 | NOT_RUN |
| SY-02 | 实体显示器拔插 | 有相应硬件时测试拔插及混合缩放，窗口和弹窗可恢复且不遗失到不可达区域 | NOT_RUN |

执行任何覆盖、降级或卸载前核对目标路径和影响范围。若安装器不能保持测试账号与数据隔离，暂停该项并记 BLOCK，不尝试绕过。使用当前包自带的说明和入口，不猜测安装目录或清理命令。

真实用户数据迁移不属于首轮操作；如另行安排，需先获得明确授权、备份与恢复方案。

## 7. 留证、状态与完成标准

### 7.1 每条用例记录

```text
用例编号：
执行时间、测试人：
候选源码 / GUI / CLI / 包标识：
实际硬件、会话、输入法、缩放、主题、托盘环境：
前置测试数据与受控服务：
操作步骤：
预期结果：
实际结果：
状态：NOT_RUN / PASS / FAIL / BLOCK
证据路径：截图或录像、日志、必要的文件哈希/权限/DB 断言
失败或阻塞原因、下一步：
```

- **NOT_RUN**：尚未执行。
- **PASS**：本轮实际操作、断言和必要证据均满足标准。
- **FAIL**：实际复现功能、数据或安全错误；不得被总体 BLOCK 掩盖。
- **BLOCK**：缺少硬件、依赖、环境、权限或有效证据，无法形成结论。
- 不得将未执行、历史通过、仅容器通过或仅有截图但未核验的项目标为真机 PASS。
- 证据仅来自假数据；分享前检查日志、截图、配置备份与导出，移除任何意外敏感内容。测试证据不默认提交到 Git。

### 7.2 完成标准

1. 第一阶段必测项目均有实测记录，所有子场景的 FAIL / BLOCK 单独列明。
2. 数据完整性、并发保护、受管进程身份与无关服务保护无未解决 FAIL。
3. “真机桌面验收通过”需要原生会话、物理输入、IME、键盘、缩放、主题、托盘和生命周期门禁均有正向证据；保留的硬件门禁必须明确披露，不能用“基本可用”替代。
4. “发布包验收通过”还需要完成安装包阶段；有架构或包格式未覆盖时明确限制结论范围。
5. 结论分别列出功能、真机桌面与发布包状态；存在 FAIL 时不能宣称该范围通过，存在 BLOCK 时不能宣称该范围完整验收。

### 7.3 收尾

- 正常退出本轮 GUI；仅通过同一隔离环境显式停止身份已核验的本轮 Proxy。
- 假上游、假监听由其创建者按已记录身份清理；不使用宽泛 `pkill`，不根据过期 PID 终止进程。
- 核验本轮监听和进程已退出；保留 run 目录、候选包、证据及失败现场，不自动删除或覆盖。
- 不修改、提交或推送其他工作树改动；本计划落盘不代表已执行任何真机测试。
