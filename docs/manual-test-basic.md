# pi-switch 基本使用测试手册

> 目的：用真实 Key 走完「供应商 → 模型 → 网关 → 使用统计」主链路，发现 bug。
> 入口以 **WebUI 为主**，CLI 只做对照备注（CLI 的 `provider` 目前仅支持 `list/show/use/delete`，新增、拉模型、暴露模型必须走 WebUI 或管理 API）。
> 全程中文，单步结构统一为：操作 → 断言（通过标准）→ 异常记录。

## 0. 前置准备

1. 备份现场：
   - WebUI → Backups 确认有自动备份；或记下当前 `~/.pi-switch/config.json` 的供应商数量。
2. 启动 WebUI（daemon）：
   ```bash
   pi-switch webui start --daemon   # http://127.0.0.1:43110
   pi-switch webui status
   ```
3. 记下基线：打开 Stats 页，记下当前 `TOTAL` 请求数；网关页确认 `pending_count` 为 0（无待发布）。
4. 密钥安全：测试用 Key 随用随删，不截图外发，不写入文档；优先使用可随时撤销的测试 Key。

5. 隔离环境（血泪教训）：服务端读配置只认 `PI_SWITCH_CONFIG`，**不认**
   `PI_SWITCH_CONFIG_DIR`（后者只决定 pid/log 目录）。只 export 后者做隔离是假隔离，
   供应商增删会直接写到真实的 `~/.pi-switch/config.json`。隔离测试必须四个变量一起 export，
   且 daemon 子进程继承父进程环境，export 后再 `start`：
   ```bash
   export PI_SWITCH_CONFIG=/tmp/pitest/config.json \
     PI_SWITCH_CONFIG_DIR=/tmp/pitest \
     PI_SWITCH_DB=/tmp/pitest/req.db \
     PI_SWITCH_MODELS=/tmp/pitest/models.json
   ```
   开测前先调 `GET /api/state` 确认 `profiles` 为空，证明隔离生效；收尾删 `/tmp/pitest` 即可。
测试供应商命名建议统一用 `manual-test-<日期>`（如 `manual-test-0902`），方便清理时识别。

## 1. 供应商：添加 API

操作（WebUI → Profiles）：

1. 点「添加供应商」，填写：
   - 名称：`manual-test-0902`
   - `api`：按上游类型选（如 `openai-completions`）
   - `baseUrl`：上游地址（如 `https://api.openai.com/v1`）
   - `apiKey`：真实测试 Key
2. 保存后用「测试连接」（`POST /api/profiles/:name/test`）验证连通。
3. 对照（CLI 只读）：
   ```bash
   pi-switch provider list
   pi-switch provider show manual-test-0902
   ```

断言：

- [ ] Profiles 列表出现 `manual-test-0902`，`provider show` 能打出刚填的 `api/baseUrl`（`apiKey` 不明文核对，只确认已保存）。
- [ ] 「测试连接」成功；失败时错误信息可读（能区分是 Key 错、地址错还是网络错）。

异常记录：连通失败先跑 `pi-switch doctor`，把 `doctor` 输出脱敏（抹 Key）后记入 bug。

## 2. 供应商：获取模型

操作（WebUI → Profiles → 该供应商 → 拉取模型）：

1. 点「从供应商获取模型」（`POST /api/profiles/:name/fetch-models`），等待返回模型列表。
2. 勾选 1–2 个轻量模型保存（`PUT /api/profiles/:name/models`），记下模型 id（如 `gpt-4o-mini`）。

断言：

- [ ] 模型列表非空，且与上游实际提供的模型对得上（抽查 1 个已知模型 id 在列表中）。
- [ ] 保存后重新进入该供应商，模型仍在（落盘成功）；`config.json` 的该供应商下有对应 `models[]` 条目。

异常记录：列表为空先确认 Key 权限与 `baseUrl` 后缀（`/v1` 有无）；把请求时间、供应商名、`fetch-models` 返回的错误原文记入 bug。

## 3. 网关：添加模型 / 删除模型

语义：网关是只读派生视图，唯一写入路径是「供应商 expose → 网关发布」。本节只测这条正道。新拉取/新建的模型默认不暴露（暴露集为空），需显式 expose 后才进网关；暴露集为空的供应商在网关与代理中均不可见。

### 3.1 添加模型并发布

操作：

1. 在供应商的模型列表中 expose 刚保存的模型（`PUT /api/profiles/:name/expose`）。
2. 进 Gateway 页，看 `Current vs Proposed`：`proposed` 应多出 `manual-test-0902/<模型id>`，`pending_count > 0`（`GET /api/models/gateway/preview`）。
3. 点「应用到 Pi」（`PUT /api/models/gateway`）。
4. 验证落盘：检查 `~/.pi/agent/models.json` 的 `providers[<providerPrefix>]` 中出现该模型 id。

断言：

- [ ] 发布前 `pending_count > 0` 且 diff 仅包含本次 expose 的模型（无无关变更）。
- [ ] 发布后横幅消失，`preview` 的 `pending_count` 回到 0；`models.json` 中能找到新模型。

### 3.2 删除模型并发布

操作：

1. 回到供应商，取消 expose 该模型（或删除该模型条目）保存。
2. Gateway 页确认 diff 为移除该模型，点「应用到 Pi」。
3. 再查 `models.json` 确认该模型已消失。

断言：

- [ ] 两次发布均为显式点击后才落盘，发布前供应商的改动不自动写 `models.json`。
- [ ] 发布后 `pending_count` 为 0；pi 端模型列表不再出现该模型。

异常记录：diff 与实际改动对不上时，记下改前改后 `preview` 的 `current/proposed` 摘要与 `pending_count`。

## 4. 使用：curl 造流，验证统计

操作：

1. 启动代理：
   ```bash
   pi-switch proxy start --daemon   # 默认 127.0.0.1:43112
   pi-switch proxy status
   curl -s http://127.0.0.1:43112/healthz
   ```
2. 记录 Stats 基线：记下 `GET /api/stats`（WebUI Stats 页同源）的 `totalRequests`。
3. 用 curl 经代理发一条非流式请求（模型用网关中的完整 id）：
   ```bash
   curl -s http://127.0.0.1:43112/v1/chat/completions \
     -H 'Content-Type: application/json' \
     -d '{"model":"manual-test-0902/<模型id>","messages":[{"role":"user","content":"ping"}],"stream":false,"max_tokens":16}'
   ```
   期望返回上游的正常 completion（内容不做断言，有 `usage` 即可）。
4. 回到 Stats 页（窗口选当天或全部历史），看请求明细多出 1 条。

断言：

- [ ] `totalRequests` 恰好 +1；`byProvider`/`byModel` 中对应供应商/模型各 +1。
- [ ] 该请求的输入/输出 token 大于 0（上游返回 `usage` 时）；若模型配了单价则消费为数字，未配单价则消费记 `unknown`（显示 `-`）——两种都是合法状态，不算 bug。
- [ ] 流式之外的请求不阻塞：curl 在常规时间内返回，代理日志无 panic。

异常记录：`totalRequests` 未涨时，记下 curl 的 HTTP 状态码与返回体、前后两次 `/api/stats` 的 `totalRequests`、代理是否在运行（`proxy status`）。

## 5. 清理（必须执行）

1. WebUI 删除测试供应商 `manual-test-0902`（`DELETE /api/profiles/:name`）。
2. Gateway 页点「应用到 Pi」，确认 `pending_count` 回到 0，`models.json` 无残留测试模型。
3. `pi-switch provider list` 确认测试供应商消失；Stats 当天窗口确认无新增异常失败请求。
4. 撤销/作废本次使用的测试 Key（如 Key 专为本次测试创建）。

## 6. Bug 提单字段模板

```text
标题：[manual-test] 一句话现象
环境：版本 / 系统 / WebUI+代理是否均为本次构建
复现步骤：1. … 2. …（精确到按钮/API 与参数，Key 脱敏）
预期：…
实际：…（贴错误原文、截图、发生时间）
现场：preview pending_count / totalRequests 前后值 / doctor 输出（脱敏）
清理状态：已清理 / 未清理（残留了什么）
```
