# pi-switch 基本功能测试手册

> 覆盖供应商、Gateway、请求统计、CLI/API 与独立 Proxy。Desktop 图形界面是 MyGo Native；本项目不再提供浏览器 WebUI。历史命令 `pi-switch webui` 只用于启动管理 API，`GET /` 返回退役提示，管理端点仍在 `/api/*`。
>
> 涉及真实上游时，只使用可撤销的测试凭据和隔离配置；不要把凭据写进文档、截图或 bug 报告。单步结构：操作 → 断言 → 异常记录。

## 0. 前置准备

1. 在临时目录准备隔离配置、数据库、Gateway 文件与 daemon 状态：
   ```bash
   export PI_SWITCH_TEST_DIR="$(mktemp -d)"
   export PI_SWITCH_CONFIG="$PI_SWITCH_TEST_DIR/config.json"
   export PI_SWITCH_CONFIG_DIR="$PI_SWITCH_TEST_DIR"
   export PI_SWITCH_DB="$PI_SWITCH_TEST_DIR/requests.db"
   export PI_SWITCH_MODELS="$PI_SWITCH_TEST_DIR/models.json"
   ```
   `PI_SWITCH_CONFIG_DIR` 只决定 PID/log 目录；配置文件路径由 `PI_SWITCH_CONFIG` 决定。启动服务前设置好所有变量，确保 daemon 子进程继承隔离环境。
2. 启动管理 API（不会打开浏览器）：
   ```bash
   pi-switch webui start --daemon
   pi-switch webui status
   curl -fsS http://127.0.0.1:43110/healthz
   ```
3. 调用 `GET /api/state` 确认测试配置为空；记录 `GET /api/stats` 和 Gateway preview 基线。
4. 如同时验证 Native UI，启动本次构建的 Desktop，使用 `Providers`、`Gateway`、`JSON Editor`、`Stats & Sessions`、`Backups` 等原生页面。关闭 GUI 不会停止独立 Proxy。

## 1. 供应商：添加并测试连接

操作可通过 Native `Providers` 页面或管理 API 完成，例如 `POST /api/profiles` 和 `POST /api/profiles/:name/test`。使用 `manual-test-<日期>` 作为测试名。

```bash
pi-switch provider list
pi-switch provider show manual-test-<日期>
```

断言：

- 测试 profile 出现在 Native UI、API 与 CLI 读结果中。
- 测试连接成功；失败时返回可诊断信息。
- 凭据不得在 CLI 输出、API 响应、日志或截图中明文出现。

## 2. 模型与 Gateway 发布

1. 使用 Native `Providers` 页面或 `POST /api/profiles/:name/fetch-models` 获取模型，并用 profile models API 保存选定模型。
2. 显式 expose 测试模型（Native UI 或 `PUT /api/profiles/:name/expose`）。未 expose 的模型不应进入 Gateway。
3. 在 Native `Gateway` 页面或 `GET /api/models/gateway/preview` 检查 current/proposed 与 pending 变化。
4. 仅在确认后发布（Native UI 或 `PUT /api/models/gateway`），再检查隔离的 `models.json`。
5. 取消 expose 后重复预览与显式发布，确认模型从 Gateway 移除。

断言：

- 预览、取消、冲突或失败不修改 `models.json`；只有显式发布才写入。
- 第三方 provider 保留，测试变更不包含无关 provider。
- `/v1/models` 与请求路由反映发布后的 Gateway 状态。

## 3. 请求与统计

1. 记录 `GET /api/stats` 的 `totalRequests` 基线；Native UI 对应页面为 `Stats & Sessions`。
2. 使用带测试模型的请求经 Proxy 发送一条非流式请求：
   ```bash
   curl -sS http://127.0.0.1:43112/v1/chat/completions \
     -H 'Content-Type: application/json' \
     -d '{"model":"manual-test-<日期>/<channel>/<model>","messages":[{"role":"user","content":"ping"}],"stream":false,"max_tokens":16}'
   ```
3. 再次调用 `GET /api/stats`，并在 Native `Stats & Sessions` 查看最近请求。

断言：

- 成功记录后 `totalRequests` 增加一条；provider/model 归属正确。
- usage 或价格未知时保持 unknown，不得伪造为零。
- Proxy 健康状态、HTTP 响应和请求历史一致。

## 4. 清理

1. 删除隔离 profile（Native UI 或 `DELETE /api/profiles/:name`）。
2. 将 Gateway 恢复到测试前状态并确认 `models.json` 无测试条目。
3. 停止本次启动的测试 Proxy/API；只停止本次隔离目录中身份可核验的进程。
4. 确认保留数据或日志不含测试凭据后，再清理本次创建的临时目录。

## 5. Bug 报告字段

```text
标题：[manual-test] 一句话现象
环境：版本 / 系统 / Native Desktop、CLI/API、Proxy 的构建信息
复现步骤：精确到页面、API 路径或命令与参数；凭据脱敏
预期：…
实际：…（HTTP 状态/错误原文/时间）
现场：隔离配置、Gateway preview 摘要、totalRequests 前后值、Proxy status
清理状态：已清理 / 未清理（说明残留）
```
