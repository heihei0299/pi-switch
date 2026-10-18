## Question

Go 重写中 TUI 如何处理？用 charmbracelet/bubbletea 重写现有 ratatui 界面，还是简化/移除？

pi-switch 现有 `tui.rs` + `ratatui` + `crossterm` 实现终端 UI（profile 切换、状态展示）。CLIProxyAPI 同样使用 `charmbracelet/bubbletea` + `bubbles` + `lipgloss` 实现 TUI。

需要决策：
1. TUI 方案：用 bubbletea 重写 ratatui / 保留 ratatui 通过 Go 调用 / 简化为纯 CLI 无 TUI？
2. 功能范围：TUI 需覆盖哪些操作（profile 列表/切换、gateway 状态、stats 简览）？
3. 与 Go 架构集成：TUI 作为 `pi-switch tui` 子命令独立运行，还是与 proxy/webui 共用进程？

## Type

grilling

## Status

resolved

## Answer

1. **TUI 方案**: 用 charmbracelet/bubbletea + bubbles + lipgloss 重写现有 ratatui 界面，对齐 CLIProxyAPI。
2. **功能范围**: 覆盖 profile 列表/切换、gateway 状态、stats 简览，与现有 TUI 1:1。
3. **集成**: 作为 `pi-switch tui` 子命令独立运行，通过 `internal/tui` 包实现，与 proxy/webui 进程分离。
