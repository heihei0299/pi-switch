package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/store"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
)

type profileItem struct {
	name    string
	current bool
	detail  string
}

func (i profileItem) Title() string {
	if i.current {
		return "● " + i.name + "  (current)"
	}
	return "  " + i.name
}
func (i profileItem) Description() string { return i.detail }
func (i profileItem) FilterValue() string { return i.name }

type Model struct {
	cfg            config.PiSwitchConfig
	list           list.Model
	tab            int // 0 profiles, 1 gateway, 2 stats
	statusMsg      string
	statsBrief     string
	gatewayPreview string
	quitting       bool
	width          int
	height         int
}

func New(cfg config.PiSwitchConfig) Model {
	items := []list.Item{}
	cur := ""
	if cfg.Current != nil {
		cur = *cfg.Current
	}
	for name, prof := range cfg.Profiles {
		desc := fmt.Sprintf("api=%s models=%d exposed=%d", prof.API, len(prof.Models), len(prof.ExposedModels))
		items = append(items, profileItem{name: name, current: name == cur, detail: desc})
	}
	if len(items) == 0 {
		items = append(items, profileItem{name: "(no profiles)", detail: "use 'pi-switch provider add' to create"})
	}
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 60, 14)
	l.Title = "Profiles — enter to switch, g=gateway, s=stats, q=quit"
	l.SetShowHelp(true)
	m := Model{cfg: cfg, list: l, tab: 0}
	m.refreshGateway()
	m.refreshStats()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m *Model) refreshGateway() {
	preview := gateway.BuildProposedGatewayEntry(m.cfg)
	if models, ok := preview["models"].([]interface{}); ok {
		m.gatewayPreview = fmt.Sprintf("Gateway %s @ %s:%d  models=%d",
			m.cfg.Settings.ProviderPrefix, m.cfg.Settings.Proxy.Host, m.cfg.Settings.Proxy.Port, len(models))
	} else {
		m.gatewayPreview = fmt.Sprintf("Gateway %s @ %s:%d", m.cfg.Settings.ProviderPrefix, m.cfg.Settings.Proxy.Host, m.cfg.Settings.Proxy.Port)
	}
}

func (m *Model) refreshStats() {
	db, err := store.GetDB()
	if err != nil {
		m.statsBrief = "stats: db unavailable"
		return
	}
	rows, err := db.Query(`SELECT COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COUNT(*), COALESCE(SUM(cost),0) FROM requests WHERE success=1`)
	if err != nil {
		m.statsBrief = "stats: query error"
		return
	}
	defer rows.Close()
	var pt, ct, cnt int64
	var cost float64
	if rows.Next() {
		_ = rows.Scan(&pt, &ct, &cnt, &cost)
	}
	costStr := "-"
	if cnt > 0 && cost != 0 {
		if cost < 0.01 {
			costStr = fmt.Sprintf("$%.4f", cost)
		} else if cost < 1000 {
			costStr = fmt.Sprintf("$%.2f", cost)
		} else {
			costStr = fmt.Sprintf("$%.1fK", cost/1000)
		}
	} else if cnt == 0 {
		costStr = "$0.00"
	}
	cacheRate := "-"
	if pt > 0 {
		var cached int64
		_ = db.QueryRow(`SELECT COALESCE(SUM(cached_tokens),0) FROM requests WHERE success=1`).Scan(&cached)
		if cached == 0 {
			cacheRate = "0.0%"
		} else {
			cacheRate = fmt.Sprintf("%.1f%%", float64(cached)/float64(pt)*100)
		}
	}
	m.statsBrief = fmt.Sprintf("Requests: %d  Tokens: %d in / %d out  Cost: %s  Cache: %s", cnt, pt, ct, costStr, cacheRate)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-8)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "1":
			m.tab = 0
			return m, nil
		case "2":
			m.tab = 1
			return m, nil
		case "3":
			m.tab = 2
			return m, nil
		case "g":
			toPublish := gateway.BuildProposedGatewayEntry(m.cfg)
			if err := gateway.Publish(m.cfg, toPublish); err != nil {
				m.statusMsg = "gateway publish failed: " + err.Error()
			} else {
				m.statusMsg = "gateway published"
				m.refreshGateway()
			}
			return m, nil
		case "s", "r":
			m.refreshStats()
			m.statusMsg = "stats refreshed"
			return m, nil
		case "enter":
			if m.tab == 0 {
				if it, ok := m.list.SelectedItem().(profileItem); ok && it.name != "(no profiles)" {
					cfgPath := configPath()
					cfg := m.cfg
					name := it.name
					cfg.Current = &name
					if err := saveConfig(cfg, cfgPath); err != nil {
						m.statusMsg = "switch failed: " + err.Error()
					} else {
						m.cfg.Current = &name
						newItems := []list.Item{}
						cur := name
						for n, prof := range m.cfg.Profiles {
							desc := fmt.Sprintf("api=%s models=%d exposed=%d", prof.API, len(prof.Models), len(prof.ExposedModels))
							newItems = append(newItems, profileItem{name: n, current: n == cur, detail: desc})
						}
						m.list.SetItems(newItems)
						m.statusMsg = "switched to " + name
					}
				}
			}
			return m, nil
		}
	}
	if m.tab == 0 {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Bye.\n"
	}
	var b strings.Builder
	header := headerStyle.Render("pi-switch TUI — 1:profiles  2:gateway  3:stats  q:quit")
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", 60) + "\n")
	switch m.tab {
	case 0:
		b.WriteString(m.list.View() + "\n")
	case 1:
		b.WriteString(titleStyle.Render("Gateway") + "\n")
		b.WriteString(m.gatewayPreview + "\n\n")
		b.WriteString(helpStyle.Render("Press g to publish gateway (Gateway Publish) → writes models.json:providers[pi-switch]") + "\n")
	case 2:
		b.WriteString(titleStyle.Render("Stats") + "\n")
		b.WriteString(m.statsBrief + "\n\n")
		b.WriteString(helpStyle.Render("Press r/s to refresh. Total cost shows '-' when unknown, else $0.00 / $0.0042 / $12.34 / $1.2K") + "\n")
	}
	if m.statusMsg != "" {
		b.WriteString("\n" + selectedStyle.Render("▶ "+m.statusMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("Keys: ↑/↓ navigate • enter switch profile • g publish • s refresh • 1/2/3 tabs • q quit") + "\n")
	return b.String()
}

func configPath() string {
	if p := os.Getenv("PI_SWITCH_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch-config.json"
	}
	return filepath.Join(home, ".pi-switch", "config.json")
}

func saveConfig(cfg config.PiSwitchConfig, path string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
