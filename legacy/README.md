# legacy/ — 旧的 Node/TypeScript 实现（已归档，不发布）

这两个目录是 pi-switch 早期 Node/TypeScript 实现，**不属于当前产品**：Go 重写后它们既没有构建入口，也不在 npm 的 `files` 列表（`bin/`、`webui/dist/`、两个 README）内，因此发布包中从来没有它们。

- `legacy/extensions/` — `pi` 扩展入口；它 `import` 下面的 `src/commands.js`
- `legacy/src/` — 旧的命令/core/proxy/stats/presets 实现

归档原因（`docs/architecture-review.md` 的 A5）：`package.json` 的 `pi.extensions` 指向 `./extensions/index.ts`，而该路径不在发布包内，属**悬空入口**——安装者按 manifest 找不到入口。既然这些文件已不发布，保留一个指向不存在文件的字段只会误导，故摘除该字段并把这些目录移出仓库根（用 `git mv`，历史保留）。

## 保留而非删除的理由

`legacy/src/sync.js` 含有**真实的配置加密导出/导入实现**，作用于同一个 `~/.pi-switch/config.json`。服务端对应能力目前是未实现的（`POST /api/config/export|import|restore` 返回 501，见 A7）。将来若要真正实现该功能，这是一份可参考的既有实现——请勿在未读它之前另起一套。

## 不要做的事

- 不要把这两个目录加回 `files` 或 `pi.extensions`：它们不参与构建，Go 版本不依赖它们（`scripts/build-go.sh`、`bin/pi-switch.js` 均不引用）。
- 不要因为"看起来没人用"就删除 `sync.js`：它是上面那条参考价值的载体。
