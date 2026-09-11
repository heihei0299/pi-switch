# 02: 补 `Start` 接线的端到端测试 + 删死分支

Status: resolved (2026-09-11)

**缺口**：fail-fast 只测了未导出的 `waitForHealth`（预关闭 channel），`Start` 里的
`go func(){ cmd.Wait(); close(exited) }()` 完全没有测试覆盖——正是本批自己写下的教训 1 那一类
「测试通过、生产不触发」。

**修**：在 `internal/daemon` 的测试里加 `TestMain`，当自己被以 daemon 参数拉起时立即退出
（`os.Exit(1)`）。`Start` 用 `os.Executable()` 派生，于是子进程会确定性地立刻死掉，`Start` 的接线
第一次可被端到端验证；顺带消除「测试子进程重跑整个套件」这一脏行为（它正是之前 23s 与端口复用的来源）。

**验收**
- [ ] 新增测试：`Start` 返回错误且耗时 < 2s（走完 spawn → Wait goroutine → 早退）
- [ ] 反向验证：把 `exited` 换成 `nil`（即去掉信号）时该测试失败（>2s）
- [ ] `waitForHealth` 的 `if exited != nil` 死分支删除（唯一调用点必传非 nil）
- [ ] 测试子进程不再重跑套件（运行时间可见地下降）
