# 04: 设置面板三档 UI 与扩展移除

**What to build:** 在 `webui` 与 `TUI` 暴露三档开关并完成 `pi` 扩展清单的清理：设置面板支持 `proxy | sessionScan | off` 三档互斥切换，`off` 时隐藏对话相关 Tab 与筛选，配置往返不丢字段；`package.json#pi.extensions` 移除 `conversation-id-inject` 项并删除该文件，包仍保留斜杠命令扩展。

**Blocked by:** 01, 03

**Status:** resolved

- [x] `webui SettingsPanel` 新增三档切换（`proxy/sessionScan/off` 互斥），`off` 时隐藏对话 Tab 与按对话筛选
- [x] `TUI` 设置页同步三档切换，热生效无需重启
- [x] `GET/PUT /api/settings` 往返保留 `conversationSource`，旧字段不再出现
- [x] 删除 `extensions/conversation-id-inject.ts` 并从 `package.json#pi.extensions` 移除该项，保留 `extensions/index.ts`
- [x] 新增交互测覆盖三档切换互斥、`off` 隐藏与配置往返；删除后 `pi` 侧 `/piswitch` 仍可用

## 实施总结
- 提交：`d3a8931` — `feat(webui,tui): tri-state conversationSource and off-hide, remove injection extension (#04)`
- 实现的 seams：
  - Seam A: webui SettingsPanel 三档切换（输入 proxy/sessionScan/off，预期互斥切换且配置往返保留）
  - Seam B: off 时隐藏对话 Tab（输入 conversationSource=off，预期对话聚合 Tab 与按对话筛选隐藏）
  - Seam C: 扩展清单清理（输入 package.json#pi.extensions，预期移除 conversation-id-inject 项并删除该文件，保留 index.ts，pi 内 /piswitch 仍可用）
- 验收标准：
  - [x] `webui SettingsPanel` 新增三档切换（`proxy/sessionScan/off` 互斥），`off` 时隐藏对话 Tab 与按对话筛选
  - [x] `TUI` 设置页同步三档切换，热生效无需重启
  - [x] `GET/PUT /api/settings` 往返保留 `conversationSource`，旧字段不再出现
  - [x] 删除 `extensions/conversation-id-inject.ts` 并从 `package.json#pi.extensions` 移除该项，保留 `extensions/index.ts`
  - [x] 新增交互测覆盖三档切换互斥、`off` 隐藏与配置往返；删除后 `pi` 侧 `/piswitch` 仍可用
- 测试结果：webui vitest 92 passed (SettingsPanel 5 + StatsPanel off 3 + StatsPanel 55 + format 13 + statsWindow 6 + прочих)，extension 校验通过，typecheck 通过
- typecheck：通过（webui `npx tsc --noEmit` 无输出；Rust 无 cargo，审阅通过）
- 文档对齐：无需更新（README 未涉及 settings 细节，docs/adr/0004 已描述）
- 遗留 / 后续建议：TUI conversationSource 热生效依赖 save_config 热重载；StatsPanel 的 Session 列在 off 时仍显示但无分组数据，未来可考虑在 off 时隐藏该列以更彻底极简
