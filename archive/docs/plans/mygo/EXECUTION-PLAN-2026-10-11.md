# MyGo Native GUI：2026-10-11 执行计划（历史记录）

> **已被取代（2026-10-13）**：本文件的状态表、任务顺序、CI 快照和禁止条件只反映 2026-10-11 的工作树，不是当前验收标准。当前阶段按单独提供的 T01→T04 任务执行；不要把本计划的旧 `未执行` 标记或门禁当成现状。

## 当时的基线

- 分支：`mygo`
- 提交：`5041101b406a82b006512adb6948e69c06924b5b`
- 当时 Desktop 仍使用 WebView；真实 Arch+niri/Wayland、fcitx5 等 GUI 门禁没有通过。
- 相应的测试环境与历史 GUI 证据保存在 `docs/test-reports/`；旧证据不代表当前构建已通过实机验收。

## 后续实现状态

- React WebUI / WebView Desktop 已退役；Desktop 使用 MyGo Native。
- 管理 API 仍位于 `/api/*`；历史 `pi-switch webui` CLI 名称仅启动/管理 API，不会打开浏览器。
- CLI、TUI、独立 Proxy 和其 `/v1/*` 接口保留。GUI 退出不隐式停止独立 Proxy。
- Native 复杂页面状态拆分已由 commit `3d49e7c` 完成；异步任务审查未发现需要额外修复的实际问题。
- 真实 Wayland、IME、缩放、托盘及完整图形生命周期验收仍留至下一阶段；不要以 Go/UI tester 或 Ubuntu/Xvfb 证据代替。

## 维护说明

本文件保留为计划沿革记录。当前代码事实以源码、现行 README/API 文档和对应测试为准；当前测试/提交结果记录在工作分支与 `docs/test-reports/` 中。
