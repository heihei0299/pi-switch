package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

// RED (2): attempts address (profile, channel); channels order by weight,
// explicit weight 0 excludes; per-channel requestRetry overrides global.

func channelProfileJSON(a, b string) string {
	return fmt.Sprintf(`{
		"version":2,"current":"chan-a",
		"profiles":{
			"chan-a":{"api":"openai-completions","responsesMode":"auto",
				"upstreams":[%s,%s],
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["chan-a"]},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, a, b)
}

func chanEntry(url, key string, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"baseUrl":%q,"apiKey":%q%s}`, url, key, extra)
}

func postChat(t *testing.T, router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, model string) int {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w.Code
}

func TestRetry_MultiChannelFailover(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hitsA, hitsB int
	mockA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA++
		if r.Header.Get("Authorization") != "Bearer sk-a" {
			t.Errorf("channel A auth = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(502)
		_, _ = w.Write([]byte(`{"error":"a down"}`))
	}))
	defer mockA.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB++
		if r.Header.Get("Authorization") != "Bearer sk-b" {
			t.Errorf("channel B auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockB.Close()
	cfgPath := writeRetryConfig(t, dir, channelProfileJSON(
		chanEntry(mockA.URL, "sk-a", ""), chanEntry(mockB.URL, "sk-b", "")))
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if code := postChat(t, NewProxyRouter(), "gpt-4o-mini"); code != 200 {
		t.Fatalf("channel failover should succeed, got %d", code)
	}
	if hitsA != 1 || hitsB != 1 {
		t.Fatalf("want A=1 B=1, got A=%d B=%d", hitsA, hitsB)
	}
}

func TestRetry_WeightOrdersChannels(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hitsLight, hitsHeavy int
	ok := func(hits *int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*hits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}
	}
	light := httptest.NewServer(ok(&hitsLight))
	defer light.Close()
	heavy := httptest.NewServer(ok(&hitsHeavy))
	defer heavy.Close()
	// config order is light-first; weight must put heavy first
	cfgPath := writeRetryConfig(t, dir, channelProfileJSON(
		chanEntry(light.URL, "sk-l", `"weight":1`), chanEntry(heavy.URL, "sk-h", `"weight":10`)))
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if code := postChat(t, NewProxyRouter(), "gpt-4o-mini"); code != 200 {
		t.Fatalf("code %d", code)
	}
	if hitsHeavy != 1 || hitsLight != 0 {
		t.Fatalf("weight must order heavy first: heavy=%d light=%d", hitsHeavy, hitsLight)
	}
}

func TestRetry_ZeroWeightExcluded(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hitsZero, hitsNormal int
	mk := func(hits *int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*hits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}))
	}
	zero := mk(&hitsZero)
	defer zero.Close()
	normal := mk(&hitsNormal)
	defer normal.Close()
	cfgPath := writeRetryConfig(t, dir, channelProfileJSON(
		chanEntry(zero.URL, "sk-z", `"weight":0`), chanEntry(normal.URL, "sk-n", "")))
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if code := postChat(t, NewProxyRouter(), "gpt-4o-mini"); code != 200 {
		t.Fatalf("code %d", code)
	}
	if hitsZero != 0 || hitsNormal != 1 {
		t.Fatalf("weight 0 must be excluded: zero=%d normal=%d", hitsZero, hitsNormal)
	}
}

func TestRetry_ChannelRetryOverridesGlobal(t *testing.T) {
	resetRetryStateForTest()
	savedSleep := retrySleep
	retrySleep = func(d time.Duration) {}
	defer func() { retrySleep = savedSleep }()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hits int
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer mock.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"chan-a",
		"profiles":{
			"chan-a":{"api":"openai-completions","responsesMode":"auto",
				"upstreams":[{"baseUrl":%q,"apiKey":"sk-a","requestRetry":2,"disableCooling":true}],
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["chan-a"],"requestRetry":0},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if code := postChat(t, NewProxyRouter(), "gpt-4o-mini"); code == 200 {
		t.Fatal("all-fail should not be 200")
	}
	if hits != 3 {
		t.Fatalf("channel requestRetry=2 must beat global 0: hits=%d want 3", hits)
	}
}

func TestOrderChannels_Unit(t *testing.T) {
	u := func(w uint32) *uint32 { return &w }
	w0, w1, w10 := uint32(0), uint32(1), uint32(10)
	prof := config.ProviderProfile{Upstreams: []config.Upstream{
		{BaseURL: "http://a", Weight: u(w1)},
		{BaseURL: "http://b"},
		{BaseURL: "http://c", Weight: u(w10)},
		{BaseURL: "http://d", Weight: u(w0)},
	}}
	got := orderedChannels(&prof)
	want := []int{2, 0, 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("orderedChannels = %v want %v (weight desc, nil=1, 0 excluded, stable)", got, want)
	}
}

// RED b2: maxCreds budgets distinct PROFILES per round. A has two failing
// channels, B is healthy; with maxCreds=1 B must still be reached.
func TestRetry_MaxCredsBudgetsProfiles(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hitsA1, hitsA2, hitsB int
	fail := func(hits *int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*hits++
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"down"}`))
		}
	}
	mockA1 := httptest.NewServer(fail(&hitsA1))
	defer mockA1.Close()
	mockA2 := httptest.NewServer(fail(&hitsA2))
	defer mockA2.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockB.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"cap-a",
		"profiles":{
			"cap-a":{"api":"openai-completions","responsesMode":"auto",
				"upstreams":[{"baseUrl":%q,"apiKey":"sk-a1"},{"baseUrl":%q,"apiKey":"sk-a2"}],
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]},
			"cap-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-b",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["cap-a","cap-b"],"maxRetryCredentials":1},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mockA1.URL, mockA2.URL, mockB.URL)
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if code := postChat(t, NewProxyRouter(), "gpt-4o-mini"); code != 200 {
		t.Fatalf("B must be reached despite cap-a eating the round-0 budget, got %d (A1=%d A2=%d B=%d)", code, hitsA1, hitsA2, hitsB)
	}
	if hitsB != 1 {
		t.Fatalf("B hits = %d want 1", hitsB)
	}
}

// RED b3: weight-0 channels never take part, including cooldown bookkeeping.
func TestCooldownKeys_SkipExcludedChannels(t *testing.T) {
	prof := config.ProviderProfile{Upstreams: []config.Upstream{
		{BaseURL: "http://keep", APIKey: "k"},
		{BaseURL: "http://drop", APIKey: "k", Weight: &[]uint32{0}[0]},
	}}
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{"p": prof}}
	keys := candidateCooldownKeys([]string{"p"}, &cfg)
	if len(keys) != 1 || keys[0] != cooldownKey("p", "http://keep") {
		t.Fatalf("cooldown keys = %v, want only the kept channel", keys)
	}
}

// RED admit: cooling-skipped profiles must not consume the round budget.
func TestAdmitForRound_Unit(t *testing.T) {
	tried := map[int]map[string]bool{}
	att := func(name string, round int) attempt { return attempt{ref: channelRef{name: name}, round: round} }
	if !admitForRound(tried, att("a", 0), 1) {
		t.Fatal("first profile admits")
	}
	if admitForRound(tried, att("b", 0), 1) {
		t.Fatal("second profile blocked by cap")
	}
	if !admitForRound(tried, att("a", 0), 1) {
		t.Fatal("same profile re-admits (channels share the budget)")
	}
	if !admitForRound(tried, att("b", 1), 1) {
		t.Fatal("budget resets each round")
	}
	if !admitForRound(tried, att("a", 0), 0) {
		t.Fatal("cap 0 means unlimited")
	}
}
