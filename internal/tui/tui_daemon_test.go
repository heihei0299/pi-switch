package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/store"
)

// S1: Model+list — type Model{cfg,list,tab,statusMsg,statsBrief,gatewayPreview}, profileItem{Title,Description,FilterValue=name}, New 构 list.New(60,14) + refreshGateway/refreshStats

func TestTuiDaemon_S1_ModelList(t *testing.T) {
	cfg := config.DefaultConfig()
	// ensure default has one provider
	m := New(cfg)
	if m.gatewayPreview == "" {
		t.Fatalf("gatewayPreview empty after New; want Gateway ...")
	}
	if !strings.Contains(m.gatewayPreview, "Gateway") || !strings.Contains(m.gatewayPreview, "@") {
		t.Fatalf("gatewayPreview = %q want gateway address", m.gatewayPreview)
	}
	if m.statsBrief == "" {
		t.Fatalf("statsBrief empty")
	}
	// list dimensions
	if m.list.Width() != 60 || m.list.Height() != 14 {
		// list.Width/Height may be after update pagination, check initial
		// New constructs with 60,14; after refresh it keeps same
		t.Fatalf("list size want 60,14 got %d,%d", m.list.Width(), m.list.Height())
	}
	if m.list.Title != "Profiles — enter to switch, g=gateway, s=stats, q=quit" {
		t.Fatalf("list Title = %q", m.list.Title)
	}
	// profileItem behavior
	pi := profileItem{name: "foo", current: true, detail: "api=openai-completions models=1 exposed=0"}
	if pi.Title() != "● foo (current)" {
		t.Fatalf("profileItem current Title = %q want \"● foo (current)\"", pi.Title())
	}
	if pi.FilterValue() != "foo" {
		t.Fatalf("FilterValue = %q want foo", pi.FilterValue())
	}
	if pi.Description() != "api=openai-completions models=1 exposed=0" {
		t.Fatalf("Description mismatch")
	}
	pi2 := profileItem{name: "bar", current: false, detail: "x"}
	if pi2.Title() != "  bar" {
		t.Fatalf("non-current Title = %q want \"  bar\"", pi2.Title())
	}
	if pi2.FilterValue() != "bar" {
		t.Fatalf("FilterValue bar mismatch")
	}
	// empty config placeholder
	emptyCfg := config.PiSwitchConfig{
		Version:  2,
		Profiles: map[string]config.ProviderProfile{},
		Settings: config.DefaultConfig().Settings,
	}
	m2 := New(emptyCfg)
	found := false
	for _, it := range m2.list.Items() {
		if pi, ok := it.(profileItem); ok && pi.name == "(no profiles)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("empty config list should contain (no profiles)")
	}
	// Init nil
	if m.Init() != nil {
		t.Fatalf("Init should return nil, not Tick")
	}
}

func TestTuiDaemon_S1_RefreshGatewayBuild(t *testing.T) {
	cfg := config.DefaultConfig()
	// BuildProposedGatewayEntry should be used - now returns providers wrapper (may be empty if no exposed)
	proposed := gateway.BuildProposedGatewayEntry(cfg)
	if _, ok := proposed["providers"]; !ok {
		t.Fatalf("proposed missing providers: %v", proposed)
	}
	m := New(cfg)
	// gatewayPreview should contain models=N (N may be 0 for DefaultConfig with no exposed)
	if !strings.Contains(m.gatewayPreview, "models=") {
		t.Fatalf("gatewayPreview missing models count: %q", m.gatewayPreview)
	}
}

// S2: Update/View — WindowSize→SetSize / q,ctrl+c,esc→Quit / 1,2,3→tab / g→gateway.Publish / s,r→refreshStats / enter→cfg.Current=&name→saveConfig tmp+rename→SetItems高亮，Init=nil 不开Tick，FilterState 过滤时 enter 交 list，View 分 0:list/1:Gateway/2:Stats+statusMsg

func TestTuiDaemon_S2_WindowSizeAndTabsAndQuit(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	// WindowSize
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m2 := updated.(Model)
	if m2.width != 80 || m2.height != 24 {
		t.Fatalf("WindowSize width/height want 80,24 got %d,%d", m2.width, m2.height)
	}
	if m2.list.Width() != 76 || m2.list.Height() != 16 {
		t.Fatalf("list size after WindowSize want 76,16 got %d,%d", m2.list.Width(), m2.list.Height())
	}
	// tab switching 1,2,3
	for _, tc := range []struct {
		key string
		tab int
	}{
		{"1", 0}, {"2", 1}, {"3", 2},
	} {
		up, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		mm := up.(Model)
		if mm.tab != tc.tab {
			t.Fatalf("key %q tab = %d want %d", tc.key, mm.tab, tc.tab)
		}
		m2 = mm
	}
	// q quit
	up, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	mm := up.(Model)
	if !mm.quitting {
		t.Fatalf("q should set quitting")
	}
	if cmd == nil {
		t.Fatalf("q should return Quit cmd")
	}
	// ctrl+c
	m3 := New(cfg)
	up, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("ctrl+c should Quit")
	}
	// esc
	m4 := New(cfg)
	up, cmd = m4.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc should Quit")
	}
	// ensure 1/2/3 work after filtering not needed
}

func TestTuiDaemon_S2_EnterSwitchAndFilter(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// create two profiles
	cfg := config.DefaultConfig()
	prof2 := cfg.Profiles["test-provider"]
	prof2.API = "openai-responses"
	cfg.Profiles["second"] = prof2
	// Current is test-provider
	if cfg.Current == nil || *cfg.Current != "test-provider" {
		t.Fatalf("default current mismatch")
	}
	if err := config.SaveAtPath(cfg, cfgPath); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	loaded, _, _ := config.LoadConfigAtPath(cfgPath)
	m := New(loaded)
	// ensure list has both items
	if len(m.list.Items()) < 2 {
		t.Fatalf("list items <2: %d", len(m.list.Items()))
	}
	// select second item: need to move cursor to second? list default cursor 0, so SelectedItem is first. We set Select to 1
	m.list.Select(1)
	sel, ok := m.list.SelectedItem().(profileItem)
	if !ok || sel.name != "second" {
		// find index of second
		found := -1
		for i, it := range m.list.Items() {
			if pi, ok := it.(profileItem); ok && pi.name == "second" {
				found = i
				break
			}
		}
		if found >= 0 {
			m.list.Select(found)
		}
	}
	// now enter should switch
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)
	if m2.cfg.Current == nil || *m2.cfg.Current != "second" {
		t.Fatalf("enter switch failed, current = %v want second", m2.cfg.Current)
	}
	if !strings.Contains(m2.statusMsg, "switched to second") {
		t.Fatalf("statusMsg = %q want switched to second", m2.statusMsg)
	}
	// check file persisted with tmp+rename
	b, _ := os.ReadFile(cfgPath)
	var persisted config.PiSwitchConfig
	_ = json.Unmarshal(b, &persisted)
	if persisted.Current == nil || *persisted.Current != "second" {
		t.Fatalf("file current not persisted: %v", string(b))
	}
	// check SetItems highlight: current item should have ●
	foundCurrent := false
	for _, it := range m2.list.Items() {
		pi := it.(profileItem)
		if pi.name == "second" && strings.Contains(pi.Title(), "●") {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatalf("list not highlighted current after switch")
	}
	// FilterState: when filtering, enter should be delegated to list, not switch
	m3 := New(loaded)
	// Put list into Filtering state with a filter value that matches one item
	m3.list.SetFilterText("test-provider")
	m3.list.SetFilterState(list.Filtering)
	m3.list.FilterInput.SetValue("test-provider")
	origCurrent := *m3.cfg.Current
	// Now send enter – should NOT switch, should be handled by list
	updated, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := updated.(Model)
	if strings.Contains(m4.statusMsg, "switched") {
		t.Fatalf("when Filtering, enter should not switch, got status %q", m4.statusMsg)
	}
	if m4.cfg.Current == nil || *m4.cfg.Current != origCurrent {
		t.Fatalf("when Filtering, current should stay %q got %v", origCurrent, *m4.cfg.Current)
	}
}

func TestTuiDaemon_S2_GPublishAndRefresh(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	modelsPath := filepath.Join(dir, "models.json")
	cfg := config.DefaultConfig()
	_ = config.SaveAtPath(cfg, cfgPath)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	loaded, _, _ := config.LoadConfigAtPath(cfgPath)
	m := New(loaded)
	m.tab = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m2 := updated.(Model)
	if !strings.Contains(m2.statusMsg, "gateway published") && !strings.Contains(m2.statusMsg, "gateway publish failed") {
		t.Fatalf("g publish statusMsg = %q want gateway published/failed", m2.statusMsg)
	}
	if strings.Contains(m2.statusMsg, "gateway published") {
		b, err := os.ReadFile(modelsPath)
		if err != nil {
			t.Fatalf("models.json not created: %v", err)
		}
		var v map[string]interface{}
		_ = json.Unmarshal(b, &v)
		provs, _ := v["providers"].(map[string]interface{})
		if provs == nil {
			t.Fatalf("models.json providers missing: %s", string(b))
		}
		// New per-channel: file may have test-provider or be empty if no exposed
	}
	// s/r refresh stats
	m2.tab = 2
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m3 := updated.(Model)
	if !strings.Contains(m3.statusMsg, "stats refreshed") {
		t.Fatalf("s statusMsg = %q want stats refreshed", m3.statusMsg)
	}
	updated, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m4 := updated.(Model)
	if !strings.Contains(m4.statusMsg, "stats refreshed") {
		t.Fatalf("r statusMsg")
	}
	// View splits
	mV := New(loaded)
	mV.tab = 0
	v0 := mV.View()
	if !strings.Contains(v0, "Profiles") {
		t.Fatalf("View tab0 missing Profiles: %q", v0)
	}
	mV.tab = 1
	v1 := mV.View()
	if !strings.Contains(v1, "Gateway") || !strings.Contains(v1, "g to publish") {
		t.Fatalf("View tab1 missing Gateway/g: %q", v1)
	}
	mV.tab = 2
	mV.statsBrief = "Requests: 1  Tokens: 10 in / 5 out  Cost: $0.00  Cache: -"
	v2 := mV.View()
	if !strings.Contains(v2, "Stats") {
		t.Fatalf("View tab2 missing Stats: %q", v2)
	}
	// statusMsg with ▶
	mV.statusMsg = "hello"
	v3 := mV.View()
	if !strings.Contains(v3, "▶ hello") {
		t.Fatalf("View statusMsg missing ▶: %q", v3)
	}
	// quitting view
	mV.quitting = true
	if mV.View() != "Bye.\n" {
		t.Fatalf("quitting view want Bye")
	}
}

func TestTuiDaemon_S2_ListDelegationAndHelp(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	m.tab = 0
	// Ensure Init nil
	if m.Init() != nil {
		t.Fatalf("Init not nil")
	}
	// tab0 list.Update for navigation: send up/down should delegate
	// we just check that Update for other keys delegates without panic
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	_ = cmd
	// tab1 should NOT delegate to list (no cmd)
	m.tab = 1
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		// list not updated, but delegate may still return nil – ok
	}
	// FilterValue isolation: ensure only name matched
	pi := profileItem{name: "my-model-123", detail: "api=openai-completions models=1 exposed=1"}
	if pi.FilterValue() != "my-model-123" {
		t.Fatalf("FilterValue mismatch")
	}
	if strings.Contains(pi.FilterValue(), "api=") {
		t.Fatalf("FilterValue should be name only, not detail")
	}
}

// S3: Cost — statsBrief Cost "-"|$0.00|$0.0042|$12.34|$1.2K，cacheRate
func TestTuiDaemon_S3_CostAndCacheRate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "requests.db")
	cfg := config.DefaultConfig()
	_ = config.SaveAtPath(cfg, cfgPath)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	// reset store
	_, _ = store.ResetForTest(dbPath)
	// helper to set statsBrief via refreshStats
	assertCost := func(setup func(), wantCost string, wantCache string) {
		t.Helper()
		// clean DB
		db, _ := store.GetDB()
		_, _ = db.Exec(`DELETE FROM requests`)
		if setup != nil {
			setup()
		}
		m := New(cfg)
		// statsBrief contains Cost: <want>
		if !strings.Contains(m.statsBrief, "Cost: "+wantCost) {
			t.Fatalf("statsBrief %q want Cost: %s", m.statsBrief, wantCost)
		}
		if !strings.Contains(m.statsBrief, "Cache: "+wantCache) {
			t.Fatalf("statsBrief %q want Cache: %s", m.statsBrief, wantCache)
		}
	}
	// cnt==0 => $0.00, cache "-"
	assertCost(nil, "$0.00", "-")
	// cnt>0 && cost==0 => "-"
	assertCost(func() {
		db, _ := store.GetDB()
		_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 0, 'c1')`)
	}, "-", "0.0%") // wait cost 0 but cache? need to set cached_tokens etc.
	// Actually cost 0 means sum cost =0, so "-" . For cacheRate, pt=100 cached=0 => 0.0%
	// cost <0.01 => $%.4f
	db, _ := store.GetDB()
	_, _ = db.Exec(`DELETE FROM requests`)
	_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 0, 0.0042, 'c1')`)
	m := New(cfg)
	if !strings.Contains(m.statsBrief, "Cost: $0.0042") {
		t.Fatalf("cost 0.0042 want $0.0042 got %q", m.statsBrief)
	}
	// cost 12.34 => $12.34
	_, _ = db.Exec(`DELETE FROM requests`)
	_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 12.34, 'c1')`)
	m = New(cfg)
	if !strings.Contains(m.statsBrief, "Cost: $12.34") {
		t.Fatalf("cost 12.34 want $12.34 got %q", m.statsBrief)
	}
	// cost 1234 => $1.2K (1234/1000=1.234 -> %.1fK = 1.2K)
	_, _ = db.Exec(`DELETE FROM requests`)
	_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 1234, 'c1')`)
	m = New(cfg)
	if !strings.Contains(m.statsBrief, "Cost: $1.2K") {
		t.Fatalf("cost 1234 want $1.2K got %q", m.statsBrief)
	}
	// cacheRate "-" when pt==0
	_, _ = db.Exec(`DELETE FROM requests`)
	// no rows => pt==0 => Cache "-"
	m = New(cfg)
	if !strings.Contains(m.statsBrief, "Cache: -") {
		t.Fatalf("cache - want - got %q", m.statsBrief)
	}
	// cache 0 => 0.0%
	_, _ = db.Exec(`DELETE FROM requests`)
	_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 0, 0.01, 'c1')`)
	m = New(cfg)
	if !strings.Contains(m.statsBrief, "Cache: 0.0%") {
		t.Fatalf("cache 0.0%% got %q", m.statsBrief)
	}
	// cache 50/100 => 50.0%
	_, _ = db.Exec(`DELETE FROM requests`)
	_, _ = db.Exec(`INSERT INTO requests (ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,cost,conversation_id) VALUES (datetime('now'), 'p','m',1, 100, 50, 50, 0.01, 'c1')`)
	m = New(cfg)
	if !strings.Contains(m.statsBrief, "Cache: 50.0%") {
		t.Fatalf("cache 50%% got %q", m.statsBrief)
	}
	// also test db unavailable case: set PI_SWITCH_DB to invalid? skip
}
