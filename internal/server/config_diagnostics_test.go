package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/protocol"
)

// 后续审查 P2（a）：整文件门必须按每个 channel 的 effective api/mode 判定，否则能存下
// 一个运行期必然被 translator.PlanRequest 拒绝的配置（channel 显式 api + 不兼容 mode）。
func TestPutConfig_ChecksChannelEffectiveResponsesMode(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	put := func(profiles string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"version":2,"profiles":`+profiles+`}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// channel 自带 openai-completions 却要求 passthrough：请求期必失败 → 400。
	w := put(`{"p":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-completions","responsesMode":"passthrough","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config with a channel-only mismatch = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "upstreams[0]") || !strings.Contains(body, "passthrough") {
		t.Fatalf("rejection must name the channel and the rule: %s", body)
	}

	// profile 未声明 responsesMode（合法，等于 auto）而 channel 配 passthrough：不能因为
	// 「profile 字段不全」整段跳过——effective 组合一样是请求期必失败。
	w = put(`{"p":{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-completions","responsesMode":"passthrough","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config without a profile-level responsesMode = %d, want 400 (the channel pair is still judged): %s", w.Code, w.Body.String())
	}

	// legacy flat profile（无 upstreams）：ResolvedUpstreams 合成的 channel 就是运行期那一个，
	// 它的 effective 组合仍必须被判定。
	w = put(`{"p":{"api":"openai-completions","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config with a legacy flat mismatch = %d, want 400 (the synthesized channel is the runtime's): %s", w.Code, w.Body.String())
	}

	// profile 顶层组合不兼容，即使每个 channel 都覆盖它：门仍拒绝——配置自身声明了一个
	// 不兼容的组合，这条是既有行为（§2.2 第 4 条），也是本门唯一比 runtime 严的地方。
	w = put(`{"p":{"api":"openai-completions","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-responses","responsesMode":"passthrough","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config with a bad profile-level pair = %d, want 400 (the profile's own pair must be self-consistent): %s", w.Code, w.Body.String())
	}

	// channel 未声明 api：回退 profile api（运行期同样如此），不得因此拒绝保存。
	w = put(`{"p":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/config with an api-less channel = %d, want 200 (the door must not be stricter than the runtime): %s", w.Code, w.Body.String())
	}
}

// capabilityWritePolicy 是两个写入口（整文件门、Profile CRUD 门）共用的一份期望表：每个
// 已知 API 能否被写入，由它能否被请求路径执行（protocol.CanProxy）决定。新增 API 时必须
// 在这里显式决定，否则两个 capability matrix 都会以「未覆盖」失败——避免只改 IsKnown 就上线。
var capabilityWritePolicy = map[string]bool{
	protocol.OpenAIChat:         true,
	protocol.OpenAIResponses:    true,
	protocol.AnthropicMessages:  true,
	protocol.GoogleGenerativeAI: false,
}

type capabilityWriteCase struct {
	cap   protocol.APICapability
	allow bool
}

// capabilityWriteCases 把 capability 集合与期望表对齐后交给两个门的矩阵测试。集合大小变了
// （新增或删除 API）、某个 API 没被决策、或决策与 CanProxy 不一致时都在这里失败，所以两个
// 门读的是同一份期望与同一条能力来源，而不是各留一份手写表。
func capabilityWriteCases(t *testing.T) []capabilityWriteCase {
	t.Helper()
	caps := protocol.Capabilities()
	if len(caps) != len(capabilityWritePolicy) {
		t.Fatalf("capability set changed (%d apis); decide the write result for each new api in capabilityWritePolicy", len(caps))
	}
	cases := make([]capabilityWriteCase, 0, len(caps))
	for _, cap := range caps {
		allow, decided := capabilityWritePolicy[cap.ID]
		if !decided {
			t.Fatalf("no write policy for api %q (CanProxy=%v CanGateway=%v); decide it in capabilityWritePolicy", cap.ID, cap.CanProxy, cap.CanGateway)
		}
		if allow != cap.CanProxy {
			t.Fatalf("api %q: write allow=%v, want %v (CanProxy is the one source both write doors follow)", cap.ID, allow, cap.CanProxy)
		}
		cases = append(cases, capabilityWriteCase{cap: cap, allow: allow})
	}
	return cases
}

// FINAL-04：整文件门的 capability regression matrix，逐 API 断言 PUT /api/config 的结果。
func TestPutConfig_CapabilityMatrix(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	for _, tc := range capabilityWriteCases(t) {
		cap := tc.cap
		t.Run(cap.ID, func(t *testing.T) {
			// legacy flat profile：effective api 就是 profile 的 api。
			body := `{"version":2,"profiles":{"p":{"api":"` + cap.ID + `","responsesMode":"` + cap.DefaultMode + `","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}}}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if tc.allow {
				if w.Code != http.StatusOK {
					t.Fatalf("PUT /api/config with api %q = %d, want 200 (proxy-supported APIs must stay writable): %s", cap.ID, w.Code, w.Body.String())
				}
				return
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("PUT /api/config with known-but-unproxyable api %q = %d, want 400: %s", cap.ID, w.Code, w.Body.String())
			}
			msg := w.Body.String()
			if !strings.Contains(msg, cap.ID) || !strings.Contains(msg, "proxy") {
				t.Fatalf("rejection must name the api and the capability: %s", msg)
			}
		})
	}
}

// CLOSE-01：Profile CRUD 门（POST /api/profiles、PUT /api/profiles/:name）必须与整文件门对
// flat profile 给出同一个 capability 判定，所以两个矩阵读同一份 capabilityWriteCases。
func TestProfileCRUD_CapabilityMatrix(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	for _, tc := range capabilityWriteCases(t) {
		cap := tc.cap
		t.Run(cap.ID, func(t *testing.T) {
			prof := `{"api":"` + cap.ID + `","responsesMode":"` + cap.DefaultMode + `","baseUrl":"https://example.test/v1","apiKey":"k"}`
			posted := callMgmt(r, http.MethodPost, "/api/profiles", `{"name":"`+cap.ID+`","profile":`+prof+`}`)
			put := callMgmt(r, http.MethodPut, "/api/profiles/"+cap.ID+"-copy", `{"profile":`+prof+`}`)
			if tc.allow {
				if posted.Code != http.StatusOK {
					t.Fatalf("POST /api/profiles with api %q = %d, want 200 (proxy-supported APIs must stay writable): %s", cap.ID, posted.Code, posted.Body.String())
				}
				if put.Code != http.StatusOK {
					t.Fatalf("PUT /api/profiles/:name with api %q = %d, want 200: %s", cap.ID, put.Code, put.Body.String())
				}
				return
			}
			// 两个门一起判定：POST 走 CreateProfile，PUT 覆写所以直接走 ValidateProfile，
			// 只有两个都拒才算 capability contract 在 CRUD 面上闭合。
			failed := ""
			for _, door := range []struct {
				name string
				w    *httptest.ResponseRecorder
			}{
				{"POST /api/profiles", posted},
				{"PUT /api/profiles/:name", put},
			} {
				if door.w.Code != http.StatusBadRequest {
					failed += fmt.Sprintf("\n%s with known-but-unproxyable api %q = %d, want 400: %s", door.name, cap.ID, door.w.Code, door.w.Body.String())
					continue
				}
				if msg := door.w.Body.String(); !strings.Contains(msg, cap.ID) || !strings.Contains(msg, "proxy") {
					failed += fmt.Sprintf("\n%s rejection must name the api and the capability: %s", door.name, msg)
				}
			}
			if failed != "" {
				t.Fatal(failed)
			}
		})
	}
}

// 门的 capability 判定来自 protocol.IsKnown：CRUD 门也必须拒掉已知集合之外的值，
// 否则 Profile 写入口仍比整文件门宽松（advisory 会把它当历史坏配置报出来）。
func TestProfileCRUD_RejectsUnknownAPI(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()
	prof := `{"api":"unknown-api","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k"}`

	posted := callMgmt(r, http.MethodPost, "/api/profiles", `{"name":"p","profile":`+prof+`}`)
	if posted.Code != http.StatusBadRequest || !strings.Contains(posted.Body.String(), "unsupported api unknown-api") {
		t.Fatalf("POST /api/profiles with an unknown api = %d (%s), want 400 naming it", posted.Code, posted.Body.String())
	}
	put := callMgmt(r, http.MethodPut, "/api/profiles/p", `{"profile":`+prof+`}`)
	if put.Code != http.StatusBadRequest || !strings.Contains(put.Body.String(), "unsupported api unknown-api") {
		t.Fatalf("PUT /api/profiles/:name with an unknown api = %d (%s), want 400 naming it", put.Code, put.Body.String())
	}
}

// FINAL-02：advisory 必须能发现磁盘上已有的不可代理 API（旧配置、手工改盘、降级场景），
// 且旧 flat profile 的路径按 profile 报。
func TestValidate_ReportsKnownButUnproxyableAPI(t *testing.T) {
	isolateConfig(t)
	flat := `{"version":2,"profiles":{"p":{"api":"` + protocol.GoogleGenerativeAI + `","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}}}`
	writeChannelConfig(t, t.TempDir(), flat)

	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config/validate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config/validate = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var issues []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &issues); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range issues {
		path, _ := issue["path"].(string)
		message, _ := issue["message"].(string)
		if path == "profiles.p.api" && strings.Contains(message, protocol.GoogleGenerativeAI) {
			if level, _ := issue["level"].(string); level != "error" {
				t.Fatalf("capability issue level = %q, want error: %v", level, issue)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("validate must report the known-but-unproxyable api at profiles.p.api; got %v", issues)
	}
}

// 预设列表必须与写入口能力一致：不可代理的 api 不再提供预设，避免点选后必然保存失败。
func TestProviderPresets_OnlyOffersProxySupportedAPIs(t *testing.T) {
	presets := ProviderPresets()
	if len(presets) == 0 {
		t.Fatal("preset list is empty")
	}
	seen := map[string]bool{}
	for _, preset := range presets {
		api, _ := preset["api"].(string)
		seen[api] = true
		if !protocol.CanProxy(api) {
			t.Fatalf("preset %v offers api %q, which the write doors reject", preset["id"], api)
		}
	}
	for _, cap := range protocol.Capabilities() {
		if cap.CanProxy != seen[cap.ID] {
			// 允许「可代理但没有预设」（例如将来新增 api 而暂无预设），只禁止反向。
			if seen[cap.ID] {
				t.Fatalf("preset for %q exists but CanProxy=%v", cap.ID, cap.CanProxy)
			}
		}
	}
}

// profile 与 channel 都没有 api：请求期 upstreamFormat("") 必失败，因此是运行期必失败
// 的组合，整文件门必须拒绝（不能当成「无可判定」放过）。
func TestPutConfig_RejectsAChannelPairWithNoAPIAnywhere(t *testing.T) {
	isolateConfig(t)
	body := `{"version":2,"profiles":{"p":{"baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config with no api anywhere = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if msg := w.Body.String(); !strings.Contains(msg, "api is required") {
		t.Fatalf("rejection should say which rule failed: %s", msg)
	}
}

// 后续审查 P2（b）：宽松保存必须配完整 advisory validation —— shape/channel/model
// 诊断整份来自 profile.ProfileIssues，且逐条带字段路径。
func TestValidate_ReportsProfileShapeIssues(t *testing.T) {
	// writeChannelConfig 自己写 PI_SWITCH_CONFIG，所以这里不再单独 isolateConfig。
	// 一个 profile 同时踩多种 shape 问题：channel baseUrl 无 scheme、重复 channel 名、
	// 模型 id 为空、exposedModels 指向池外模型。
	cfg := `{"version":2,"profiles":{"p":{
		"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k",
		"upstreams":[
			{"name":"main","baseUrl":"ftp://example.test","apiKey":"k","api":"openai-completions","models":[{"id":""},{"id":"m1"},{"id":"m1"}],"exposedModels":["ghost"]},
			{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-completions","models":[{"id":"m2"}]}
		]}}}
	`
	writeChannelConfig(t, t.TempDir(), cfg)
	r := NewMgmtRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config/validate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config/validate = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var issues []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &issues); err != nil {
		t.Fatalf("decode issues: %v (%s)", err, w.Body.String())
	}
	paths := map[string]string{}
	for _, issue := range issues {
		path, _ := issue["path"].(string)
		message, _ := issue["message"].(string)
		paths[path] = message
	}
	// 每一种 shape 问题都必须以真实字段路径被报出来（不止第一条）。
	for _, want := range []string{
		"profiles.p.upstreams[0].baseUrl",
		"profiles.p.upstreams[0].models[0].id",
		"profiles.p.upstreams[0].models[2].id",
		"profiles.p.upstreams[0].exposedModels[0]",
		"profiles.p.upstreams[1].name",
	} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("missing issue for %s; got %v", want, paths)
		}
	}
	if msg := paths["profiles.p.upstreams[1].name"]; !strings.Contains(msg, "duplicate upstream name") {
		t.Fatalf("duplicate channel message = %q", msg)
	}
}
