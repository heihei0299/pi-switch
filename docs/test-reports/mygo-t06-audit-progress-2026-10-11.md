# MyGo Native T06 最终架构审计进度（2026-10-11）

## 状态

**INCOMPLETE / NOT RELEASE ACCEPTANCE。** 本文记录 T06 自动化与静态审计进度，不关闭最终验收，不代表桌面发布就绪。

审计代码提交：`31e06b8a9cf91eb4fbc8cf6b4379b2fadcba8223`（`mygo`）；核验时 `origin/mygo` 与该提交一致。

## 本阶段变更

| Commit | 范围 | 文件 |
| --- | --- | --- |
| `dab65a6` | 加入 legacy/current Pi session JSONL 读取回归；修正过期 CI 说明 | `internal/scan/scan_compat_test.go`, `desktop/README.md` |
| `a47b8d8` | 明确 Linux build target 不构成发行版支持承诺 | `desktop/README.md` |
| `79f994e` | 两个架构均运行 tarball install/reinstall/uninstall archive smoke | `.github/workflows/desktop.yml` |
| `4e59abf` | 记录双架构 smoke 结果及其不执行应用二进制的边界 | `desktop/README.md` |
| `fedf99e` | 加载 Stats 时禁用自定义日期控件 | `desktop/native_complex.go`, `desktop/native_stats_loading_test.go` |
| `31e06b8` | 处理加载期间已打开的日期弹窗：丢弃旧筛选结果并按最新日期重跑，仅重置变化范围的分页 | `desktop/native_complex.go`, `desktop/shell.go`, `desktop/native_stats_loading_test.go` |

以上提交均分别推送到 `origin/mygo`。其他既有 staged、unstaged、untracked 改动未纳入这些提交。

## 审计证据

| 项目 | 状态 | 证据与边界 |
| --- | --- | --- |
| Native 与 HTTP 适配器边界 | PASS（静态检查） | `desktop/native_*.go`、`desktop/shell.go` 未导入 `internal/server` 或 `webui`；唯一旧 UI 消费者是保留的 `desktop/management.go` 回退注册器。 |
| Stats 自定义日期异步一致性 | PASS（自动化） | 新测试覆盖加载时无法打开日期控件、已打开日历在加载期间修改后重跑最新筛选，以及只重置受影响分页。红测曾复现“加载期间旧日历仍可改日期”。 |
| 配置、备份、Gateway、Proxy 身份 | PASS（自动化范围） | Root/Desktop 全套测试通过；现有用例覆盖旧配置迁移、备份/恢复、Gateway 冲突与并发发布保护、旧 SQLite 行/unknown 保留及 daemon 身份核验。测试通过不替代真实用户数据迁移。 |
| Pi session 兼容 | PASS（自动化范围） | 新增旧格式与当前格式 JSONL 文件摘要读取测试；只读扫描，不写 session 文件。 |
| Root CI | PASS | run `38153441383`，精确提交 `1f876e1`；Go vet/test、WebUI 检查、secret scan、双架构 CLI build、functional smoke、npm package check 成功。Release preflight 与 publish 跳过。 |
| Desktop CI | PASS | run `38153416434`，精确提交 `1f876e1`；Desktop test/vet、双架构打包、amd64 与 arm64 archive 安装/覆盖/卸载 smoke 成功。amd64 `.deb` artifact 的 build identity 与该 SHA 一致。 |
| 本地 Desktop 回归 | PASS | `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CGO_ENABLED=0 go -C desktop test -count=1 -mod=readonly ./...`、`go -C desktop vet -mod=readonly ./...` 和 Stats 定向回归测试通过。 |
| Secret scan | PASS（tracked source） | `scripts/check-secrets.sh` 本地与 CI 均通过；该扫描器范围是 Git 跟踪文件。 |
| Debian `.deb` 生命周期 | PASS（隔离范围） | 从精确 HEAD `1f876e1` 的 Desktop CI run `38153416434` 下载 amd64 artifact；包 SHA-256 `a5f474587f9b015ffa126fe06b15b578d285180bd92ac64a5a49792554b83ae1`，内嵌 build identity 与 HEAD 一致。在 Ubuntu 24.04 测试容器的 rootless user/mount namespace 中，以独立 `dpkg --root` 完成 install、reinstall、`dpkg --verify`、remove、purge；确认宿主 dpkg 数据库无该包。因隔离 root 没有 GTK/WebKitGTK，测试显式使用 `--force-depends`；未安装宿主依赖、未启动 GUI，不能替代系统级安装/依赖解析/回滚验收。 |
| Linux 分发边界 | 已记录，仍有限制 | Desktop 仍是短期 CI test artifact，不是公开发行；CI tarball smoke 只检查归档安装/覆盖/卸载和数据保留，不执行打包程序。系统级 Debian 包安装/回滚、真实 arm64 二进制运行、真实用户迁移仍未验证。 |
| 正式 GUI 门禁 | **PENDING（未运行，不是 PASS）** | Arch+niri/Wayland、fcitx5/IME、焦点与键盘、硬件缩放、托盘降级、受控 Proxy 生命周期尚无正式验收证据。Ubuntu/Xvfb/Openbox smoke 不替代这些门禁。 |

## 未关闭项与停止边界

1. 正式 GUI 门禁仍为 **PENDING**，不能写成 PASS 或据此宣称发布就绪。
2. 按用户兼容决策保留 Desktop `--web` / `pi-switch-ui://` fallback；不得在正式 GUI 门禁通过且弃用说明发布前删除。故 T05 尚未完全关闭。
3. 系统级 Debian 包依赖解析/安装回滚、arm64 实际运行、真实用户数据升级/迁移与卸载后数据检查仍待验证。
4. 本报告是进度审计，不是最终 T06 结论；T06 验收与最终发布门禁继续开放。本阶段未 merge、未创建新 tag、未发布 Desktop，也未修改真实用户数据；此前的 `v20261011.0.0` 是 CLI/npm 发布，不是 Desktop 发布。
