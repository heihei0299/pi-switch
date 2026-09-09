# 07: 构建与嵌入基座与 buildInfo 可观测性

**What to build:** 让两阶段构建与 `embed.FS` 嵌入可验证：`npm --prefix webui run build` 产 `dist/index.html + assets/index-*.js|css`（`base=/`）后，`go build -ldflags "-s -w"` 将 `webui/dist` bake 进二进制，`GET /` 与 `GET /assets/*` 按正确 `Cache-Control` 与 SPA/`/api` 404 分流可被 `httptest` 断言，`GET /api/buildInfo` 返回 `version/indexHash/script` 供 hard refresh。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `webui/dist/*` 保持 `.gitignore`（`! .gitkeep` 占位），`npm run build`（`build:webui && build:go`）为发布/CI 唯一路径，`go vet` 在无 `dist` 时仍通过（`handleWebUIIndex` 回退 placeholder）
- [x] `webui/embed.go` 持有 `//go:embed dist`（子目录合法），`internal/server` 仅 `fs.Sub(webUIFS,"dist")` 消费，`grep -r "go:embed.*\.\."` 回归零命中
- [x] `GET /` → `200 text/html; charset=utf-8` 且 `Cache-Control: no-cache, no-store, must-revalidate`；`GET /assets/index-*.js` → `200 application/javascript` 且 `Cache-Control: public, max-age=31536000, immutable`（`assetsSub=fs.Sub(webUIFS,"dist/assets")`）
- [x] `NoRoute` 区分 `GET /api/unknown → 404 {"error":"not found"}` 与 `GET /unknown-page → 200 index.html`（SPA），`..` 路径 `404`
- [x] `GET /api/buildInfo → {status:"ok", version, buildTime, webui:{embedded, indexHash:sha256(dist/index.html), script:"/assets/index-*.js", assetCount}}`（`indexHash` 在 `init()` 缓存，`assetCount==0` 时跳过断言；`healthz` 保持 `{"status":"ok"}` 极简）
- [x] 最高缝 `go test ./... (gin.TestMode, PI_SWITCH_CONFIG_DIR=t.TempDir(), webUIFS 空时 placeholder)` 覆盖上述外部行为；`npm run build:webui && go vet` 门禁通过

## 实施总结
- 提交：`be35838` — `feat(rewrite-go-increment): build embed foundation (#07)`
- 实现的 seams：S1 GET / no-cache html, S2 GET /assets/* immutable, S3 NoRoute SPA200 vs /api404, S4 GET /api/buildInfo
- 验收标准：6/6 全选
- 测试结果：go test ./internal/server -run TestBuildEmbed_07 4/4 全绿；关联 TestHealthz/TestRoot 全绿
- typecheck：go vet ./... 通过
- 文档对齐：webui/dist/.gitkeep 已追踪；构建链路已验证
- 遗留 / 后续建议：无
