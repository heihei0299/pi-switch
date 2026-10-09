# T06 — 最终架构审计、Linux GUI 与发布前完整验证

- **源需求**：SPEC §5、§6 WP-07、§8
- **优先级**：P0 / 最终合并门禁
- **前置**：T05 已交付；T01～T05 均有测试、Review、Commit 记录
- **交付**：最终验证证据、兼容清单及必要的 CI/文档收尾（如需变更则一个独立交付 Commit）；不自动合并或发布

## 工作范围

完成完整 Linux amd64/arm64 测试及安装验证、真实 Wayland 图形验收、安全/依赖审计，并形成可以人工判断是否合并 `main` 的最终报告。此 Ticket 不是重新实现业务，也不是把 T01～T05 已处理的所有测试无理由逐项重复。

## 实施步骤

1. **架构审计**：检验 Native → application → core 的依赖方向、无第二套业务规则、页面状态隔离、异步过期结果、并发安全及旧 React/WebView 依赖清零。允许保留无头 HTTP Proxy/必要管理 API，但要求清晰注释其消费者。
2. **自动化 CI**：运行主模块与 Desktop 模块测试/vet、安全扫描、Linux amd64/arm64 构建及 tarball install/reinstall/uninstall smoke；更新对应 workflow 以反映纯 Native 构建事实，确保无误触发 tag/npm 发布。
3. **实际 GUI**：Arch Linux + niri/Wayland 上验证启动/退出、单实例、窗口/焦点、菜单/托盘降级、显示缩放（3200×2000、scale 2）、fcitx5 中文输入法、选区复制粘贴、键盘导航、深浅色、滚动长列表。
4. **复杂功能与安全回归**：Gateway Preview/Conflict/Cancel/Apply、JSON 非法草稿和 IME、Stats unknown 与图表、Sessions 归属、Packages 软卸载、Proxy 进程身份与独立保活。
5. **安装与数据兼容**：隔离用户 HOME 中执行冷装、升级、备份/恢复、卸载/重装；确认 config、models、DB、Sessions 完整且无密钥泄露；不同 Linux 分发目标未实测则明确“不支持/未验证”，不冒充 PASS。
6. **发布材料**：记录依赖版本、Linux ABI/图形库要求、明确支持的桌面环境与包格式、已知限制、回退方案以及最终人工审批结论。

## 合并门禁（全部必须）

- [ ] 正式 GUI 全部是 MyGo Native，无运行中的 React/WebView 管理入口。
- [ ] HTTP/Native 业务规则不分叉；无 core 依赖 GUI、无 Native 依赖 handler 业务实现。
- [ ] 全量 Go/Native 测试、vet、Secrets Scan、Linux amd64/arm64 构建与安装 smoke PASS。
- [ ] 指定 Arch/niri/Wayland 的真实 GUI 功能及中文 IME 验收 PASS；失败或未执行不能写 PASS。
- [ ] 原有配置/模型/请求历史/Pi sessions 兼容，密钥未泄露；Gateway 仅显式发布。
- [ ] 退出 GUI 不误杀 Proxy、CLI/Proxy 可无头运行，未接管未知进程。
- [ ] Windows/macOS 不在支持及构建矩阵；README、兼容/退役说明、CI 与最终实现一致。
- [ ] 所有阶段都有可追踪验证与 Review；未解决阻断问题清零，由人工决定是否合并。

## 建议验证命令（以最终仓库脚本为准）

~~~bash
go test ./...
go vet ./...
go -C desktop test ./...
go -C desktop vet ./...
bash scripts/check-secrets.sh
# 另执行更新后的 Desktop CI matrix / Linux installer smoke
~~~

GUI、图形驱动、托盘和中文输入必须实测，不得以 CI 的交叉编译替代。

## 产物

- T01～T06 验收表：命令/环境/时间、PASS/FAIL/BLOCK、未测项及风险。
- 依赖方向/旧 UI 残余审计报告；Linux 发行版与包格式支持声明。
- 回滚与恢复说明；提交 SHA 清单；明确的人工合并审查结论。

**禁止事项**：默认发布/npm publish、自动合并 `main`、隐瞒无法测试的平台、修改真实用户数据、无授权强推或大规模破坏性清理。

