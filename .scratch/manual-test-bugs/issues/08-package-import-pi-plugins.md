# 08 — 包管理「从 Pi Agent 导入」无法导入实际 Pi 插件

**现象：** WebUI「包管理 → 从 Pi Agent 导入」无法发现实际安装在 Pi 目录中的插件、扩展、技能或主题，页面显示导入数量为 0。

**复现/现场：**

1. 在 `pi-switch-web` 容器中检查实际 `/root/.pi`：存在 `agent/models.json`、session JSONL 等 Pi 数据，但没有 `/root/.pi/agent/settings.json`。
2. 真实 WebUI 的「包管理」按钮调用 `POST /api/packages/import`。
3. 当前 Go 实现只读取 `~/.pi/agent/settings.json` 的顶层 `packages` 数组；该文件不存在时直接返回：

```json
{"ok":true,"count":0,"message":"no pi agent settings"}
```

4. 即使实际插件以 Pi 支持的目录/包形式存在，只要没有被写进这个 `packages` 数组，也不会进入 pi-switch 的 package DB。

**预期：** 导入应扫描并识别实际 Pi 包生态中已安装的插件/扩展/技能/提示词/主题，导入 package 元数据和能力标记，且 WebUI 显示真实导入数量；没有 settings 文件时不应把“未找到配置”伪装成成功导入 0 个包。

**实际：** `handlePackageImport` 只处理 `~/.pi/agent/settings.json#packages` 字符串数组：

- 不扫描实际 Pi 的扩展、技能、prompt、theme 目录。
- 不读取包 manifest 或能力文件。
- 不复制/登记实际插件内容。
- settings 文件缺失时返回 HTTP 200 和 `ok:true,count:0`，用户无法区分“没有包”和“导入器根本没有扫描到插件”。
- 现有 `package add/install/sync` 仍是数据库元数据桩，不是完整的 Pi 插件安装/同步生命周期。

**根因：** Go 重写只移植了 `settings.json#packages` 的最小字符串导入，尚未移植原 Pi package/plugin 的真实发现与同步语义。仓库既有 `.scratch/rewrite-go/issues/14-tui-release-migration.md` 也明确记录了 package add/install 和真实 `~/.pi/agent/settings.json` 同步仍是遗留项。

**证据：** 本轮 `incus` 实例 `pi-switch-web` 的 `/root/.pi` 目录盘点、Go `handlePackageImport` 路由代码和真实 WebUI/API contract；当前容器没有可直接导入的真实 plugin fixture，因此“实际插件目录被扫描后仍漏导入”还需要用真实 Pi 插件样本完成第二阶段复现，但当前实现的能力缺口已由代码和 settings 缺失响应确定。

**Blocked by:** 需要确认当前 Pi 版本实际使用的插件/包目录约定，并准备一个不含密钥的真实插件 fixture；不阻塞设计和扫描器实现。

**Status:** ready-for-agent

- [ ] 确认 Pi 当前版本的 package/plugin/extension/skill/prompt/theme 目录与 manifest 约定
- [ ] 无 settings 文件时，导入结果明确区分“未发现配置”和“发现 0 个包”
- [ ] 扫描真实 Pi 插件并导入 package 元数据、版本、能力标记
- [ ] package DB 与实际内容/启用状态保持一致，避免只写一行字符串元数据
- [ ] WebUI 真实导入按钮显示可验证的导入结果
- [ ] 用真实 Pi plugin fixture 覆盖 CLI、API 和 WebUI 导入回归

## 建议修复方向

先固定 Pi 版本和真实目录契约，再将发现逻辑抽成可测试的 importer：扫描已支持目录、解析 manifest、归一化 package identity/capabilities，并以事务方式写入 package DB；对缺失 settings 与空 package 列表返回不同的可读状态。最后用隔离 HOME 中的真实 fixture 通过 WebUI「从 Pi Agent 导入」验收，而不是只测试手工插入 `settings.json#packages`。

## Comments

- 这是能力缺口/未完整移植，不应把 `count:0` 当成成功导入。
- 外部真实 Pi 插件样本本轮未在容器中发现，因此具体目录名与 manifest 字段仍需在修复前由目标 Pi 版本确认。
