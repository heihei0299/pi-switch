# 07: 删 `cmd.Process.Release()`

Status: resolved (2026-09-11)

**依据（实测，见本批 progress）**：`Release()` 与 `Wait()` 是文档上的互斥替代。实测两种顺序：
Wait 已先行（`Start` 里可达的顺序，健康检查至少 200ms）→ Wait 存活并继续等待；
**Release 先于 Wait** → Wait 在 **15µs** 内以 `err=invalid argument` 返回，channel 会在子进程仍存活时关闭。
今天无实时影响（成功路径上 `exited` 已无人读），但这是隐患，且让「顺便回收僵尸」的说法在反向顺序下不成立。

**修**：删掉成功路径上的 `Release()`——现在由 Wait goroutine 负责收尾。

**验收**
- [ ] 仓库内不再出现 `Process.Release`
- [ ] `Start` 成功路径行为不变（测试 + 真实二进制各跑一次）
