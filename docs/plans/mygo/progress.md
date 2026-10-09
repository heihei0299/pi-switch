# MyGo 桌面化进度

更新日期：2026-10-08
分支：`mygo`（记录时 `HEAD` 为 `f8ebc89`，领先 `origin/mygo` 4 个提交）

## 当前结论

- **继续保留 WebUI 作为默认生产界面**：MyGo 窗口当前承载 React/WebUI；不推进 WebUI 退役或默认切换到原生 UI。
- 原生页面仍是可选原型。整体工作暂停，尚未达到发布验收条件。

## 工作包进度

| 工作包 | 进度 |
| --- | --- |
| WP-00 | 本地基线完成；远程 GitHub CI 尚未验证。 |
| WP-01 | Linux spike 部分完成；Gate A 阻断，交互式 GUI 验收未完成。 |
| WP-02～WP-04 | 桌面外壳、服务适配和 WebUI 承载已有实现；相关验收仍未完成，Gate B 未通过。 |
| WP-05 | 原生 Overview 可选原型已加入；WebUI 仍是默认/回退入口，Gate B 未通过。 |
| WP-06 | JSON、Gateway、Stats、Packages 原生原型已加入；功能等价和实机验收不足，Gate C 未通过。 |
| WP-07 | 独立测试产物构建与 Linux tarball 检查已加入；尚未发布，Gate D 未通过。 |
| WP-08 | 延后 WebUI 退役；未删除 WebUI 代码、依赖或入口。 |

WP-05～WP-08 的本地提交：`f8f773e`、`4d260b0`、`7d93c26`、`f8ebc89`。尚未推送。

## 验证与待办

桌面 Go 测试、`go vet`、MyGo vet、构建及若干隔离数据专项测试已通过。最终全量测试尚未运行；真实交互式 GUI、跨平台实机验收和远程 CI 也未完成。后续如恢复推进，需补齐相应 Gate 证据，再按原计划完成全量测试。

详见各工作包记录：[`docs/test-reports/`](../../test-reports/)。
