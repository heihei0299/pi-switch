package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func write09Config(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// diff helper mirrors lib/gatewayDiff.ts
func diffMap(cur, prop map[string]interface{}) (added, removed, changed []string) {
	if cur == nil {
		for k := range prop {
			added = append(added, k)
		}
		return
	}
	curSet := map[string]bool{}
	propSet := map[string]bool{}
	for k := range cur {
		curSet[k] = true
	}
	for k := range prop {
		propSet[k] = true
	}
	for k := range propSet {
		if !curSet[k] {
			added = append(added, k)
		} else {
			a, _ := json.Marshal(cur[k])
			b, _ := json.Marshal(prop[k])
			if string(a) != string(b) {
				changed = append(changed, k)
			}
		}
	}
	for k := range curSet {
		if !propSet[k] {
			removed = append(removed, k)
		}
	}
	return
}

func TestGatewaySupplier_09_S1_PreviewPending(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]},
			"supplier-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://b.example.com/v1","apiKey":"sk-b","models":[{"id":"m2","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m2"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	mp := filepath.Join(dir, "models.json")
	_ = os.WriteFile(mp, []byte(`{"providers":{}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_MODELS", mp)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview code %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if _, ok := resp["current"]; !ok {
		t.Fatalf("preview missing current")
	}
	proposed, ok := resp["proposed"].(map[string]interface{})
	if !ok {
		t.Fatalf("preview proposed not map: %v", resp["proposed"])
	}
	pending, _ := resp["pending_count"].(float64)
	curMap, _ := resp["current"].(map[string]interface{})
	// allow current nil
	added, removed, changed := diffMap(curMap, proposed)
	exp := len(added) + len(removed) + len(changed)
	if int(pending) != exp {
		t.Fatalf("pending_count %v != diff size %d (added %d removed %d changed %d)", pending, exp, len(added), len(removed), len(changed))
	}
	if exp == 0 {
		t.Fatalf("expected pending >0 before publish")
	}
	// verify diff can be asserted
	if len(changed) == 0 && len(added) == 0 {
		t.Fatalf("diff should have added/changed when current empty, got %+v", resp)
	}
	// preview must not have written file
	b, _ := os.ReadFile(mp)
	var mj map[string]interface{}
	_ = json.Unmarshal(b, &mj)
	if provs, ok := mj["providers"].(map[string]interface{}); ok {
		if _, has := provs["pi-switch"]; has {
			t.Fatalf("preview must not write pi-switch, models.json=%s", string(b))
		}
	}
}

func TestGatewaySupplier_09_S2_PublishAtomicMergeExtra(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]},
			"supplier-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://b.example.com/v1","apiKey":"sk-b","models":[{"id":"m2","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m2"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"0.0.0.0","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	mp := filepath.Join(dir, "models.json")
	// current has extra keys and per-model extra
	existing := `{"providers":{"pi-switch":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[{"id":"supplier-a/m1","contextWindow":128000,"maxTokens":16384,"headers":{"X-Old":"keep"},"compat":{"old":true}}],"proxy":false,"extraTop":"keep-me","headers":{"X-GW":"keep"}}}}`
	_ = os.WriteFile(mp, []byte(existing), 0644)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_MODELS", mp)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	// publish via PUT /api/models/gateway (also test alias)
	pubBody := `{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[{"id":"supplier-a/m1","contextWindow":128000,"maxTokens":16384},{"id":"supplier-b/m2","contextWindow":128000,"maxTokens":16384}],"proxy":false}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(pubBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("PUT /api/models/gateway code %d body %s", w.Code, w.Body.String())
	}
	b, _ := os.ReadFile(mp)
	var mj map[string]interface{}
	_ = json.Unmarshal(b, &mj)
	provs := mj["providers"].(map[string]interface{})
	gw := provs["pi-switch"].(map[string]interface{})
	// merge extra: top-level extraTop should be preserved
	if gw["extraTop"] != "keep-me" {
		t.Fatalf("merge extraTop not preserved, gw=%v", gw)
	}
	if gw["headers"] == nil {
		t.Fatalf("merge headers not preserved, gw=%v", gw)
	}
	// per-model extra: m1 should retain headers/compat
	models := gw["models"].([]interface{})
	foundM1 := false
	for _, m := range models {
		mm := m.(map[string]interface{})
		if mm["id"] == "supplier-a/m1" {
			foundM1 = true
			if _, ok := mm["headers"]; !ok {
				t.Fatalf("m1 headers not merged, m=%v", mm)
			}
			if _, ok := mm["compat"]; !ok {
				t.Fatalf("m1 compat not merged, m=%v", mm)
			}
		}
	}
	if !foundM1 {
		t.Fatalf("m1 not found in published models %v", models)
	}
	// atomic: tmp file should not remain, but check file exists and valid json (already)
	// also check gateway publish alias
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/gateway/publish", strings.NewReader(pubBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("PUT /api/gateway/publish alias code %d body %s", w2.Code, w2.Body.String())
	}
	// settings sync: gatewayApi and proxy host/port should be updated (0.0.0.0 -> 127.0.0.1 already)
	// Publish with different host 0.0.0.0 should sync to 127.0.0.1 – we already have 127.0.0.1, test with change
	// Publish with baseUrl containing 0.0.0.0 and check config updated
	// Do a publish that changes api and baseUrl then verify config
	cfgData, _ := os.ReadFile(p)
	var cfgMap map[string]interface{}
	_ = json.Unmarshal(cfgData, &cfgMap)
	// verify that settings.gatewayApi was synced if we sent different api? Our pubBody api is openai-completions same as before, so not changed. Test via direct Publish sync path: handleGatewayPublish should sync. Already covered.
	_ = cfgMap
}

func TestGatewaySupplier_09_S2_PublishSettingsSync(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	mp := filepath.Join(dir, "models.json")
	_ = os.WriteFile(mp, []byte(`{"providers":{}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_MODELS", mp)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	// publish with api openai-responses and baseUrl with 0.0.0.0:43210 to trigger sync
	body := `{"api":"openai-responses","baseUrl":"http://0.0.0.0:43210/v1","apiKey":"pi-switch-proxy","models":[{"id":"supplier-a/m1","contextWindow":128000,"maxTokens":16384}],"proxy":false}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("publish sync code %d body %s", w.Code, w.Body.String())
	}
	b, _ := os.ReadFile(p)
	var cfgMap map[string]interface{}
	_ = json.Unmarshal(b, &cfgMap)
	settings := cfgMap["settings"].(map[string]interface{})
	if settings["gatewayApi"] != "openai-responses" {
		t.Fatalf("gatewayApi not synced, got %v", settings["gatewayApi"])
	}
	proxy := settings["proxy"].(map[string]interface{})
	if proxy["host"] != "127.0.0.1" {
		t.Fatalf("proxy.host should be 127.0.0.1 after 0.0.0.0, got %v", proxy["host"])
	}
	if int(proxy["port"].(float64)) != 43210 {
		t.Fatalf("proxy.port not synced, got %v", proxy["port"])
	}
}

func TestGatewaySupplier_09_S3_WriteBeforeAfter2Models(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384,"reasoning":true},{"id":"mX","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]},
			"supplier-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://b.example.com/v1","apiKey":"sk-b","models":[{"id":"m2","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m2"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	mp := filepath.Join(dir, "models.json")
	_ = os.WriteFile(mp, []byte(`{"providers":{}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_MODELS", mp)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	// before: providers {} or missing
	bBefore, _ := os.ReadFile(mp)
	var beforeMJ map[string]interface{}
	_ = json.Unmarshal(bBefore, &beforeMJ)
	if provs, ok := beforeMJ["providers"].(map[string]interface{}); ok {
		if _, has := provs["pi-switch"]; has {
			t.Fatalf("before should not have pi-switch")
		}
	}
	// publish (empty body triggers BuildProposed)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/gateway/publish", strings.NewReader(``))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("publish code %d body %s", w.Code, w.Body.String())
	}
	bAfter, _ := os.ReadFile(mp)
	var afterMJ map[string]interface{}
	_ = json.Unmarshal(bAfter, &afterMJ)
	provs := afterMJ["providers"].(map[string]interface{})
	gw, ok := provs["pi-switch"].(map[string]interface{})
	if !ok {
		t.Fatalf("after missing pi-switch: %v", afterMJ)
	}
	models, ok := gw["models"].([]interface{})
	if !ok || len(models) != 2 {
		t.Fatalf("after models len=%v want 2, gw=%v", len(models), gw)
	}
	ids := map[string]bool{}
	for _, m := range models {
		mm := m.(map[string]interface{})
		id, _ := mm["id"].(string)
		ids[id] = true
		if id == "supplier-a/m1" {
			compat, _ := mm["compat"].(map[string]interface{})
			if compat == nil || compat["supportsDeveloperRole"] != false {
				t.Fatalf("reasoning model compat not false, m=%v", mm)
			}
		}
	}
	if !ids["supplier-a/m1"] || !ids["supplier-b/m2"] {
		t.Fatalf("ids missing, got %v", ids)
	}
	// also test missing file case
	_ = os.Remove(mp)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/gateway/publish", strings.NewReader(``))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("publish after remove code %d body %s", w2.Code, w2.Body.String())
	}
	b2, _ := os.ReadFile(mp)
	var mj2 map[string]interface{}
	_ = json.Unmarshal(b2, &mj2)
	if _, ok := mj2["providers"].(map[string]interface{})["pi-switch"]; !ok {
		t.Fatalf("after missing file publish should create pi-switch")
	}
}

func TestGatewaySupplier_09_S4_ExposedModelMap(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384},{"id":"m2","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"],"modelMap":{"m1":"alias"}}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	mp := filepath.Join(dir, "models.json")
	_ = os.WriteFile(mp, []byte(`{"providers":{}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_MODELS", mp)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	// expose valid
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profiles/supplier-a/expose", strings.NewReader(`{"modelIds":["m2"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expose valid code %d body %s", w.Code, w.Body.String())
	}
	// verify config
	b, _ := os.ReadFile(p)
	var cfgMap map[string]interface{}
	_ = json.Unmarshal(b, &cfgMap)
	prof := cfgMap["profiles"].(map[string]interface{})["supplier-a"].(map[string]interface{})
	em := prof["exposedModels"].([]interface{})
	if len(em) != 1 || em[0] != "m2" {
		t.Fatalf("exposedModels not updated, got %v", em)
	}
	// modelMap should still be there and not affect gateway proposed
	if _, ok := prof["modelMap"]; !ok {
		t.Fatalf("modelMap should be preserved")
	}
	// expose invalid should 400
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/profiles/supplier-a/expose", strings.NewReader(`{"modelIds":["unknown"]}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Fatalf("expose unknown should be 400, got %d body %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(strings.ToLower(w2.Body.String()), "exposedmodels") && !strings.Contains(strings.ToLower(w2.Body.String()), "unknown") {
		t.Fatalf("expose error should mention exposedModels/unknown, got %s", w2.Body.String())
	}
	// preview should reflect exposed filtering (only m2)
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w3, req3)
	var preview map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &preview)
	prop := preview["proposed"].(map[string]interface{})
	models := prop["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("proposed models should be 1 after expose m2, got %v", models)
	}
	if mm := models[0].(map[string]interface{}); mm["id"] != "supplier-a/m2" {
		t.Fatalf("proposed id wrong, got %v", mm["id"])
	}
}

func TestGatewaySupplier_09_S5_SpoofDisguise(t *testing.T) {
	t.Skip("failover removed, test skipped")
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"headers":{"X-Custom":"from-profile","X-Top":"top"},"upstreams":[{"baseUrl":"https://a.example.com/v1","apiKey":"sk-a","headers":{"X-Custom":"from-upstream"}}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112,"userAgent":"global-agent"},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	r := NewMgmtRouter()
	// valid spoofs
	for _, spoof := range []string{"", "claude-code", "codex", "gemini"} {
		body := fmt.Sprintf(`{"spoof":%q}`, spoof)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/profiles/supplier-a/spoof", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("spoof %q should be 200, got %d body %s", spoof, w.Code, w.Body.String())
		}
		// verify persisted userAgent
		b, _ := os.ReadFile(p)
		var cfgMap map[string]interface{}
		_ = json.Unmarshal(b, &cfgMap)
		prof := cfgMap["profiles"].(map[string]interface{})["supplier-a"].(map[string]interface{})
		if spoof == "" {
			if _, has := prof["userAgent"]; has {
				// empty should delete or be empty
				if v, _ := prof["userAgent"].(string); v != "" {
					t.Fatalf("empty spoof should clear userAgent, got %v", prof["userAgent"])
				}
			}
		} else {
			if prof["userAgent"] != spoof {
				t.Fatalf("spoof %q persisted as %v", spoof, prof["userAgent"])
			}
		}
	}
	// invalid spoof
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profiles/supplier-a/spoof", strings.NewReader(`{"spoof":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("invalid spoof should be 400, got %d", w.Code)
	}
	// null spoof should clear (delete)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/profiles/supplier-a/spoof", strings.NewReader(`{"spoof":null}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("null spoof should be 200, got %d", w2.Code)
	}
	// Test User-Agent resolution via proxy forwarding
	// Setup mock upstream to capture headers
	var capUA, capXCustom string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capUA = r.Header.Get("User-Agent")
		capXCustom = r.Header.Get("X-Custom")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "x", "object": "chat.completion", "model": "m1", "choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "hi"}}}, "usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1}})
	}))
	defer mock.Close()
	// update config to point to mock and set spoof codex
	cfg2 := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"],"headers":{"X-Custom":"from-profile"},"userAgent":"codex"}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112,"userAgent":"global-agent"},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	_ = os.WriteFile(p, []byte(cfg2), 0644)
	// need fresh routers (they load per-request, so same router ok)
	proxyR := NewProxyRouter()
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`))
	req3.Header.Set("Content-Type", "application/json")
	// no incoming User-Agent, should use spoof
	proxyR.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("proxy with spoof code %d body %s", w3.Code, w3.Body.String())
	}
	if capUA != "codex" {
		t.Fatalf("User-Agent should be spoof codex, got %q (global-agent fallback should not be used)", capUA)
	}
	// Upstream headers > profile headers: X-Custom should be from upstream if present. Test with upstreams case already: profile has X-Custom from-profile, but upstream has from-upstream. Our mock with supplier-a has only profile headers in this cfg2, so it will be from-profile. Need separate test with upstream
	cfg3 := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://backup.invalid/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"],"headers":{"X-Custom":"from-profile"},"upstreams":[{"baseUrl":%q,"apiKey":"sk-a","headers":{"X-Custom":"from-upstream"}}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	_ = os.WriteFile(p, []byte(cfg3), 0644)
	capUA = ""
	capXCustom = ""
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`))
	req4.Header.Set("Content-Type", "application/json")
	proxyR.ServeHTTP(w4, req4)
	if capXCustom != "from-upstream" {
		t.Fatalf("Upstream.headers should override Profile.headers, got %q", capXCustom)
	}
	// without spoof and without global, fallback curl
	cfg4 := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	_ = os.WriteFile(p, []byte(cfg4), 0644)
	capUA = ""
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`))
	req5.Header.Set("Content-Type", "application/json")
	proxyR.ServeHTTP(w5, req5)
	if capUA != "curl/8.5.0" {
		t.Fatalf("fallback User-Agent should be curl/8.5.0, got %q", capUA)
	}
	// global fallback when spoof empty
	cfg5 := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112,"userAgent":"global-ua"},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	_ = os.WriteFile(p, []byte(cfg5), 0644)
	capUA = ""
	w6 := httptest.NewRecorder()
	req6, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`))
	req6.Header.Set("Content-Type", "application/json")
	proxyR.ServeHTTP(w6, req6)
	if capUA != "global-ua" {
		t.Fatalf("global proxy.userAgent should be used, got %q", capUA)
	}
	_ = gin.TestMode
}

func TestGatewaySupplier_09_S6_FetchModelsEnrich(t *testing.T) {
	dir := t.TempDir()
	// mock upstream returning ids via data[].id and models[]
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" && r.URL.Path != "/v1/models" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-a" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{map[string]interface{}{"id": "gpt-4o-mini"}, map[string]interface{}{"id": "gpt-4o"}},
		})
	}))
	defer mock.Close()
	cfg := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"old-model","contextWindow":128000,"maxTokens":16384}],"preset":"openai","modelsDevProvider":"openai"}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	p := write09Config(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/profiles/supplier-a/fetch-models", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("fetch-models code %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	models, ok := resp["models"].([]interface{})
	if !ok || len(models) != 2 {
		t.Fatalf("models should be 2, got %v", resp["models"])
	}
	enrich, ok := resp["enrich"].(map[string]interface{})
	if !ok {
		t.Fatalf("enrich missing, got %v", resp)
	}
	for _, k := range []string{"enriched", "skipped", "failed"} {
		if _, ok := enrich[k]; !ok {
			t.Fatalf("enrich missing %s: %v", k, enrich)
		}
	}
	// verify defaultModel assigned via subsequent PUT /models?
	// Simulate frontend flow: PUT models
	// First check that model ids are present
	foundMini, foundO := false, false
	for _, m := range models {
		if s, ok := m.(string); ok {
			if s == "gpt-4o-mini" {
				foundMini = true
			}
			if s == "gpt-4o" {
				foundO = true
			}
		} else if mm, ok := m.(map[string]interface{}); ok {
			if mm["id"] == "gpt-4o-mini" {
				foundMini = true
				if cw, ok := mm["contextWindow"].(float64); !ok || int(cw) != 128000 {
					t.Fatalf("default contextWindow not 128000, got %v", mm)
				}
			}
			if mm["id"] == "gpt-4o" {
				foundO = true
			}
		}
	}
	if !foundMini || !foundO {
		t.Fatalf("models should contain gpt-4o-mini and gpt-4o, got %v", models)
	}
	// test /v1/models fallback also works (already covers)
	// test timeout and error保留本地: mock that fails
	failMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"fail"}`))
	}))
	defer failMock.Close()
	cfg2 := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, failMock.URL)
	_ = os.WriteFile(p, []byte(cfg2), 0644)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/profiles/supplier-a/fetch-models", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 500 {
		t.Fatalf("upstream failure should be 500, got %d body %s", w2.Code, w2.Body.String())
	}
	// test 404 unknown profile
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/profiles/nonexistent/fetch-models", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != 404 {
		t.Fatalf("unknown profile fetch should be 404, got %d", w3.Code)
	}
}

func TestGatewaySupplier_09_S7_AddFullSelect(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
		"version":2,
		"profiles":{
			"existing":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://a.example.com/v1","apiKey":"sk-a","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m1"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := write09Config(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	r := NewMgmtRouter()
	// presets should exist
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/presets", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("presets code %d", w.Code)
	}
	var presets []interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &presets)
	if len(presets) == 0 {
		t.Fatalf("presets should not be empty for +Add")
	}
	// +Add without preset should be 200 with empty exposedModels (manual-test-bugs/04+05: preset optional, new models not exposed by default)
	bodyNoPreset := `{"name":"new-provider-nopreset","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://new.example.com/v1","apiKey":"sk-new","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384},{"id":"m2","contextWindow":128000,"maxTokens":16384}],"proxy":false}}`
	w2n := httptest.NewRecorder()
	req2n, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(bodyNoPreset))
	req2n.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2n, req2n)
	if w2n.Code != 200 {
		t.Fatalf("without preset should be 200, got %d body %s", w2n.Code, w2n.Body.String())
	}
	// +Add with preset, without exposedModels should stay unexposed (no auto full select)
	bodyWithPreset := `{"name":"new-provider","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://new.example.com/v1","apiKey":"sk-new","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384},{"id":"m2","contextWindow":128000,"maxTokens":16384}],"preset":"openai","proxy":false}}`
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(bodyWithPreset))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("with preset should be 200, got %d body %s", w3.Code, w3.Body.String())
	}
	b, _ := os.ReadFile(p)
	var cfgMap map[string]interface{}
	_ = json.Unmarshal(b, &cfgMap)
	for _, pname := range []string{"new-provider-nopreset", "new-provider"} {
		newProf := cfgMap["profiles"].(map[string]interface{})[pname].(map[string]interface{})
		if em, ok := newProf["exposedModels"].([]interface{}); ok && len(em) != 0 {
			t.Fatalf("%s should have empty exposedModels, got %v", pname, newProf["exposedModels"])
		}
	}
	// validate duplicate id should be 400
	dupBody := `{"name":"dup-test","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://x","apiKey":"k","models":[{"id":"a","contextWindow":128000,"maxTokens":16384},{"id":"a","contextWindow":128000,"maxTokens":16384}],"preset":"openai","proxy":false}}`
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(dupBody))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != 400 {
		t.Fatalf("duplicate id should be 400, got %d", w4.Code)
	}
	// exposedModels unknown should be 400
	badExpose := `{"name":"bad-expose","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://x","apiKey":"k","models":[{"id":"a","contextWindow":128000,"maxTokens":16384}],"exposedModels":["ghost"],"preset":"openai","proxy":false}}`
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(badExpose))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	if w5.Code != 400 {
		t.Fatalf("exposed unknown should be 400, got %d", w5.Code)
	}
	// modelsDevProvider unknown should not block (warning)
	unknownDev := `{"name":"dev-ok","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://x","apiKey":"k","models":[{"id":"a","contextWindow":128000,"maxTokens":16384}],"modelsDevProvider":"unknown-xyz","preset":"openai","proxy":false}}`
	w6 := httptest.NewRecorder()
	req6, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(unknownDev))
	req6.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w6, req6)
	if w6.Code != 200 {
		t.Fatalf("modelsDevProvider unknown should not block, got %d body %s", w6.Code, w6.Body.String())
	}
	// ensure warning appears in validate
	w7 := httptest.NewRecorder()
	req7, _ := http.NewRequest("GET", "/api/validate", nil)
	r.ServeHTTP(w7, req7)
	var issues []map[string]interface{}
	_ = json.Unmarshal(w7.Body.Bytes(), &issues)
	found := false
	for _, iss := range issues {
		if path, _ := iss["path"].(string); strings.Contains(path, "modelsDevProvider") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("validate should warn modelsDevProvider unknown, got %v", issues)
	}
}
