---
name: functional-test
disable-model-invocation: true
description: "Live functional test with real daemons: isolated HOME, ephemeral ports, mock upstreams, and real proxy/webui to verify supplier/gateway/credits behavior end-to-end. Use when the user says live test / 功能实测 / e2e with real services and wants isolated, reproducible evidence."
---

# Functional Test（功能实测）

`live` + `isolated` 为领衔词的实测编排：用**真实守护进程**而非 mock，在**隔离环境**中跑通完整链路并给出可复现证据。术语以 `CONTEXT.md` 为准，技能设计规则见 `docs/agents/skill-design.md`。本技能是**长程任务**，自带回合连续性与任务分解规则。

## 分支

- **单实例**：用户给出单个待验行为，走 Steps ①→⑤。
- **批量**：`spec.md`/`README` 验收条目或用户列出多条，批量派生实例后按序串行执行（见 Step ③ Chunking）。

## Steps

按序执行，每步达到完成条件才进入下一步；进入任一步前先读取其在本文的定义。

| Step | 做什么 | 完成条件（可验证） | 详规 |
|------|--------|-------------------|------|
| ① 明确实例 | 收集待验实例清单并与用户确认 | 实例列表已固定（命令+预期+校验方式），无歧义 | [§1](#1-明确实例) |
| ② 准备隔离环境 | 创建隔离 `HOME`、随机端口、mock 上游、初始 config/models | 隔离目录已创建且 `cargo check` / `vite build` 通过 | [§2](#2-准备隔离环境) |
| ③ 起服务并跑实例 | 起 proxy/webui 真进程 → 逐实例执行 → 采集 stdout/文件快照 | 每个实例均有运行目录与捕获输出 | [§3](#3-起服务并跑实例) |
| ④ 判定 | 对比实际 vs 预期（文件/接口/文件锁） | 每个实例已标 `PASS/FAIL` 并附证据 | [§4](#4-判定) |
| ⑤ 报告与清理 | 会话内汇总 `PASS m/n`、失败差距与复现目录，清理临时产物 | 汇总已输出，工作区干净（`git status` 无残留） | [§5](#5-报告与清理) |

### 阶段间流转

- 正常流转：出口条件满足即进入下一阶段，不在阶段间停顿。
- 回退路由：③ 中服务起不来 → 回 ② 修端口/依赖；④ 中失败 → 回 ③ 修实现或补用例后重跑。
- 回合连续性：阶段内连续动作（起服务→跑实例→采快照→下一实例）在**一个回合内串行完成**，直至阶段出口；预告下一步后立即执行，不等用户说“继续”。
- 任务分解：单次 `write` 超 ~150 行先写骨架再分批补全；批量 `replace` 超 ~5 处分批执行并验证；批量实例按序串行，不并行。

## 1. 明确实例

### 入口

- 用户已给出待验行为，或指向 `spec.md`/`README`/`docs/adr/` 中的验收条目。

### 操作

1. 收集实例集：
   - 用户显式给出的实例（命令、预期文件/stdout/exit code），或
   - 从 `spec.md`/`README` 验收标准逐条抽取可验证行为（如 `PUT /profiles/:name/spoof` 后 `preview pending` 增加、`GET /api/gateway/health` 返回 `has_models_file`）— 抽取后与用户确认清单。
2. 每个实例必须声明：执行命令、预期文件/接口内容、预期 stdout 关键词、预期 exit code、校验命令（`test -f`/`curl -s`/`grep -q`/`diff`）。
3. 实例列表固定后不再中途新增。

### 出口

- 实例列表已确认且无歧义。

## 2. 准备隔离环境

### 操作

1. **隔离 HOME**：`TMP=$(mktemp -d)`，`export HOME=$TMP`，`mkdir -p $HOME/.pi-switch $HOME/.pi/agent`，写入最小 `config.json` / `models.json`（`providers: {}`），避免污染真实 `~/.pi-switch`。
2. **随机端口**：`proxy_port=$(python -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1])')`，`web_port` 同理；记录到 `env.sh` 供后续步骤引用。
3. **Mock 上游**：按需起轻量 mock（如 `python -m http.server` 或 `node` 小脚本），模拟 `GET /v1/credits` / `GET /v1/models` / `POST /v1/chat/completions`，返回归一化 JSON；端口同样随机，避免硬编码。
4. **构建校验**：`cargo check` + `npm --prefix webui run typecheck` 通过；`npm --prefix webui run build`（如需嵌入 `webui/dist`）按需执行。

### 出口

- 隔离目录已就绪，端口与 mock 已记录，构建校验通过。

## 3. 起服务并跑实例

### 操作

对每个实例按序：

1. **起真服务**（首个实例前一次，后续复用）：
   - `pi-switch proxy start --daemon --host 127.0.0.1 --port $proxy_port`（或 `cargo run -- proxy start`），`pi-switch webui start --daemon --host 127.0.0.1 --port $web_port`；通过 `curl --retry` 探活 `http://127.0.0.1:$web_port/api/state` 与 `http://127.0.0.1:$proxy_port/v1/models`。
   - 探活失败 → 读 `~/.pi-switch/proxy.log` / `webui.log` 定位端口占用或构建缺失，回 ② 修后重起。
2. **执行实例命令**：在隔离 `HOME` 下执行（如 `curl -s http://127.0.0.1:$web_port/api/profiles/:name/credits`、`api.applyGateway`、`config.json` 读写），捕获 `stdout/stderr/exit code` 到 `run-$i/`。
3. **快照**：按实例声明快照 `config.json`、`models.json`、`requests.log`、接口响应体；不并行跑实例，避免端口/文件锁碰撞。

### 出口

- 每个实例均有 `run-$i/` 目录与捕获输出。

## 4. 判定

### 操作

逐实例对比实际 vs 预期：

- 文件存在/内容：`test -f` / `grep -q` / `diff -u`。
- 接口：`curl -s http://127.0.0.1:$web_port/api/... | jq -e '.pending_count > 0'` 等。
- stdout 含预期短语，exit code 匹配。
- 网关/供应商隔离：错侧 400/500 后 `curl` 对侧仍 200。

标 `PASS`/`FAIL` 并附证据（文件路径、stdout 行、diff 片段或 `jq` 输出）。

### 出口

- 每个实例均已 `PASS` 或 `FAIL` 且有证据，无未判定实例。

## 5. 报告与清理

### 操作

1. 会话内汇总：
   - `PASS m/n` 与每实例证据。
   - 失败列差距（预期 vs 实际）与复现目录（`$TMP/run-$i`）。
2. 清理：`pi-switch proxy stop; pi-switch webui stop`（按 `daemon` 记录的 pid），`rm -rf $TMP`（除非 `--keep` 或失败需保留调试）；`git status` 确认工作区干净，无 `[DEBUG-...]` 残留与未跟踪临时文件。

### 出口

- 汇总已输出，临时产物已清理，工作区干净。

## 不做什么

- 不用 mock 替代真守护进程：本技能的价值即 live 链路，mock 仅用于上游模拟。
- 不并行跑实例：串行隔离，避免端口与文件锁竞争。
- 不落盘报告文件：结果仅会话输出，不写 `report-*.md`。
- 不污染真实 HOME：全程在 `HOME=$TMP` 下执行，承诺前先检查端口/依赖可用性。

## 引用

- 术语：`CONTEXT.md`（供应商/上游/网关/网关发布/余量）
- 决策：`docs/adr/0006-supplier-gateway-logical-isolation.md`
- 实例来源：`spec.md` / `README` / `docs/agents/issue-tracker.md`
- 技能设计：`docs/agents/skill-design.md`
- 轻量实例形态（对照）：[instance-test](.pi/skills/instance-test/SKILL.md)
