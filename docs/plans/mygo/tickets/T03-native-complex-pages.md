# T03 — Gateway、JSON、Stats、Sessions、Packages 全原生化

- **源需求**：SPEC §4、§5、§6 WP-04
- **优先级**：P1／关键路径
- **前置**：T01 完成；可与 T02 并行，但不得提交 T04 前遗留关键功能缺口
- **交付**：复杂业务功能由 MyGo 原生 UI 完整承担；本 Ticket 一个交付 Commit

## 目标与范围

基于现有 `desktop/native_complex.go` 原型，不重写稳定的 Gateway、Stats/Store、Pi Session 或 Packages 业务规则。补齐原型与正式 WebUI 的关键交互能力，消除“可显示但无法完成工作”的伪等价。

## 实施步骤（内部检查点；不必拆额外 Tickets）

1. **Gateway**：将模型选择、生成/草稿预览、当前与 proposed 的差异、冲突、错误诊断、显式确认发布形成完整操作闭环；展示由共享 canonical plan 计算的结果，不在前端重算规则。发布仅写规定目标，保留外部 provider。
2. **JSON 编辑器**：补齐编辑、粘贴、选区、撤销/重做、JSON 校验/错误定位及必要高亮/行号或等效可读提示，重点实测 fcitx5 中文组合输入。非法草稿保留，预览不写入，草稿与生成 metadata 策略不同。
3. **Stats**：补齐时间窗、筛选、分页、请求/模型/供应商汇总、图表与错误信息的安全呈现；unknown 不能当 0，失败请求不得错误计费。
4. **Sessions**：复用现有 conversation attribution 与 sessionScan、分页及会话历史读取；Pi sessions 保持只读，不能将未知归属猜测为某个会话。
5. **Packages**：完成列表、导入、添加、启停、确认软卸载和失败恢复；使用 T01 提取的共享服务，避免 HTTP handler 业务耦合。
6. **负载和交互**：100/1000 项数据用虚拟列表/分页，验证长列表导航、输入法、键盘焦点与窗口缩放；不得删除敏感错误脱敏策略。

## 验收（全部必须）

- [ ] Gateway 预览、选择、草稿、冲突处理和发布能全程在 Native UI 完成；预览/取消/冲突零写入；成功发布原子更新，仅用户显式确认；第三方 provider 保留。
- [ ] JSON 编辑中非法内容、中文输入组合、撤销/重做、选区、错误定位不会丢草稿或泄露 Key。
- [ ] Stats/Sessions 具备原 WebUI 的关键筛选、统计、图表、分页、请求明细、会话历史能力；unknown 与归属语义一致。
- [ ] Packages 导入、启停、软卸载及取消/失败均可恢复；无额外持久化事实来源。
- [ ] 性能测量记录冷/热启动、空闲资源和 100/1000 项操作；阈值须在测试前确定，不能事后重定。
- [ ] 至少在实际 Linux Wayland GUI 上走通关键复杂流程；若 MyGo 控件能力存在阻断，**不允许进入 T04/T05**，先修复或明确需求调整并审查。

## 验证与证据

- `go -C desktop test ./...`、`go test ./internal/gateway ./internal/server`，以及新提取服务包测试。
- 使用临时 `PI_SWITCH_CONFIG`、`PI_SWITCH_CONFIG_DIR`、`PI_SWITCH_DB`、`PI_SWITCH_MODELS` 和隔离 sessions 夹具；检查前后文件字节及请求数据库数据不被 UI 浏览改写。
- GUI 手测：中文 IME 输入与非法 JSON → Gateway Preview/Conflict/Cancel/Apply → 1000 行 Stats/请求筛选 → Sessions 分页 → Packages 导入/软卸载。

## 不在范围内

重写 Proxy、统计数据入库、协议/模型路由、创建第二套 JSON 编辑业务数据模型、删除旧前端。

