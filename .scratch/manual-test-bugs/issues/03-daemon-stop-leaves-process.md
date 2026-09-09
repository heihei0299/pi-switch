# 03 — daemon stop 杀不掉进程，只清了 pid 文件

**现象：** `pi-switch proxy stop` / `pi-switch webui stop` 报告清理了 pid，但实际进程仍在跑、端口仍在监听。

**复现步骤：**
1. `node bin/pi-switch.js webui start --daemon`（proxy 同理），`status` 显示启动，端口可连。
2. 执行 `stop`，输出 `PID 4294967295 is not alive (cleaned up stale PID)`。
3. 再查端口：43110/43199 仍在监听，`/proc` 中进程仍在；最后只能手动 kill。

**预期：** `stop` 后进程退出、端口释放；`status` 显示未运行。

**实际：** pid 文件里记的是哨兵值 `4294967295`（即 `(uint32)-1`），根本不是真实 pid；`stop` 按此 pid 判定存活失败，只删文件不杀进程，造成“停掉了但没停掉”。`start` 报的也是同一哨兵 PID，说明 pid 写入侧就有问题（手动测试 2026-09-02，proxy/webui 双双复现）。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] `start --daemon` 写入真实子进程 pid，`status` 显示的 PID 与实际一致
- [x] `stop` 后进程退出、端口释放，重复 `stop` 不报错且幂等
- [x] daemon 化失败（端口被占等）时返回失败、不留 stale pid 文件
- [x] 相关单测全绿（启停一轮、重复 stop、端口冲突）

## 建议修复方向

修 daemon pid 写入侧：`daemon.Start` 返回并落盘真实子进程 pid（排查哨兵值来源，疑似 pid 回传/类型转换丢失）；`Stop` 按 pid 先发信号再回查，杀不掉则报错而非静默清文件。

## Comments

- 来源：`docs/manual-test-basic.md` 基本使用测试收尾清理阶段，43110（webui）+ 43199（proxy）均复现。
- 现场：测试供应商/网关/Key 均已清理完毕；外部转发的 43112 旧代理全程未碰。
- 关联记忆：多实例下 pid 文件只记最后一个实例（memory #35）；本次是单实例场景，连单实例的 pid 都是错的，优先级更高。

## 实施总结

- 提交：未提交（工作区改动，待确认后提交）
- 实现：根因是 `daemon.Start` 里 `cmd.Process.Release()` 写在取 Pid 之前——`Release` 会把 `Process.Pid` 置 -1，转 `uint32` 即 4294967295。修复：先取 `pid` 再 `Release`，且 `Release` 移到健康检查通过之后（失败分支的 `Kill` 在 Release 后是空操作）。`Stop` 本体逻辑是对的，pid 正确后启停全通。
- 验收标准：隔离环境（`PI_SWITCH_CONFIG_DIR=/tmp/pitest`，43201 端口）实测——start 写入真实 pid（34565），stop 后进程退出、端口释放、`status` 干净；重复 stop 幂等；`go test ./internal/daemon/ ./internal/config/` 通过。
- typecheck：`go vet ./...` 通过。
- 文档对齐：无需更新。
- 遗留：历史残留的坏 pid 文件（值为 4294967295）：`isAlive` 对其必然失败，会按原有 stale 路径自动清理，无需迁移。
