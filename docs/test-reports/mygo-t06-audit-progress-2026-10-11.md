# MyGo Native T06 最终架构审计进度（2026-10-11）

## 状态

**INCOMPLETE / NOT RELEASE ACCEPTANCE。** 本文记录 T06 自动化与静态审计进度，不关闭最终验收，不代表桌面发布就绪。

产品代码验证基线为 `30c6cc3e05617bd6ee2e9ab68289b95da590f960`（Root/Desktop CI 均针对该精确 SHA）。随后 `f1d0b85` 增加包二进制回滚 smoke，`d89de8e` 动态选择源码差异基线，`de16226` 在 arm64 上启动降级后的旧版应用，`6a2e364` 增加有效 SQLite requests.db fixture，`5af89d9` 检查隐私数据文件模式保持 `0600`，`a4be70b` 将回滚基线选择限定为 Desktop 生产 Go 源码。上述提交只改 Desktop CI/测试流程、未改产品代码；Desktop CI 均通过，未重跑 Root CI。

## 本阶段变更

| Commit | 范围 | 文件 |
| --- | --- | --- |
| `dab65a6` | 加入 legacy/current Pi session JSONL 读取回归；修正过期 CI 说明 | `internal/scan/scan_compat_test.go`, `desktop/README.md` |
| `a47b8d8` | 明确 Linux build target 不构成发行版支持承诺 | `desktop/README.md` |
| `79f994e` | 两个架构均运行 tarball install/reinstall/uninstall archive smoke | `.github/workflows/desktop.yml` |
| `4e59abf` | 记录双架构 smoke 结果及其不执行应用二进制的边界 | `desktop/README.md` |
| `fedf99e` | 加载 Stats 时禁用自定义日期控件 | `desktop/native_complex.go`, `desktop/native_stats_loading_test.go` |
| `31e06b8` | 处理加载期间已打开的日期弹窗：丢弃旧筛选结果并按最新日期重跑，仅重置变化范围的分页 | `desktop/native_complex.go`, `desktop/shell.go`, `desktop/native_stats_loading_test.go` |
| `69ccb34` | 固定 Ubuntu 24.04 amd64/native arm64 runner；在 arm64 runner 上执行构建出的 CLI 并核验 target/commit | `.github/workflows/desktop.yml` |
| `435eea6` | 在 amd64 与 arm64 临时 runner 上用 apt/dpkg 测试 `.deb` 安装、重装、移除与 purge | `.github/workflows/desktop.yml` |
| `c117a9b` | 扩展隔离 functional smoke：验证 CLI managed Proxy 的 start/status/health/stop，Desktop CI 在两个架构上执行 | `scripts/functional-check.sh`, `.github/workflows/desktop.yml` |
| `7a93755` | 增加 Debian `.deb` 版本升级/降级回滚及隔离用户数据 sentinel 测试 | `.github/workflows/desktop.yml`, `desktop/scripts/test-deb-package-lifecycle.sh` |
| `6fc5331` | 在双架构 functional smoke 中验证旧 `requests.log` 启动导入 SQLite、统计内容和源文件只读 | `scripts/functional-check.sh` |
| `8332d65` | 增加 native arm64 Desktop 包在 Xvfb 下启动并检查 Native 窗口可见的 smoke；明确不替代正式 GUI 门禁 | `.github/workflows/desktop.yml`, `desktop/scripts/test-native-window-xvfb.sh` |
| `d02b33c` | 增加隔离 v1 配置经 API 读取/保存迁移到 v2 的 runtime smoke | `scripts/functional-check.sh` |
| `6146be4` | 将 `.deb` 回滚 smoke 扩展为高版本含测试 marker 文件，检查升级增加和降级移除 payload | `desktop/scripts/test-deb-package-lifecycle.sh` |
| `8f560d4` | 修正 rollback marker 的安装路径断言；双架构 Desktop CI 通过 | `desktop/scripts/test-deb-package-lifecycle.sh` |
| `30c6cc3` | 通过隔离 Python sqlite fixture 验证旧 requests 表 schema 启动迁移、失败请求事实保留及 legacy log 导入 | `scripts/functional-check.sh` |
| `f1d0b85` | 用 push 事件的前一提交构建旧包，双架构安装→升级→降级，并核对实际应用可执行文件 SHA-256 恢复 | `.github/workflows/desktop.yml`, `desktop/scripts/test-deb-package-lifecycle.sh` |
| `d89de8e` | 不固定历史 SHA；动态选取最新生产 Go 源码提交的父提交作为旧包，确保回滚覆盖真实源代码差异 | `.github/workflows/desktop.yml` |
| `de16226` | 在 arm64 包回滚到源码不同的旧版本后，用隔离 Xvfb 启动并确认窗口可见；明确不是正式 GUI 门禁 | `.github/workflows/desktop.yml`, `desktop/scripts/test-deb-package-lifecycle.sh` |
| `6a2e364` | 用有效 SQLite requests.db 与 sentinel 行替代文本占位，检查 dpkg 安装/升级/降级/移除/purge 后完整性与行事实 | `desktop/scripts/test-deb-package-lifecycle.sh` |
| `5af89d9` | 检查 config/models/SQLite/session 四类隔离用户数据文件在包生命周期后仍为 `0600` | `desktop/scripts/test-deb-package-lifecycle.sh` |
| `a4be70b` | 串行主代理 Review 发现旧基线选择可能命中未被桌面程序链接的 Go 包；改为只选择 Desktop 生产 Go 文件并要求该范围有源码差异 | `.github/workflows/desktop.yml` |

以上提交均分别推送到 `origin/mygo`。其他既有 staged、unstaged、untracked 改动未纳入这些提交。

## 审计证据

| 项目 | 状态 | 证据与边界 |
| --- | --- | --- |
| Native 与 HTTP 适配器边界 | PASS（静态检查） | `desktop/native_*.go`、`desktop/shell.go` 未导入 `internal/server` 或 `webui`；唯一旧 UI 消费者是保留的 `desktop/management.go` 回退注册器。 |
| Stats 自定义日期异步一致性 | PASS（自动化） | 新测试覆盖加载时无法打开日期控件、已打开日历在加载期间修改后重跑最新筛选，以及只重置受影响分页。红测曾复现“加载期间旧日历仍可改日期”。 |
| 配置、备份、Gateway、Proxy 身份 | PASS（自动化范围） | Root/Desktop 全套测试通过；现有用例覆盖旧配置迁移、备份/恢复、Gateway 冲突与并发发布保护、旧 SQLite 行/unknown 保留及 daemon 身份核验。测试通过不替代真实用户数据迁移。 |
| Pi session 兼容 | PASS（自动化范围） | 新增旧格式与当前格式 JSONL 文件摘要读取测试；只读扫描，不写 session 文件。 |
| Root CI | PASS | run `38163109346`，精确提交 `30c6cc3`；Go vet/test、WebUI typecheck/test/responsive、secret scan、双架构 build、API/legacy-config/legacy-SQLite-schema/legacy-log/managed-Proxy functional smoke、npm package check 成功。Release preflight 与 publish 跳过。 |
| Desktop CI | PASS | run `38163090704`，精确提交 `30c6cc3`；Desktop test/vet、amd64 与 native arm64 打包、arm64 Xvfb Native window smoke、双架构 archive smoke 及 API/legacy-config/SQLite/log/managed-Proxy functional smoke 成功。最新 run `38169863342` 对精确提交 `a4be70b` 成功；动态基线仅考虑 Desktop 生产 Go 文件，双架构包生命周期检查有效 SQLite 行/完整性和四类用户数据文件 `0600` 模式，arm64 另启动降级后的 Native 窗口。runner 为 Ubuntu 24.04 amd64 与 native arm64。 |
| T06 主代理自审：规范/架构/安全 | PASS（仅主代理 Review，非独立审查） | 审查 `8f560d4..a4be70b` 的 `.github/workflows/desktop.yml`、`desktop/scripts/test-deb-package-lifecycle.sh`、`scripts/functional-check.sh`。未增依赖或改生产业务边界；包测试拒绝在非 GitHub runner 执行，数据写入临时 HOME，私有 fixture 保持 0600；Xvfb 步骤明确不是 GUI gate。发现并修复“全仓 Go 文件提交可误选不链接进桌面程序的源码作为回滚基线”的测试可信度问题，`a4be70b` 现限定 Desktop 生产 Go 文件并验证其源码差异。 |
| T06 主代理自审：Ticket/功能验收 | PASS（仅主代理 Review，非独立审查；只针对可自动化子范围） | 回滚包由动态源码基线生成；CI 核对实际可执行文件 SHA、包版本、dpkg 校验、SQLite 行/完整性与文件模式，arm64 降级旧包可启动并显示窗口。此证据仍不覆盖实际用户数据库迁移、旧版 GUI 交互等价或正式 Wayland/IME/托盘/缩放/Proxy GUI gate；这些保持未验证/PENDING。 |
| 本地 Desktop 回归 | PASS | `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CGO_ENABLED=0 go -C desktop test -count=1 -mod=readonly ./...`、`go -C desktop vet -mod=readonly ./...` 和 Stats 定向回归测试通过。 |
| Secret scan | PASS（tracked source） | `scripts/check-secrets.sh` 本地与 CI 均通过；该扫描器范围是 Git 跟踪文件。 |
| Native arm64 CLI runtime | PASS（不含 GUI） | Desktop CI run `38158277964` 的 `package test / linux-arm64` 在 Ubuntu 24.04 arm64 runner 上验证 `uname -m=aarch64`，实际运行 Native 构建出的 `pi-switch --version` 与 `--build-info`，并核对 `linux/arm64` 和 commit；同一 job 的 managed Proxy smoke 与 tarball install/reinstall/uninstall smoke 通过。没有启动 Desktop GUI，不能替代 Wayland/IME 或 GUI 进程验收。 |
| Headless Proxy CLI lifecycle | PASS（不含 GUI） | `scripts/functional-check.sh` 为 HOME/XDG/config/DB/daemon 状态配置临时路径，选取临时 loopback 端口并实际执行 `proxy start --daemon`、`proxy status`、health probe、`proxy stop`；验证端口关闭、PID 状态文件删除且停止后 status 返回非运行。旧 GUI 生命周期门禁仍未验证。 |
| 隔离旧版数据 runtime 兼容 smoke | PASS（测试 fixture，不是真实用户数据） | Root run `38163109346` 与 Desktop run `38163090704`、精确提交 `30c6cc3`。旧 v1 config 经 management API GET 解析为 v2（读操作未改写源文件），PUT 保存后确认旧字段移除、conversation source 保持 `proxy`、文件权限为 0600。隔离 `requests.db` 预置缺少新列的旧 `requests` 表和失败请求行；API stats 核验启动时新增 schema 后旧失败事实仍保留（100/20 token），状态/错误/URL 为 null。另一个 legacy `requests.log` 启动导入同一 SQLite stats，API 验证新导入成功请求和源文件哈希不变。未使用真实用户数据；不等于已验证真实用户全量迁移。 |
| Debian `.deb` 生命周期 | PASS（rootless 隔离范围） | 从精确 HEAD `69ccb34` 的 Desktop CI run `38155870873` 下载 amd64 artifact；包 SHA-256 `ac045bbd4810f82d0fd163f771a80bc645f6f1005e51f6d5244946cf7be323c0`，内嵌 build identity 与 HEAD 一致。在 Ubuntu 24.04 测试容器的 rootless user/mount namespace 中，以独立 `dpkg --root` 完成 install、reinstall、`dpkg --verify`、remove、purge；确认宿主 dpkg 数据库无该包。Control archive 仅含 control/md5sums，无 maintainer scripts。隔离数据库缺少 GTK/WebKitGTK，故显式使用 `--force-depends`；系统依赖解析由下行 CI 测试覆盖。 |
| Debian apt/dpkg 基础生命周期 | PASS（安装/重装/移除范围） | Desktop CI run `38156783875` 在 Ubuntu 24.04 amd64 与 native arm64 临时 runner 上，对精确提交 `435eea6` 生成的 `.deb` 执行安装（未使用 `--force-depends`）、`dpkg-query`/`dpkg --verify`、重装、移除、重新安装和 purge；两个架构均成功。未启动 GUI。版本事务补验见下行。 |
| Debian apt/dpkg 合成 payload 回滚（历史） | PASS | Desktop CI run `38161785887`、代码提交 `8f560d4` 曾验证高版本测试包增加 marker、降级后移除；用于包管理器文件差异基础覆盖，不作为应用二进制回滚证据。 |
| Debian apt/dpkg 实际可执行文件回滚及用户数据保留 | PASS（包管理器文件/hash 与合成 SQLite fixture 范围） | 最新 Desktop CI run `38168886741`、提交 `5af89d9`，在 Ubuntu 24.04 amd64 与 native arm64 上，动态选取 `fedf99e` 旧源码包（最新生产 Go 改动 `31e06b8` 的父提交），安装旧包、升级当前包、再降级回旧包。`desktop/native_complex.go` 与 `desktop/shell.go` 的生产代码确实不同。每一步核对 dpkg 版本、安装路径实际 Desktop 可执行文件 SHA-256、`dpkg --verify`；隔离 HOME 中 config/models/session sentinel 不变，四类数据文件模式均为 `0600`，`requests.db` 是有效 SQLite，逐步执行 `PRAGMA integrity_check` 并确认 `('sentinel', 100, 20)` 行未丢失。amd64 旧/当前 SHA-256 分别为 `5d95d6cd8cf6a6815a62d9e439d66be9f84ad5abf9da25125bad25f009d8ab4a` / `c5a3f008e598dc31b551fd8ddc238df31c478bfa0f16c570aa5f62c4d0a7a02e`；arm64 分别为 `72ecef9be4933e20e5ac0f4e71efea345a050dd0b1f030d235eee2728aa9c3d7` / `096f99bc3c856fb4647d0f4078ac70d8fe22455ffe5f1125927a9f108b0f58c0`。arm64 降级后应用也在隔离 Xvfb 中启动并显示窗口。这些是合成隔离数据，不是实际用户数据；Xvfb smoke 不验证旧 Stats 行为或正式 GUI/Wayland/IME/托盘/缩放/Proxy 门禁。 |
| Packaged Native arm64 Xvfb runtime | PASS（仅窗口启动 smoke，不是 GUI 门禁） | Desktop CI runs `38161785887`、`38166877875` 与 `38168886741` 在 native Ubuntu 24.04 arm64 runner 安装打包 app 和 Xvfb，使用隔离 HOME/config/DB/XDG 启动 `--native`，由 `xwininfo` 确认 `pi-switch — Native Management` 窗口可见。两个最新 run 同时验证了源码差异旧包降级后的窗口启动；随后当前包 smoke 也通过。不是 Arch+niri/Wayland、IME、托盘、高 DPI、Proxy 生命周期或真实桌面会话验收。 |
| Linux 分发边界 | 已记录，仍有限制 | Desktop 仍是短期 CI test artifact，不是公开发行；Ubuntu 24.04 amd64/arm64 临时 runner 已通过 `.deb` 安装/重装/移除/purge、marker payload 回滚及行为代码不同源码版本的可执行文件 SHA 回滚。Native arm64 CLI、当前包与降级旧包的 Xvfb 窗口均已实际执行；正式桌面会话和真实用户迁移仍未验证。 |
| 正式 GUI 门禁 | **PENDING（未运行，不是 PASS）** | Arch+niri/Wayland、fcitx5/IME、焦点与键盘、硬件缩放、托盘降级、受控 Proxy 生命周期尚无正式验收证据。Ubuntu/Xvfb/Openbox smoke 不替代这些门禁。 |

## 未关闭项与停止边界

1. 正式 GUI 门禁仍为 **PENDING**，不能写成 PASS 或据此宣称发布就绪。
2. 按用户兼容决策保留 Desktop `--web` / `pi-switch-ui://` fallback；不得在正式 GUI 门禁通过且弃用说明发布前删除。故 T05 尚未完全关闭。
3. Debian 包管理器现已通过动态选取真实生产源码差异版本的可执行文件 SHA-256 升级/降级 smoke；有效 SQLite sentinel 行与完整性检查覆盖完整包生命周期，降级后的旧版应用也通过 arm64 Xvfb 窗口启动 smoke。但这不是实际用户数据库迁移，也没有验证 Stats 等具体旧版交互行为。正式 GUI arm64 门禁、真实用户数据升级/迁移与卸载后数据检查仍待验证。
4. 本报告是进度审计，不是最终 T06 结论；T06 验收与最终发布门禁继续开放。本阶段未 merge、未创建新 tag、未发布 Desktop，也未修改真实用户数据；此前的 `v20261011.0.0` 是 CLI/npm 发布，不是 Desktop 发布。
