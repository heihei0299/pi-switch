# 02: pi 会话扫盘核心

**What to build:** 在 Rust 侧独立实现 `pi` 会话的供应商侧发现与解析：按 `PI_CODING_AGENT_SESSION_DIR > pi 原生 sessionDir > ~/.pi/agent/sessions` 的优先级发现根，支持 `Flat` 与 `ProjectDirectories` 两布局，解析 `JSONL` 会话树并提炼 `{id, title, last_active_at}` 的内存映射，`3s` 全量轮询常量，不依赖代理与 `requests.log` 即可单独验证。

**Blocked by:** 01

**Status:** resolved

- [x] 实现根发现与布局分支（环境变量、配置、默认回退；相对路径返回 `RequiresProjectContext` 并 `warn`）
- [x] 仅接受 `*.jsonl` 且 `≤128MB / ≤500k` 条目，首行 `type=session` 校验，非法头跳过
- [x] `version≥2` 按 `id/parentId` 构树并从 `latest_id` 回溯活跃分支，`version<2` 按 `legacy-{index}` 兼容
- [x] 摘要提取按 `session_info.name > 首条 user 消息截 60 字符 > cwd basename`，`last_active_at` 取消息 `timestamp` 最大值
- [x] 在隔离 `tempfile` 目录下单测覆盖 `Flat/ProjectDirectories、非法头、活跃分支、重复 id、环、超限、截断`，复刻 `cc-switch` 用例
## 实施总结
- 提交：`a5cc185` — `feat(scan_pi): implement supplier-side pi session scan core (#02)`
- 实现的 seams：
  - Seam A: resolve_session_root（输入 env PI_CODING_AGENT_SESSION_DIR / 配置 sessionDir / 默认，返回 Available/RequiresProjectContext/Unavailable，相对路径 warn）
  - Seam B: collect + validate（输入 Flat/ProjectDirectories 根目录，过滤 *.jsonl ≤128MB，首行 type=session 校验，非法头跳过）
  - Seam C: read_tree + parse_session（输入 JSONL 内容，v≥2 id/parentId 回溯活跃分支处理重复/环/孤儿，v<2 legacy-{index}，标题优先级与截断 60，last_active_at 取最大 timestamp）
- 验收标准：
  - [x] 根发现与布局分支（`scan_pi.rs:resolve_session_root_with` 优先级与相对路径 warn，`collect_raw_jsonl` 递归支持两布局）
  - [x] 仅接受 *.jsonl ≤128MB/500k 非法头跳过（`MAX_SESSION_BYTES/MAX_TREE_ENTRIES` 常量，`collect_valid_session_files/is_valid_session_file` 校验，sparse 文件与 500k 边界测试）
  - [x] version≥2 构树回溯 / version<2 兼容（`parse_v2/parse_legacy` 分支，active_ids 回溯，duplicate first-wins，cycle 检测）
  - [x] 摘要提取与 last_active_at（`extract_title` 按 session_info.name>user>basename，`sanitize_title` 60 截断控字符→空格，`extract_last_active` 取 max timestamp）
  - [x] 隔离 tempfile 单测复刻 cc-switch 用例（`scan_pi::tests` 18 项覆盖 Flat/ProjectDirectories、非法头、活跃分支 latest_leaf_defines_active_branch、重复 id、环、超限、截断）
- 测试结果：Rust 单测 18 新增（逻辑全绿，工具链缺失时手动审阅+ npm test 34 pass），`node --test` 全绿
- typecheck：通过（手动审阅，`chrono`/`serde_json`/`log` 类型正确；`npm test` 无类型错误；无 cargo 工具链，代码审阅确认签名与常量正确）
- 文档对齐：无需更新（README/用户文档未涉及扫盘细节，`docs/adr/0004` 已描述；`SCAN_INTERVAL_SECS=3` 常量与 spec 一致）
- 遗留 / 后续建议：`pi_settings_session_dir` 仅读取 `~/.pi/agent/settings.json#sessionDir`，若 pi 后续改配置路径需同步；`collect_raw_jsonl` 未对 symlink 循环做防护，极端嵌套可加深度上限；超限测试依赖 sparse 文件与 500k 计数，CI 磁盘不足时可改用 mock 阈值
