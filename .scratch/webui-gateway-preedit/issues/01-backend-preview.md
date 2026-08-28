# 01 后端预览与保留

Status: resolved

Seam: `GET /api/models/gateway/preview` dry-run + `PUT /api/models/gateway` 落盘 + `sync_gateway_to_pi` extra 保留

输入：
- 当前 `~/.pi/agent/models.json` 的 gateway 条目（可能不存在）
- 当前 `~/.pi-switch/config.json` 的 profiles/settings

输出：
- `preview`: `{ current: Value|null, proposed: Value, conflicts: string[] }`，不写盘
- `apply`: 校验通过后原子写入 models.json，仅替换 gateway id，其他 providers 保留
- `sync` 保留：无编辑时自动合并 current 的顶层 extra 与 model extra

验收：
- preview 不产生备份文件、不写盘
- conflicts 正确列出将被覆盖的手写键
- apply 非法 JSON / 非法 gateway schema 返回 400
- sync 后非网关 providers 原样保留

## 实施总结
- 提交：`13e4bd2` — `feat(webui): add gateway pre-edit preview to prevent models.json overwrite`
- 实现的 seams：后端预览/应用 + 前端 diff + 预览弹窗 + 面板接入
- 验收标准：
  - [x] preview 不写盘且返回 current/proposed/conflicts
  - [x] apply 校验并原子写入，仅替换 gateway
  - [x] sync 保留手写 extra
  - [x] 前端 diff/校验/弹窗 可编辑且非法时禁用确认
  - [x] 全部保存链路经预览，取消不写盘
- 测试结果：Rust 212 passed, webui 114 passed
- typecheck：通过 (cargo check + tsc --noEmit)
- 文档对齐：WEBUI_GUIDE 已更新 REST map 与 gateway pre-edit 章节
- 遗留 / 后续建议：pending-aware preview（基于待提交 profile 的 proposed）可进一步做 POST preview；per-model conflicts 细化
