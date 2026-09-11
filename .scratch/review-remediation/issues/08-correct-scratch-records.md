# 08: 修正 `.scratch` 里的两处事实错误

Status: resolved (2026-09-11)

**错误 1**：`known-gaps-remediation/issues/01` 的验收框写「`webui/src/api.ts` 的 `proxyStart` 无调用点（已核）」
——**假**：`webui/src/components/ProxyPanel.tsx:66` 调用它。我当时 grep 了错误的标识符（`startProxy`）。
可达性：清空 host 输入框后点 Start → body `{"host":""}` → 命中该分支。前端 `req()`（`api.ts:75`）对
对象型 `error` 退化用 `res.statusText`，故用户看到 "Not Implemented"，可操作的 message 不可见。

**错误 2**：同批 `progress.md` 写「警告可能误报（只会误报、不会漏报）」——**方向写反**，实测是漏报（见票 01）。

**修**：两处改成准确表述，保留"曾写错"的痕迹而不是删掉；前端读 `error.message` 列为后续项（spec 已记）。

**验收**
- [ ] 两处记录与实际一致，且能看出被更正过
