package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
)

func TestNativeJSONDraftKeepsInvalidTextAndUndoRedo(t *testing.T) {
	preview := newNativeComplexPreview()
	original := preview.jsonDraft
	tester := ui.NewTester(preview.view, 900, 720)
	if !tester.HasText("Publishing a draft requires explicit confirmation and writes models.json only") {
		t.Fatalf("draft write boundary is not clear: %q", tester.Texts())
	}
	if err := tester.Click("Draft JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("{")
	if preview.jsonDraft != "{" {
		t.Fatalf("selected draft was not replaced: %q", preview.jsonDraft)
	}
	tester.Key(ui.Cmd, ui.KeyZ)
	if preview.jsonDraft != original {
		t.Fatalf("undo restored %q, want %q", preview.jsonDraft, original)
	}
	tester.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if preview.jsonDraft != "{" {
		t.Fatalf("redo restored %q, want invalid draft", preview.jsonDraft)
	}
	if err := tester.Click("Validate Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if preview.jsonDraft != "{" || !tester.HasText("Invalid JSON at line 1, column 2 — draft preserved") {
		t.Fatalf("invalid draft was changed or not reported: %q / %q", preview.jsonDraft, tester.Texts())
	}
}

func TestNativeJSONSyntaxStatusCountsUnicodeColumns(t *testing.T) {
	got := nativeJSONSyntaxStatus("{\"label\":\"你\",\n  \"x\": }")
	want := "Invalid JSON at line 2, column 8 — draft preserved"
	if got != want {
		t.Fatalf("syntax status = %q, want %q", got, want)
	}
}

func TestNativeJSONSyntaxLineRangeUsesRuneOffsets(t *testing.T) {
	raw := "{\"label\":\"你\",\n  \"x\": }"
	start, end, ok := nativeJSONSyntaxLineRange(raw)
	if !ok || string([]rune(raw)[start:end]) != "  \"x\": }" {
		t.Fatalf("syntax line range = %d:%d, %v; want entire invalid line", start, end, ok)
	}
	if start != len([]rune(raw[:strings.LastIndex(raw, "\n")+1])) {
		t.Fatalf("syntax line starts at rune %d; want the line after the newline", start)
	}
	if start, end, ok := nativeJSONSyntaxLineRange(`{"value":`); !ok || start != 0 || end != len([]rune(`{"value":`)) {
		t.Fatalf("unexpected-EOF line range = %d:%d, %v; want the incomplete line", start, end, ok)
	}
}

func TestNativeJSONSyntaxHighlightUsesThemeDanger(t *testing.T) {
	preview := newNativeComplexPreview()
	preview.jsonDraft = `{"x": }`
	preview.jsonStatus = nativeJSONSyntaxStatus(preview.jsonDraft)
	tester := ui.NewTester(preview.view, 900, 720)
	image := tester.Image()
	danger := ui.LightTheme().Danger
	for y := image.Bounds().Min.Y; y < image.Bounds().Max.Y; y++ {
		for x := image.Bounds().Min.X; x < image.Bounds().Max.X; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if uint8(r>>8) == danger.R && uint8(g>>8) == danger.G && uint8(b>>8) == danger.B {
				return
			}
		}
	}
	t.Fatal("invalid JSON editor did not render the error line in the theme danger color")
}

func TestNativeJSONDraftComposesAndCommitsChineseInput(t *testing.T) {
	preview := newNativeComplexPreview()
	tester := ui.NewTester(preview.view, 900, 720)
	if err := tester.Click("Draft JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type(`{"name":"`)
	tester.Compose("ni", 2)
	if !preview.composing || preview.jsonDraft != `{"name":"` {
		t.Fatalf("composition was committed early: composing=%v draft=%q", preview.composing, preview.jsonDraft)
	}
	tester.Type("你\"}")
	if preview.composing || preview.jsonDraft != `{"name":"你"}` {
		t.Fatalf("composition commit = composing %v, draft %q", preview.composing, preview.jsonDraft)
	}
}

func TestNativeGatewayDraftLoadsWithoutSecretsAndUsesSharedValidation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	channelName := "primary"
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{
		"supplier": {
			API:    "openai-completions",
			APIKey: "config-test-secret",
			Upstreams: []config.Upstream{{
				Name:          &channelName,
				API:           "openai-completions",
				BaseURL:       "https://supplier.example/v1",
				APIKey:        "upstream-test-secret",
				Models:        []config.ModelEntry{{ID: "model-a", ContextWindow: 1000, MaxTokens: 100}},
				ExposedModels: []string{"model-a"},
			}},
		},
	}
	if err := config.SaveAtPath(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	originalConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	originalModels := []byte(`{"providers":{"third-party":{"api":"openai-completions","baseUrl":"https://other.example/v1","apiKey":"models-test-secret","models":[{"id":"keep-me"}]}}}`)
	if err := os.WriteFile(modelsPath, originalModels, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	preview := newNativeComplexPreview()
	preview.publishGatewayDraft = func(raw string) {
		preview.gatewayPublishing = true
		updated, err := publishNativeGatewayDraft(raw)
		preview.gatewayPublishing = false
		if err != nil {
			t.Errorf("publish validated Gateway draft: %v", err)
			return
		}
		preview.gateway = updated
		preview.gatewayLoaded = true
		preview.jsonStatus = "Published Gateway draft."
		preview.gatewayPublishMessage = preview.jsonStatus
		preview.jsonDraftPending = 0
	}
	tester := ui.NewTester(preview.view, 900, 1000)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("Load safe Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.jsonDraft, "pi-switch-chat") || !strings.Contains(preview.jsonDraft, "model-a") {
		t.Fatalf("safe generated draft missing Gateway model: %s", preview.jsonDraft)
	}
	validDraft := preview.jsonDraft
	text := strings.Join(tester.Texts(), " ")
	for _, secret := range []string{"config-test-secret", "upstream-test-secret", "models-test-secret", "third-party"} {
		if strings.Contains(preview.jsonDraft, secret) || strings.Contains(text, secret) {
			t.Fatalf("safe draft exposed %q in draft=%q text=%q", secret, preview.jsonDraft, text)
		}
	}
	if err := tester.Click("Validate Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Valid Gateway draft · 1 pending change(s)") {
		t.Fatalf("generated draft did not use shared plan validation: %q", tester.Texts())
	}
	if !tester.HasText("Added provider: pi-switch-chat") || tester.HasText("Removed provider: third-party") {
		t.Fatalf("draft provider diff missing or third-party provider marked for removal: %q", tester.Texts())
	}
	if err := tester.Click("Draft JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.SetClipboard(`{"providers":{"pi-switch-extra":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[]}}}`)
	tester.Key(ui.Cmd, ui.KeyV)
	if err := tester.Click("Validate Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Gateway draft has 2 conflict(s)") || !tester.HasText("unsupported third pi-switch provider") {
		t.Fatalf("shared Gateway business validation did not report the conflict: %q", tester.Texts())
	}
	if err := tester.Click("Publish validated Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if preview.jsonPublishConfirm {
		t.Fatal("conflicting draft opened the publish confirmation")
	}
	unchangedConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedModels, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchangedConfig) != string(originalConfig) || string(updatedModels) != string(originalModels) {
		t.Fatal("conflicting Gateway draft changed config.json or models.json")
	}

	if err := tester.Click("Draft JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.SetClipboard(validDraft)
	tester.Key(ui.Cmd, ui.KeyV)
	if err := tester.Click("Validate Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Valid Gateway draft · 1 pending change(s)") {
		t.Fatalf("resolved Gateway draft did not produce a pending plan: %q", tester.Texts())
	}
	if err := tester.Click("Publish validated Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Publish Gateway draft?") {
		t.Fatalf("draft publish confirmation missing: %q", tester.Texts())
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	updatedModels, err = os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedModels) != string(originalModels) {
		t.Fatal("canceling draft publish changed models.json")
	}
	if err := tester.Click("Publish validated Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Publish"); err != nil {
		t.Fatal(err)
	}
	if preview.gatewayPublishing || !tester.HasText("Published Gateway draft.") {
		t.Fatalf("draft publish status = %q / %q", preview.jsonStatus, tester.Texts())
	}
	published, err := bootstrap.NewGatewayService().ReadCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	providers := published["providers"].(map[string]interface{})
	if _, ok := providers["third-party"]; !ok {
		t.Fatalf("draft publish removed third-party provider: %#v", providers)
	}
	chat := providers["pi-switch-chat"].(map[string]interface{})
	chatModels := chat["models"].([]interface{})
	if len(chatModels) != 1 || chatModels[0].(map[string]interface{})["id"] != "model-a" {
		t.Fatalf("draft publish produced wrong models: %#v", chatModels)
	}
	publishedBytes, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Validate Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Valid Gateway draft · 0 pending change(s)") {
		t.Fatalf("published draft did not become a no-op: %q", tester.Texts())
	}
	if tester.HasText("Added provider: pi-switch-chat") || tester.HasText("Changed provider: pi-switch-chat") {
		t.Fatalf("no-op validation retained a stale provider diff: %q", tester.Texts())
	}
	if err := tester.Click("Publish validated Gateway draft"); err != nil {
		t.Fatal(err)
	}
	if preview.jsonPublishConfirm {
		t.Fatal("no-op draft opened the publish confirmation")
	}
	unchangedModels, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	unchangedConfig, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchangedModels) != string(publishedBytes) || string(unchangedConfig) != string(originalConfig) {
		t.Fatal("valid no-op draft changed config.json or models.json")
	}
}

func TestNativeComplexGatewayAndPackagesSections(t *testing.T) {
	preview := newNativeComplexPreview()
	preview.refreshGateway = func([]domaingateway.GatewaySelection) {}
	tester := ui.NewTester(preview.view, 900, 720)
	if err := tester.Click("Gateway Diff"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Current vs proposed Gateway", "Refresh Gateway Preview", "Refresh to load the current configuration", "explicit confirmation"} {
		if !strings.Contains(strings.Join(tester.Texts(), " "), want) {
			t.Fatalf("Gateway preview is missing %q: %q", want, tester.Texts())
		}
	}

	if err := tester.Click("Packages"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Packages · local package registry", "Import from Pi Agent", "Register package", "Package service is unavailable."} {
		if !tester.HasText(want) {
			t.Fatalf("native package section is missing %q: %q", want, tester.Texts())
		}
	}
}

func TestNativeStatsReadsRealHistoryPagesUnknownFactsWithoutChangingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "requests.db")
	configPath := filepath.Join(t.TempDir(), "config.json")
	sessionsDir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Settings.ConversationSource = "sessionScan"
	if err := config.SaveAtPath(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", sessionsDir)
	t.Setenv("PI_AGENT_SESSIONS", sessionsDir)
	db, err := store.ResetForTest(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	base := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	for i := 0; i < 999; i++ {
		cost := 0.01
		ts := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if err := store.InsertRequest(db, "supplier", fmt.Sprintf("model-%03d", i), true, 100, 50, 0, &cost, fmt.Sprintf("conv-%02d", i%50), 10, ts); err != nil {
			t.Fatal(err)
		}
	}
	unknownTS := base.Add(20 * time.Minute).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO requests(ts,provider,model,success) VALUES (?, ?, ?, 1)`, unknownTS, "supplier", "unknown-model"); err != nil {
		t.Fatal(err)
	}
	oldTS := base.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano)
	if err := store.InsertRequest(db, "old-supplier", "old-model", true, 1, 1, 0, nil, "conv-old", 1, oldTS); err != nil {
		t.Fatal(err)
	}
	session := fmt.Sprintf("{\"type\":\"session\",\"id\":\"session-scan-1\",\"cwd\":\"/tmp/session-scan-project\",\"timestamp\":%q}\n{\"type\":\"message\",\"timestamp\":%q,\"modelId\":\"unknown-model\"}\n", unknownTS, unknownTS)
	sessionPath := filepath.Join(sessionsDir, "session.jsonl")
	if err := os.WriteFile(sessionPath, []byte(session), 0600); err != nil {
		t.Fatal(err)
	}
	var countBefore int
	if err := db.QueryRow(`SELECT count(*) FROM requests`).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}

	filters := newNativeStatsFilters(time.Now())
	sessionsFilters := newNativeSessionsFilters(time.Now())
	filters.requestRange = nativeRange7d
	sessionsFilters.conversationRange = nativeRange7d
	filters.requestFrom = nativeCalendarDay(base)
	filters.requestTo = filters.requestFrom.AddDate(0, 0, 1)
	sessionsFilters.conversationFrom = nativeCalendarDay(base)
	sessionsFilters.conversationTo = sessionsFilters.conversationFrom.AddDate(0, 0, 1)
	first, err := loadNativeStats(0, 0, 10, filters, sessionsFilters)
	if err != nil {
		t.Fatal(err)
	}
	modelKeys := make([]string, len(first.modelRows))
	for i, row := range first.modelRows {
		modelKeys[i] = row.key
	}
	if first.nativeStatsPage.totalRequests != 1000 || first.nativeStatsPage.requestTotal != 1000 || first.nativeStatsPage.okRequests != 1000 || first.nativeStatsPage.failedRequests != 0 || first.nativeStatsPage.avgLatencyMs != 10 || first.nativeStatsPage.totalTokens["input"] != 99900 || first.nativeStatsPage.totalTokens["output"] != 49950 || first.nativeStatsPage.totalTokens["total"] != 149850 || first.nativeStatsPage.cacheHitRate != "0.0%" || first.nativeStatsPage.costUnknown != 1 || first.nativeSessionsPage.conversationSource != "sessionScan" || first.nativeSessionsPage.conversationTotal != 51 || len(first.nativeStatsPage.rows) != 10 || len(first.nativeSessionsPage.conversations) != 10 || len(first.nativeStatsPage.providerRows) != 1 || len(first.nativeStatsPage.modelRows) != 1000 || !sort.StringsAreSorted(modelKeys) || !strings.HasPrefix(first.nativeStatsPage.modelRows[0].label, "model-000 ·") || first.nativeStatsPage.modelRows[len(first.nativeStatsPage.modelRows)-1].label != "unknown-model · requests: 1 · ok: 1 · input: - · output: - · cached: - · cache rate: - · cost: unknown" {
		t.Fatalf("first stats page = %+v", first)
	}
	if first.rows[0].name != "supplier / unknown-model" || first.rows[0].tokens != nil || first.rows[0].cost != nil {
		t.Fatalf("unknown request facts were fabricated: %+v", first.rows[0])
	}
	var unknownConversation *nativeConversation
	for i := range first.nativeSessionsPage.conversations {
		if first.nativeSessionsPage.conversations[i].id == "session-scan-1" {
			unknownConversation = &first.nativeSessionsPage.conversations[i]
			break
		}
	}
	if unknownConversation == nil || !strings.Contains(unknownConversation.label, "session-scan-project") || len(unknownConversation.requests) != 1 || unknownConversation.requests[0] != "supplier / unknown-model · tokens: unknown · cost: unknown" {
		t.Fatalf("request was not attributed to its conversation: %+v", unknownConversation)
	}

	preview := newNativeComplexPreview()
	preview.tab = 2
	preview.stats = first.nativeStatsPage
	preview.sessions = first.nativeSessionsPage
	preview.statsLoaded = true
	preview.statsFilters = filters
	preview.sessionsFilters = sessionsFilters
	preview.sessionOutline.Open.Add("session-scan-1")
	preview.refreshStats = func(requestPage, conversationPage int, statsFilters nativeStatsFilters, sessionsFilters nativeSessionsFilters) {
		loaded, err := loadNativeStats(requestPage, conversationPage, 10, statsFilters, sessionsFilters)
		if err != nil {
			preview.statsError = err.Error()
			return
		}
		preview.stats = loaded.nativeStatsPage
		preview.sessions = loaded.nativeSessionsPage
		preview.statsError = ""
	}
	preview.refreshConversationRequests = func(id string, page int) {
		loaded, err := loadNativeConversationPage(id, page, 10)
		if err != nil {
			t.Errorf("load conversation %s page %d: %v", id, page, err)
			return
		}
		preview.conversationRequests = loaded
	}
	tester := ui.NewTester(preview.view, 900, 1800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if !tester.HasText("Requests: 1000") || !tester.HasText("failed: 0") || !tester.HasText("Tokens: input 99900 · output 49950 · cached - · reasoning - · total 149850 · cache rate: 0.0% · avg latency: 10 ms") || !tester.HasText("Recent requests · page 1 of 100") || !tester.HasText("input: unknown · output: unknown · cached: unknown · reasoning: unknown · cache rate: - · total: unknown · cost: unknown") || !tester.HasText("session-scan-project · 1 requests") {
		t.Fatalf("real stats page missing expected rows: %q", tester.Texts())
	}
	if err := tester.Click("By provider (1)"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("supplier · requests: 1000 · ok: 1000 · failed: 0 · input: 99900 · output: 49950 · cached: - · cache rate: 0.0% · cost: $9.99") {
		t.Fatalf("provider summary missing or incorrect: %q", tester.Texts())
	}
	if err := tester.Click("By model (1000)"); err != nil {
		t.Fatal(err)
	}
	tester.SetSize(900, 2600)
	if !tester.HasText("model-000 · requests: 1 · ok: 1 · input: 100 · output: 50 · cached: - · cache rate: 0.0% · cost: $0.01") {
		t.Fatalf("model summary missing or incorrect: %q", tester.Texts())
	}
	if err := tester.Click("View requests · conv-48"); err != nil {
		t.Fatal(err)
	}
	if preview.conversationRequests.id != "conv-48" {
		t.Fatalf("selected conversation = %q, want conv-48", preview.conversationRequests.id)
	}
	for _, want := range []string{"Conversation request page 1 of 2 · 20 requests", "model-998", "input: 100", "output: 50", "cached: 0", "total: 150", "cost: $0.01"} {
		if !tester.HasText(want) {
			t.Fatalf("full conversation request page missing %q: %q", want, tester.Texts())
		}
	}
	if err := tester.Click("Next conversation request page"); err != nil {
		t.Fatal(err)
	}
	if preview.conversationRequests.page != 1 {
		t.Fatalf("selected conversation request page = %d, want 1", preview.conversationRequests.page)
	}
	if !tester.HasText("Conversation request page 2 of 2 · 20 requests") || !tester.HasText("model-498") {
		t.Fatalf("conversation request details did not page: %q", tester.Texts())
	}
	if err := tester.Click("Next page"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Recent requests · page 2 of 100") || tester.HasText("unknown-model") || !tester.HasText("Conversation page 1 of 6 · 51 conversations") {
		t.Fatalf("request paging changed the conversation range/page: %q", tester.Texts())
	}
	if err := tester.Click("All time"); err != nil {
		t.Fatal(err)
	}
	if preview.stats.requestPage != 0 || preview.stats.totalRequests != 1001 || preview.stats.requestTotal != 1001 || !tester.HasText("Recent requests · page 1 of 101") || preview.sessions.conversationTotal != 51 || !tester.HasText("Conversation page 1 of 6 · 51 conversations") {
		t.Fatalf("request range did not load independently: stats=%+v text=%q", preview.stats, tester.Texts())
	}
	if err := tester.Click("24h"); err != nil {
		t.Fatal(err)
	}
	if preview.stats.totalRequests != 1000 || preview.stats.requestTotal != 1000 || !tester.HasText("Recent requests · page 1 of 100") {
		t.Fatalf("rolling request range = %+v / %q", preview.stats, tester.Texts())
	}
	if err := tester.Click("Full history"); err != nil {
		t.Fatal(err)
	}
	if preview.sessions.conversationTotal != 52 || !tester.HasText("Conversation page 1 of 6 · 52 conversations") || preview.stats.totalRequests != 1000 {
		t.Fatalf("conversation range did not load independently: stats=%+v text=%q", preview.stats, tester.Texts())
	}
	if err := tester.Click("Custom"); err != nil {
		t.Fatal(err)
	}
	if preview.stats.totalRequests != 1000 || preview.stats.requestPage != 0 || !tester.HasText("Request from") || !tester.HasText("Request to") {
		t.Fatalf("custom request range did not load: stats=%+v text=%q", preview.stats, tester.Texts())
	}
	if err := tester.Click("Request to"); err != nil {
		t.Fatal(err)
	}
	nextCustomDay := preview.statsFilters.requestTo.AddDate(0, 0, 1)
	if err := tester.Click(nextCustomDay.Format("January 2, 2006")); err != nil {
		t.Fatal(err)
	}
	if !nativeCalendarDay(preview.statsFilters.requestTo).Equal(nativeCalendarDay(nextCustomDay)) || preview.stats.totalRequests != 1000 {
		t.Fatalf("custom date did not refresh the request window: filters=%+v stats=%+v", preview.statsFilters, preview.stats)
	}
	if err := tester.Click("Next conversation page"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Conversation page 2 of 6 · 52 conversations") || tester.HasText("session-scan-project · 1 requests") || preview.stats.requestPage != 0 {
		t.Fatalf("conversation page did not advance independently: %q", tester.Texts())
	}
	if !tester.HasText("Conversation request page 2 of 2 · 20 requests") {
		t.Fatalf("request summary paging changed the full conversation detail page: %q", tester.Texts())
	}
	unknownDetail, err := loadNativeConversationPage("session-scan-1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if unknownDetail.total != 1 || len(unknownDetail.rows) != 1 {
		t.Fatalf("unknown conversation facts were fabricated: %+v", unknownDetail)
	}
	for _, want := range []string{"unknown-model", "input: unknown", "output: unknown", "total: unknown", "cost: unknown"} {
		if !strings.Contains(unknownDetail.rows[0], want) {
			t.Fatalf("unknown conversation fact %q was fabricated: %+v", want, unknownDetail.rows[0])
		}
	}
	if err := tester.Click("Custom dates"); err != nil {
		t.Fatal(err)
	}
	if preview.sessions.conversationTotal != 51 || !tester.HasText("Conversation from") || !tester.HasText("Conversation to") {
		t.Fatalf("conversation custom range failed: stats=%+v text=%q", preview.stats, tester.Texts())
	}
	if err := tester.Click("Conversation to"); err != nil {
		t.Fatal(err)
	}
	nextConversationDay := preview.sessionsFilters.conversationTo.AddDate(0, 0, 1)
	if err := tester.Click(nextConversationDay.Format("January 2, 2006")); err != nil {
		t.Fatal(err)
	}
	if preview.sessions.conversationTotal != 51 || !nativeCalendarDay(preview.sessionsFilters.conversationTo).Equal(nativeCalendarDay(nextConversationDay)) {
		t.Fatalf("conversation custom date did not filter/refresh: filters=%+v stats=%+v", preview.statsFilters, preview.stats)
	}
	invalidFrom := preview.statsFilters.requestTo.AddDate(0, 0, 1)
	if err := tester.Click("Request from"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(invalidFrom.Format("January 2, 2006")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.statsError, "end date must be on or after start date") || !tester.HasText("Showing the last successful stats snapshot.") {
		t.Fatalf("invalid custom range did not retain and identify the last snapshot: error=%q text=%q", preview.statsError, tester.Texts())
	}
	var countAfter int
	if err := db.QueryRow(`SELECT count(*) FROM requests`).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != countBefore {
		t.Fatalf("stats preview changed request facts: before=%d after=%d", countBefore, countAfter)
	}
	updatedSession, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedSession) != session {
		t.Fatal("stats preview changed the session file")
	}
}

func TestNativeStatsDoesNotCreateMissingHistoryDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "requests.db")
	t.Setenv("PI_SWITCH_DB", path)
	loaded, err := loadNativeStats(0, 0, 10, newNativeStatsFilters(time.Now()), newNativeSessionsFilters(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.requestTotal != 0 || loaded.successRate != "0%" || loaded.cacheHitRate != "-" {
		t.Fatalf("missing history summary = %+v", loaded)
	}
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{stats: loaded.nativeStatsPage, statsLoaded: true}, nativeSessionsState: nativeSessionsState{sessions: loaded.nativeSessionsPage}}
	tester := ui.NewTester(preview.view, 900, 900)
	if !tester.HasText("Tokens: input - · output - · cached - · reasoning - · total - · cache rate: - · avg latency: -") {
		t.Fatalf("missing-history metrics should remain unknown: %q", tester.Texts())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stats preview created a missing request database: stat err=%v", err)
	}
	conversationPage, err := loadNativeConversationPage("conversation", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if conversationPage.total != 0 || len(conversationPage.rows) != 0 {
		t.Fatalf("missing history conversation page = %+v", conversationPage)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("conversation preview created a missing request database: stat err=%v", err)
	}
}

func TestNativeStatsAutoRefreshUsesConfiguredIntervalWithoutOverlap(t *testing.T) {
	for tier, want := range map[int]time.Duration{
		0: 0,
		1: 5 * time.Second,
		2: 30 * time.Second,
		3: 5 * time.Minute,
	} {
		if got := nativeStatsRefreshInterval(tier); got != want {
			t.Errorf("tier %d interval = %v, want %v", tier, got, want)
		}
	}

	var refreshes, requestPage, conversationPage int
	var filters nativeStatsFilters
	var sessionsFilters nativeSessionsFilters
	preview := &nativeComplexPreview{
		tab: 2,
		nativeStatsState: nativeStatsState{
			stats:        nativeStatsPage{requestPage: 2},
			statsFilters: nativeStatsFilters{requestRange: nativeRange7d},
		},
		nativeSessionsState: nativeSessionsState{
			sessions:        nativeSessionsPage{conversationPage: 3},
			sessionsFilters: nativeSessionsFilters{conversationRange: nativeRange24h},
		},
	}
	preview.refreshStats = func(request, conversation int, current nativeStatsFilters, currentSessions nativeSessionsFilters) {
		refreshes++
		requestPage, conversationPage, filters = request, conversation, current
		sessionsFilters = currentSessions
	}
	tester := ui.NewTester(preview.view, 900, 1000)
	if preview.statsRefreshTier != 0 || !tester.HasText("Auto-refresh") {
		t.Fatalf("auto-refresh should be visible and off by default: tier=%d text=%q", preview.statsRefreshTier, tester.Texts())
	}
	if err := tester.Click("5s"); err != nil {
		t.Fatal(err)
	}
	if preview.statsRefreshTier != 1 || preview.statsRefreshAt.IsZero() {
		t.Fatalf("5s interval was not scheduled: tier=%d at=%v", preview.statsRefreshTier, preview.statsRefreshAt)
	}

	preview.statsRefreshAt = time.Now().Add(-time.Second)
	tester.Frame()
	if refreshes != 1 || preview.statsRefreshAt.Before(time.Now()) || requestPage != 2 || conversationPage != 3 || filters != preview.statsFilters || sessionsFilters != preview.sessionsFilters {
		t.Fatalf("due auto-refresh count=%d pages=%d/%d filters=%+v next=%v", refreshes, requestPage, conversationPage, filters, preview.statsRefreshAt)
	}

	preview.statsLoading = true
	preview.statsRefreshAt = time.Now().Add(-time.Second)
	tester.Frame()
	if refreshes != 1 {
		t.Fatalf("auto-refresh overlapped an in-flight load: %d refreshes", refreshes)
	}
	preview.statsLoading = false
	preview.statsRefreshAt = time.Now().Add(-time.Second)
	tester.Frame()
	if refreshes != 2 {
		t.Fatalf("auto-refresh did not resume after loading: %d refreshes", refreshes)
	}

	if err := tester.Click("Off"); err != nil {
		t.Fatal(err)
	}
	preview.statsRefreshAt = time.Now().Add(-time.Second)
	tester.Frame()
	if refreshes != 2 || !preview.statsRefreshAt.IsZero() {
		t.Fatalf("Off did not stop auto-refresh: refreshes=%d next=%v", refreshes, preview.statsRefreshAt)
	}
}

func TestNativeStatsSummaryShowsFailuresAndOnlyKnownUsage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "requests.db")
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Settings.ConversationSource = "off"
	if err := config.SaveAtPath(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	db, err := store.ResetForTest(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	now := time.Now()
	timestamp := now.UTC().Format(time.RFC3339Nano)
	cost := 0.25
	if err := store.InsertRequest(db, "supplier", "ok-model", true, 10, 5, 0, &cost, "", 20, timestamp); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRequest(db, "supplier", "failed-model", false, 100, 50, 0, nil, "", 40, timestamp); err != nil {
		t.Fatal(err)
	}

	filters := newNativeStatsFilters(now)
	sessionsFilters := newNativeSessionsFilters(now)
	filters.requestRange = nativeRangeAll
	sessionsFilters.conversationRange = nativeRangeAll
	loaded, err := loadNativeStats(0, 0, 10, filters, sessionsFilters)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.totalRequests != 2 || loaded.okRequests != 1 || loaded.failedRequests != 1 || loaded.totalTokens["input"] != 10 || loaded.totalTokens["output"] != 5 || loaded.totalTokens["total"] != 15 || loaded.avgLatencyMs != 30 {
		t.Fatalf("stats included failed-request usage or lost failure metrics: %+v", loaded)
	}
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{stats: loaded.nativeStatsPage, statsLoaded: true}, nativeSessionsState: nativeSessionsState{sessions: loaded.nativeSessionsPage}}
	tester := ui.NewTester(preview.view, 900, 900)
	for _, want := range []string{
		"Requests: 2 · successful: 1 (50.0%) · failed: 1 · cost: $0.25 · unknown cost: 0",
		"Tokens: input 10 · output 5 · cached - · reasoning - · total 15 · cache rate: 0.0% · avg latency: 30 ms",
		timestamp + " · supplier / failed-model · failed",
	} {
		if !tester.HasText(want) {
			t.Fatalf("stats summary missing %q: %q", want, tester.Texts())
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM requests`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("stats preview changed request facts: count=%d", count)
	}
}

func TestNativeStatsProviderRequestShareChart(t *testing.T) {
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{stats: nativeStatsPage{providerRows: []nativeStatsRow{
		{key: "alpha", requests: 3},
		{key: "beta", requests: 1},
	}}, statsLoaded: true}}
	tester := ui.NewTester(preview.view, 900, 900)
	if err := tester.Click("Requests by provider (2)"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"alpha · 75% of requests", "beta · 25% of requests"} {
		if !tester.HasText(want) {
			t.Fatalf("provider request share chart missing %q: %q", want, tester.Texts())
		}
	}
}

func TestNativeStatsProviderRequestShareChartWithNoRequests(t *testing.T) {
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{stats: nativeStatsPage{providerRows: []nativeStatsRow{{key: "empty"}}}, statsLoaded: true}}
	tester := ui.NewTester(preview.view, 900, 900)
	if err := tester.Click("Requests by provider (1)"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("No request data.") {
		t.Fatalf("empty provider chart state = %q", tester.Texts())
	}
}

func TestNativeStatsModelSummaryVirtualizesAndNavigates(t *testing.T) {
	models := make([]nativeStatsRow, 1000)
	for i := range models {
		models[i] = nativeStatsRow{
			key:   fmt.Sprintf("model-%04d", i),
			label: fmt.Sprintf("model-%04d · requests: 1 · ok: 1 · input: 100 · output: 50 · cached: - · cache rate: 0.0%% · cost: $0.01", i),
		}
	}
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{stats: nativeStatsPage{modelRows: models}, statsLoaded: true}}
	tester := ui.NewTester(preview.view, 900, 900)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("By model (1000)"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText(models[0].label) || tester.HasText(models[len(models)-1].label) {
		t.Fatalf("model summary list did not virtualize 1,000 rows: %q", tester.Texts())
	}
	if err := tester.Click(models[0].label); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyEnd)
	if preview.modelSelected != len(models)-1 || !tester.HasText(models[len(models)-1].label) {
		t.Fatalf("End did not navigate the virtualized model summary: selected=%d text=%q", preview.modelSelected, tester.Texts())
	}
}

func TestNativeStatsWindowPresets(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 30, 0, 0, time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name  string
		index int
		from  time.Time
	}{
		{name: "today", index: nativeRangeToday, from: today},
		{name: "24h", index: nativeRange24h, from: now.Add(-24 * time.Hour)},
		{name: "7d", index: nativeRange7d, from: now.Add(-7 * 24 * time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			window, err := nativeStatsWindow(tc.index, today, today, now)
			if err != nil || window == nil || window.From != tc.from.UnixMilli() || window.To != now.UnixMilli() {
				t.Fatalf("window = %+v, want [%d,%d)", window, tc.from.UnixMilli(), now.UnixMilli())
			}
		})
	}
	if window, err := nativeStatsWindow(nativeRangeAll, today, today, now); err != nil || window != nil {
		t.Fatalf("all-time window = %+v, want nil", window)
	}
	fromDate := time.Date(2026, 10, 1, 16, 0, 0, 0, time.Local)
	toDate := time.Date(2026, 10, 3, 9, 0, 0, 0, time.Local)
	window, err := nativeStatsWindow(nativeRangeCustom, fromDate, toDate, now)
	if err != nil || window == nil || window.From != time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local).UnixMilli() || window.To != time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local).UnixMilli() {
		t.Fatalf("custom window = %+v, err=%v", window, err)
	}
	if _, err := nativeStatsWindow(nativeRangeCustom, toDate, fromDate, now); err == nil {
		t.Fatal("reversed custom range was accepted")
	}
}

func TestNativeConversationRequestTextOmitsRawErrors(t *testing.T) {
	errorText, provider, model := "provider-error-secret", "supplier", "model-a"
	status, ok := int64(500), false
	text := nativeConversationRequestText(analytics.ConversationRequestDTO{
		Provider: &provider,
		Model:    &model,
		OK:       &ok,
		Status:   &status,
		Error:    &errorText,
	})
	if !strings.Contains(text, "failed (500)") || strings.Contains(text, errorText) {
		t.Fatalf("conversation request status = %q", text)
	}
}

func TestNativeRecentRequestTextShowsTimestampAndStatus(t *testing.T) {
	prompt, completion, cached, reasoning, tokens, cost := int64(10), int64(5), int64(2), int64(3), int64(15), 0.25
	got := nativeRecentRequestText(nativeUsageRow{
		name: "supplier / model-a", timestamp: "2026-10-08T12:00:00Z", status: "failed (502)",
		promptTokens: &prompt, completionTokens: &completion, cachedTokens: &cached, reasoningTokens: &reasoning,
		tokens: &tokens, cacheRate: "20.0%", cost: &cost,
	})
	want := "2026-10-08T12:00:00Z · supplier / model-a · failed (502) · input: 10 · output: 5 · cached: 2 · reasoning: 3 · cache rate: 20.0% · total: 15 · cost: $0.25"
	if got != want {
		t.Fatalf("recent request text = %q, want %q", got, want)
	}
}

func TestNativeGatewayPreviewUsesSharedPlanWithoutWritingOrShowingSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	channelName := "primary"
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{
		"supplier": {
			Upstreams: []config.Upstream{{
				Name:          &channelName,
				API:           "openai-completions",
				BaseURL:       "https://supplier.example/v1",
				APIKey:        "provider-test-secret",
				Models:        []config.ModelEntry{{ID: "model-a", ContextWindow: 1000, MaxTokens: 100}, {ID: "model-b", ContextWindow: 1000, MaxTokens: 100}},
				ExposedModels: []string{"model-a", "model-b"},
			}},
		},
	}
	if err := config.SaveAtPath(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	originalConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	originalModels := []byte(`{"providers":{"third-party":{"api":"openai-completions","baseUrl":"https://other.example/v1","apiKey":"models-test-secret","models":[{"id":"keep-me"}]}}}`)
	if err := os.WriteFile(modelsPath, originalModels, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	loaded, err := loadNativeGatewayPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(loaded.currentProviders, ","), "third-party") || strings.Contains(strings.Join(loaded.proposedProviders, ","), "pi-switch-chat") {
		t.Fatalf("provider summary current=%v proposed=%v", loaded.currentProviders, loaded.proposedProviders)
	}
	if loaded.pendingCount != 0 || loaded.selectedCount != 0 || loaded.proposedModelCount != 0 || len(loaded.candidates) != 2 || loaded.candidates[0].status != "pending" || loaded.candidates[0].selected || loaded.candidates[1].selected {
		t.Fatalf("generated preview = %#v", loaded)
	}
	if _, err := publishNativeGatewaySelection(nil); err == nil {
		t.Fatal("unchanged Gateway plan was published")
	}
	updatedModels, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedModels) != string(originalModels) {
		t.Fatal("read-only Gateway preview changed models.json")
	}
	updatedConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedConfig) != string(originalConfig) {
		t.Fatal("read-only Gateway preview changed config.json")
	}

	preview := newNativeComplexPreview()
	preview.tab = 1
	preview.gateway = loaded
	preview.gatewayLoaded = true
	preview.refreshGateway = func(selections []domaingateway.GatewaySelection) {
		updated, err := loadNativeGatewayPreviewForSelection(selections)
		if err != nil {
			t.Errorf("load selected Gateway preview: %v", err)
			return
		}
		preview.gateway = updated
		preview.gatewaySelectionExplicit = true
	}
	preview.publishGateway = func(selections []domaingateway.GatewaySelection) {
		updated, err := publishNativeGatewaySelection(selections)
		if err != nil {
			t.Errorf("publish selected Gateway preview: %v", err)
			preview.gatewayPublishing = false
			preview.gatewayPublishMessage = "Publish failed: " + err.Error()
			return
		}
		preview.gatewayPublishing = false
		preview.gateway = updated
		preview.gatewayLoaded = true
		preview.gatewaySelectionExplicit = selections != nil
		preview.gatewayPublishMessage = "Published selected Gateway models."
	}
	tester := ui.NewTester(preview.view, 900, 1000)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	text := strings.Join(tester.Texts(), " ")
	for _, want := range []string{"Current:", "third-party", "Proposed:", "model-a · pending", "model-b · pending"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Gateway preview missing %q: %q", want, text)
		}
	}
	if err := tester.Click("Publish selected models"); err != nil {
		t.Fatal(err)
	}
	if preview.gatewayPublishConfirm {
		t.Fatal("unchanged Gateway plan opened the publish confirmation")
	}
	if err := tester.Click("supplier / primary / model-b · pending"); err != nil {
		t.Fatal(err)
	}
	if preview.gateway.selectedCount != 1 || preview.gateway.proposedModelCount != 1 || preview.gateway.pendingCount != 1 || !strings.Contains(strings.Join(preview.gateway.proposedProviders, ","), "pi-switch-chat") || !preview.gateway.candidates[1].selected || preview.gateway.candidates[0].selected {
		t.Fatalf("partial selection preview = %#v", preview.gateway)
	}
	if !tester.HasText("Selected models: 1 / 2 · proposed: 1 · pending: 1") {
		t.Fatalf("selected-subset summary missing: %q", tester.Texts())
	}
	text = strings.Join(tester.Texts(), " ")
	if strings.Contains(text, "provider-test-secret") || strings.Contains(text, "models-test-secret") {
		t.Fatalf("Gateway preview exposed credentials: %q", text)
	}
	if err := tester.Click("Publish selected models"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Publish Gateway changes?") || !tester.HasText("Write 1 selected model(s)") {
		t.Fatalf("publish confirmation missing: %q", tester.Texts())
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	updatedModels, err = os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedModels) != string(originalModels) {
		t.Fatal("canceling Gateway publish changed models.json")
	}
	if err := tester.Click("Publish selected models"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Publish"); err != nil {
		t.Fatal(err)
	}
	if preview.gatewayPublishing || preview.gateway.pendingCount != 0 || !tester.HasText("Published selected Gateway models.") {
		t.Fatalf("confirmed Gateway publish state = %#v / %q", preview.gateway, tester.Texts())
	}
	published, err := bootstrap.NewGatewayService().ReadCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	providers := published["providers"].(map[string]interface{})
	thirdParty := providers["third-party"].(map[string]interface{})
	if thirdParty["apiKey"] != "models-test-secret" {
		t.Fatalf("third-party provider was not preserved: %#v", thirdParty)
	}
	chat := providers["pi-switch-chat"].(map[string]interface{})
	chatModels := chat["models"].([]interface{})
	if len(chatModels) != 1 || chatModels[0].(map[string]interface{})["id"] != "model-b" {
		t.Fatalf("published Gateway subset = %#v", chatModels)
	}
	updatedConfig, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedConfig) != string(originalConfig) {
		t.Fatal("publishing selected Gateway models changed config.json")
	}
	text = strings.Join(tester.Texts(), " ")
	if strings.Contains(text, "provider-test-secret") || strings.Contains(text, "models-test-secret") {
		t.Fatalf("published Gateway view exposed credentials: %q", text)
	}
}

func TestNativeGatewayPublishRejectsConflictWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	channelName, backupChannel := "primary", "backup"
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{
		"supplier": {Upstreams: []config.Upstream{{
			Name:          &channelName,
			API:           "openai-completions",
			BaseURL:       "https://supplier.example/v1",
			Models:        []config.ModelEntry{{ID: "model-a", ContextWindow: 1000, MaxTokens: 100}},
			ExposedModels: []string{"model-a"},
		}}},
		"other": {Upstreams: []config.Upstream{{
			Name:          &backupChannel,
			API:           "openai-completions",
			BaseURL:       "https://other.example/v1",
			Models:        []config.ModelEntry{{ID: "model-a", ContextWindow: 1000, MaxTokens: 100}},
			ExposedModels: []string{"model-a"},
		}}},
	}
	if err := config.SaveAtPath(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	originalModels := []byte(`{"providers":{"third-party":{"api":"openai-completions","baseUrl":"https://other.example/v1","apiKey":"other-secret","models":[{"id":"legacy"}]}}}`)
	if err := os.WriteFile(modelsPath, originalModels, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	_, err := publishNativeGatewaySelection([]domaingateway.GatewaySelection{{Supplier: "supplier", Channel: "primary", Model: "model-a"}, {Supplier: "other", Channel: "backup", Model: "model-a"}})
	if err == nil || !strings.Contains(err.Error(), "duplicate exposed model ids") {
		t.Fatalf("conflicting Gateway publish error = %v", err)
	}
	updated, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != string(originalModels) {
		t.Fatalf("conflicting Gateway publish changed models.json: %s", updated)
	}
}

func TestNativeStatsExportButtonsAndAtomicFileWrite(t *testing.T) {
	format := ""
	preview := &nativeComplexPreview{tab: 2, nativeStatsState: nativeStatsState{exportStats: func(selected string) { format = selected }}}
	tester := ui.NewTester(preview.view, 900, 1000)
	if !tester.HasText("error text and upstream URLs may contain sensitive data") {
		t.Fatalf("export sensitivity warning missing: %q", tester.Texts())
	}
	if err := tester.Click("Export CSV"); err != nil || format != "csv" {
		t.Fatalf("CSV action format=%q err=%v", format, err)
	}
	if err := tester.Click("Export JSON"); err != nil || format != "json" {
		t.Fatalf("JSON action format=%q err=%v", format, err)
	}

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.InsertRequest(db, "supplier", "model-a", true, 10, 5, 2, nil, "conversation", 12, "2026-10-11T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	service := bootstrap.NewSummaryAnalyticsService(db)
	path := filepath.Join(dir, "pi-switch-logs.json")
	if err := os.WriteFile(path, []byte("previous export"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeNativeStatsExport(path, "json", service); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil || !json.Valid(written) || strings.Contains(string(written), "previous export") {
		t.Fatalf("atomic JSON export=%q err=%v", written, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("export permissions=%v", info.Mode().Perm())
	}
	beforeFailure := append([]byte(nil), written...)
	if err := writeNativeStatsExport(path, "xml", service); err == nil {
		t.Fatal("unsupported export format was accepted")
	}
	afterFailure, err := os.ReadFile(path)
	if err != nil || string(afterFailure) != string(beforeFailure) {
		t.Fatalf("failed export changed previous file: %q err=%v", afterFailure, err)
	}
}
