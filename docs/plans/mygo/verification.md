# 验证命令、CI 准入与交付证据

> 以下命令是执行阶段要求，非本次文档重组的运行记录。每个工作包的实际命令与结果必须逐项记录。

## 5. 验证命令与 CI 门槛

以下是**执行阶段应运行的命令清单**，不是本计划提交已运行的测试。若脚本/工具链变化，记录新命令与原因，不能静默跳过。

### 5.1 既有核心基线（仓库根目录）

~~~bash
go version
node --version
npm install
npm run build:webui
go vet ./...
PI_SWITCH_DB="$(mktemp -u)" go test ./...
npm --prefix webui run typecheck
npm run test:webui
cd webui && npx playwright install --with-deps chromium && cd ..
npm --prefix webui run test:e2e
bash scripts/check-secrets.sh
npm run build:go
bash scripts/functional-check.sh bin/pi-switch
bin/pi-switch --build-info
~~~

受限机器可以用仓库自带 <code>bash scripts/test-limited.sh</code> 跑 Go/WebUI/TS/smoke 套件，但仍需另跑 CI 中的 vet、Playwright、secret scan、构建身份验证；该脚本不能替代完整 CI。CI 当前 Go 测试失败项应按 WP-00 单独解决后才把全绿作为实施前置。

**注意**：上面 PI_SWITCH_DB 的一次性空路径只用于测试套件；写入配置或触发 daemon 的集成实验必须使用 AT 测试专门目录并同时设置四个隔离环境变量，不能仅靠 PI_SWITCH_CONFIG_DIR。

### 5.2 新增桌面验证（WP-01 验证通过后确定并落库）

建议独立桌面模块，以实际创建的命令和依赖版本为准：

~~~bash
# 示例路径/任务，不表示 desktop/ 目前已经存在
go -C desktop version
go -C desktop test ./...
go -C desktop vet ./...
# mygo dev/build：使用锁定版 CLI，记录精确调用参数和输出目录
# 每个目标平台：编译产物 + 安装包内容校验 + 真机 GUI smoke
~~~

- 新建 GUI/IPC 测试：服务调用 DTO、错误/取消、安全 origin、单实例和窗口生命周期；
- 新建原生 UI 测试（如果选择 M3）：页面事件、键盘焦点、长列表、草稿编辑、无障碍；
- 新建跨进程回归：GUI 与 Proxy/daemon 在相互崩溃或退出后仍符合所有权约定；
- 新增主分支保护前先检查 CI 的触发规则：现有 workflows 仅对 main push、面向 main PR 和 workflow_dispatch 运行；在 mygo 分支的普通 push 不会自动获得完整 CI 证明。

### 5.3 最终 PR 验收顺序

1. 单一工作包的新增/改动测试（必过），再执行对应核心回归（必过）。
2. code review：确认未复制业务规则、无密钥/未授权文件读取、遵守系统契约。
3. 完成工作包的验证后提交一个 commit；报告 commit SHA、确实执行的命令、失败与未验证项。
4. M2 候选走全量 Go + WebUI + Desktop + CI cross build + 安全审计 + 至少 Linux/Windows 真机 GUI 验收。
5. 主分支全量测试和评审通过后**再单独决定**是否合并；不得自动合并、推送额外分支或发布 npm。
6. M3 删除旧栈之前另做完整一轮行为、性能、用户数据迁移与回滚验证。

## 7. 必须留存的实施记录

每个工作包/issue 至少包含：

~~~text
WP/Issue：WP-xx / #xx
目标及不在范围内的事项：
前置条件 / 基线 SHA / MyGo 锁定版本：
修改文件及架构边界：
具体测试命令（实际执行）：
验收 AT-*：PASS / FAIL / BLOCK（含链接）
跨平台结果：Linux Wayland / Windows / macOS（未测必须写明）
安全与数据兼容检查：
code review 结论：
遗留风险与回滚步骤：
交付 commit SHA（每工作包一次；不含无关变更）：
是否提议合并：否 / 待批准
~~~

验收证据可以保存为独立 docs/test-reports/ 文档、CI artifact 或 issue 评论，**不得包含真实 API Key 或任何敏感文件**。不复用旧会话的隐含结论；后续实现首先读取本计划和最新 system-contract，若发现冲突先新增 ADR 决策，不悄悄更改合同。
