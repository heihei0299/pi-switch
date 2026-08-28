# 01 后端预览与保留

Status: ready-for-agent

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
