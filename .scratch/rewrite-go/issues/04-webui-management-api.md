## Question

Go 重写中 WebUI 前端和管理 API 应如何整合？保留现有 React WebUI 还是重新设计？

pi-switch 现有独立的 React WebUI（vite 构建，rust-embed 嵌入 Rust .node 中），通过 REST API 与后端通信。CLIProxyAPI 有内建的管理控制面板（/management.html）和 /v0/management/* API，面板从 GitHub 自动下载更新。

需要决策：
1. 前端方案：保留现有 React WebUI（适配新 Go API）/ 重写为 Go 模板或内嵌 SPA / 采用 CLIProxyAPI 式的管理面板？
2. 管理 API 设计：是否引入 CLIProxyAPI 式的 /v0/management/* 结构化 API？还是保留现有 WebUI 的 ad-hoc REST 端点？
3. 嵌入方式：CLIProxyAPI 用静态文件服务，pi-switch 现有用 rust-embed，Go 中可用 embed.FS。如何组织前端资源？
4. 功能范围：管理 API 需要暴露哪些操作？（profile CRUD、stats 查询、config 编辑、package 管理、daemon 控制？）
5. 认证：管理 API 和 WebUI 的访问控制如何设计？（现有 pi-switch WebUI 有 basic auth）

## Type

grilling

## Status

resolved

## Answer

1. **前端方案**: 保留现有 React WebUI（vite 构建），仅将后端从 Rust/napi 迁移到 Go/gin，Go 用 `embed.FS` 嵌入 `webui/dist`。
2. **管理 API**: 保留现有 REST 端点，Go 中规范化为 `/api/*` 前缀；前端仅改 baseURL，无需重构调用。
3. **嵌入方式**: `embed.FS` 替代 `rust-embed`，构建流程 `npm run build:webui && go build`。
4. **功能范围**: 覆盖 profile CRUD、stats 查询、config 编辑、package 管理、daemon 控制；与现有 WebUI 1:1 对齐。
5. **认证**: 沿用现有 basic auth（WebUI + 管理 API 共用）。
