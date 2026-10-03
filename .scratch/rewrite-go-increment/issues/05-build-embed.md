## Question

`webui/dist` 的构建与嵌入一致性应如何保障？`npm run build:webui` 产物的 `embed.FS` 版本一致性校验、`index.html` 回退与 `/assets` 缓存，以及 `all:../../webui/dist` 非法 `embed` 的修正？

需决策：
1. 构建：`npm --prefix webui run build` 的 `dist` 产物（`index.html` + `assets/*`）与 `go build -ldflags "-s -w"` 的 `embed.FS` 的 `//go:embed dist` 在 `webui/embed.go` 的 `FS` 的版本一致性（`git` 的 `dist` 是否提交 vs 构建时生成）
2. 嵌入：`all:../../webui/dist` 非法 `embed`（`embed` 仅允许 `//go:embed dist` 在 `webui` 包内，`server.go` 的 `../../` 越界）已修为 `webui/embed.go` 的 `//go:embed dist`，`server.go` 的 `fs.Sub(webUIFS, "dist")` 与 `ReadFile("dist/index.html")` 的 `fs.Sub` 正确性
3. 回退：`GET /` 的 `index.html` 与 `GET /assets/*` 的 `http.FileServer` 的 `Cache-Control`，`NoRoute` 的 `index.html` 回退（SPA）与 `404` 的区分
4. 一致性校验：`webui/dist` 的 `index.html` 的 `script` 的 `hash` 与 `go` 二进制的 `embed.FS` 的 `modTime` 是否在 `healthz` 或 `GET /api/buildInfo` 中暴露以供 `hard refresh` 校验

## Type

grilling

## Status

resolved

## Blocked by

01

## Answer

**决策总览**：以现有 `webui/embed.go: //go:embed dist` + `internal/server/server.go: fs.Sub(webUIFS,"dist")` 为既成修复，不回退为 `all:../../webui/dist`；两阶段构建与 SPA 回退沿用当前实现，仅补齐 `Cache-Control` 与 `buildInfo` 可观测性，达到 `go vet + go test 66 passed + webui 5` 回归基线可直接 `tdd-implement`。

1. **构建：`dist` 不入库，两阶段强制，版本一致性构建时生成**
   - 产物：`npm --prefix webui run build`（`vite build`）产出 `webui/dist/index.html + assets/index-*.js|css`（`outDir="dist", emptyOutDir:true, base:"/"`，`assets` 文件名含 content hash `index-CbRnp9x6.js`）。`webui/vite.config.ts` 保持 `base:"/"`，`dist` 仅含 `index.html` 与 `assets/`。
   - 嵌入：`go build -ldflags "-s -w" -o bin/pi-switch ./cmd/pi-switch` 编译时将 `webui/dist`  bake 进 Go 二进制（`webui/embed.go: package webui; import "embed"; //go:embed dist; var FS embed.FS`）。`FS` 根为 `webui/` 目录，含子目录 `dist/`。
   - 一致性：`webui/dist/*` 列入 `.gitignore`（`webui/dist/*` + `!webui/dist/.gitkeep`），**不提交** `dist` 到 git，避免 `index-CbRnp9x6.js` 哈希漂移的合并冲突与二进制体积膨胀；发布与 CI 统一执行 `npm run build`（`package.json: build = build:webui && build:go`）保证 `dist` 与 Go 二进制同次生成。`webui/dist/.gitkeep` 占位使 `go vet / go test` 在无 `npm run build:webui` 时仍可通过（`embed.FS` 为空但不报错，`handleWebUIIndex` 回退 placeholder）。
   - 校验：`healthz` 保持 `{"status":"ok"}` 极简，不塞构建信息；一致性由新增 `GET /api/buildInfo` 暴露（见 4），前端 `hard refresh` 对比 `index.html` 的 `script[src]` 哈希与 `embed.FS` 哈希。

2. **嵌入：`all:../../webui/dist` 非法已正，`fs.Sub` 路径规范化**
   - 非法原因：`go:embed` 指令的 pattern 必须位于声明包的目录树内，`../../` 越界编译期报错 `pattern all:../../webui/dist is outside the current package`，`all:` 前缀仍不允许越界；`internal/server/server.go` 内写 `//go:embed` 亦不符合“`webui` 包内持有前端资源”的分层。
   - 正确形态（已在库内）：`webui/embed.go` 持有 `//go:embed dist`（`dist` 为 `webui/` 的子目录，合法），`internal/server/server.go` 仅消费：
     ```go
     import webuiFS "github.com/heihei0299/pi-switch/webui"
     var webUIFS = webuiFS.FS
     ```
     暴露的 `webUIFS` 根为 `webui/`，含 `dist/index.html` 与 `dist/assets/*`。
   - `fs.Sub` 正确性：`sub, err := fs.Sub(webUIFS, "dist")` 返回以 `dist` 为根的 `fs.FS`（含 `index.html` 与 `assets/`）；后续两类读法等价但规范为 Sub 侧：
     - `handleWebUIIndex`: 优先 `fs.ReadFile(sub, "index.html")`（或兼容 `webUIFS.ReadFile("dist/index.html")`），`len>0` 则 `c.Data(200, "text/html; charset=utf-8", data)`，否则回退 placeholder `<!doctype html>... pi-switch WebUI placeholder`（保证 `go test` 无 `dist` 时不 500）。
     - 静态资源：见 3 的 `/assets` 绑定；`server.go` 不再含任何 `//go:embed`，`fs.Sub` 错误仅发生在 `dist` 不存在时（`err != nil` 时跳过 `StaticFS` 注册，`NoRoute` 仍兜底 placeholder）。
   - 约束：`gin` 保持 `TestMode` 隔离，`embed.FS` 的 `modTime` 为零值不依赖文件系统，可测试；`server.go` 的 `../../` 写法永久禁止，`code-review` 以 `grep -r "go:embed.*\.\."` 为回归。

3. **回退：`GET /` 明确 `index.html`，`/assets/*` 强缓存，`NoRoute` 的 SPA 200 与 `/api/*` 404 区分**
   - `GET /` → `handleWebUIIndex`：`200 text/html; charset=utf-8`，`Cache-Control: no-cache, no-store, must-revalidate`（`index.html` 永不缓存，确保脚本哈希更新即时生效）。
   - `GET /assets/*` → 强缓存：`r.StaticFS("/assets", http.FS(assetsSub))` 其中 `assetsSub, _ := fs.Sub(webUIFS, "dist/assets")`；或等价由 `handleWebUIFallback` 的 `tryPaths=["dist/"+clean, clean]` 命中 `dist/assets/*` 时附加 `Cache-Control: public, max-age=31536000, immutable`（`vite` 的 `index-CbRnp9x6.js / index-Dfflu_Hn.css` 为 content hash，可 immutable）。`MIME` 按后缀：`.js→application/javascript, .css→text/css, .json→application/json, .svg→image/svg+xml`。
   - `NoRoute handleWebUIFallback`：
     ```go
     if strings.HasPrefix(c.Request.URL.Path, "/api/") { c.JSON(404, gin.H{"error":"not found"}); return }
     clean := filepath.Clean(strings.TrimPrefix(c.Request.URL.Path, "/"))
     if strings.Contains(clean, "..") { c.JSON(404, gin.H{"error":"not found"}); return }
     for _, tp := range []string{"dist/"+clean, clean} {
         if data, err := webUIFS.ReadFile(tp); err==nil { // 猜 MIME + Cache-Control 同上; c.Data(200,...); return }
     }
     handleWebUIIndex(c) // SPA 回退：200 index.html
     ```
     区分：`GET /api/unknown` → `404 json`（前端 `api.ts: req` 抛 `data.error || statusText`），`GET /unknown-page` 或 `GET /profiles`（前端路由）→ `200 index.html` 由浏览器路由接管。
   - 现有代码的 `r.StaticFS("/assets", http.FS(sub))` 其中 `sub=fs.Sub(webUIFS,"dist")` 会将 `/assets/x` 映射为 `dist/x` 而非 `dist/assets/x`，实测 `assets` 由 `NoRoute tryPaths` 兜底成功；决议保留 `assetsSub` 的显式绑定以让 `gin` 的 `http.FileServer` 正确设置 `immutable` 与 `ETag`，`NoRoute` 保留兜底双保险，不删除。

4. **一致性校验：`GET /api/buildInfo`（+ 可选 `healthz` 扩展字段）暴露 `embed.FS` 哈希供 hard refresh**
   - 不采用 `git` 提交 `dist` 的 hash 校验（与 1 的“不入库”矛盾），改为二进制自描述：
     ```go
     GET /api/buildInfo → {
       status: "ok",
       version: strings.TrimSpace(ldflagsVersion), // -ldflags "-X main.version=$(git rev-parse --short HEAD)" 注入，无注入时为 "dev"
       buildTime: ldflagsBuildTime,               // 同 ldflags 注入 RFC3339
       webui: {
         embedded: bool,          // webUIFS.ReadFile("dist/index.html") 成功且 len>0
         indexHash: hex(sha256(dist/index.html)), // 前 8 位即可，空时 ""
         script:  "/assets/index-CbRnp9x6.js", // 从 index.html 正则提取首个 <script src>
         assetCount: int,         // fs.Glob(dist/assets/*) 计数，测试期为 0 时跳过校验
       }
     }
     ```
     `indexHash` 与 `script` 在 `init()` 时对 `webUIFS` 计算一次缓存，`modTime` 恒零不采用。
   - 前端校验：WebUI 首屏 `fetch("/api/buildInfo")` 与当前 `document.querySelector('script[src^="/assets/"]')` 对比；`indexHash/script` 不一致时顶部提示 “前端与二进制不一致，请 hard refresh 或重新 `npm run build`”。
   - `healthz`（`GET /healthz`, `GET /health`）保持 `{"status":"ok"}` 以兼容探活；`buildInfo` 独立于 `healthz`，避免监控轮询污染。`open` 环境 `go test` 以 `gin.TestMode` + `webUIFS` placeholder 验证：`GET /` 返回 placeholder 或 `dist/index.html` 均 200，`GET /assets/index-*.js` 在有 `dist` 时 200 含 `immutable`，无 `dist` 时 `NoRoute` 回退 `index.html`（`assetCount==0` 时跳过断言）。

   **可测试性**（最高缝 `go test ./...` + `NODE_ENV=test vitest`）：两阶段构建顺序（`vite build` 后 `go build`）、`//go:embed dist` 越界回归（`all:..` 禁止）、`GET /` no-cache vs `/assets/*` immutable、`NoRoute` 的 `API 404 json` 与 `SPA 200 html` 分流、`GET /api/buildInfo` 的 `indexHash/script` 对比均可在 `gin.New()/httptest` 外部行为断言；WebUI 侧 `vitest` 断言 `GatewayPanel` 等与本票无关，保留 `5` 用例基线。
