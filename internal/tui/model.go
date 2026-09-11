package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/stats"
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
		return "● " + i.name + " (current)"
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
		models, exposed := channelCounts(prof)
		desc := fmt.Sprintf("api=%s models=%d exposed=%d", prof.API, models, exposed)
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
	// The unified generated entry point; nil current means "no published gateway
	// yet", so the count is the config-derived model total.
	preview := gateway.BuildGeneratedPlan(m.cfg, nil).Proposed
	total := 0
	if provs, ok := preview["providers"].(map[string]interface{}); ok {
		for _, pv := range provs {
			if entry, ok := pv.(map[string]interface{}); ok {
				if models, ok := entry["models"].([]interface{}); ok {
					total += len(models)
				}
			}
		}
		m.gatewayPreview = fmt.Sprintf("Gateway @ %s:%d  models=%d",
			m.cfg.Settings.Proxy.Host, m.cfg.Settings.Proxy.Port, total)
	} else {
		m.gatewayPreview = fmt.Sprintf("Gateway @ %s:%d", m.cfg.Settings.Proxy.Host, m.cfg.Settings.Proxy.Port)
	}
}

func (m *Model) refreshStats() {
	service, err := stats.OpenService(m.cfg.Settings.ConversationSource)
	if err != nil {
		m.statsBrief = "stats: db unavailable"
		return
	}
	summary, err := service.Summary(nil)
	if err != nil {
		m.statsBrief = "stats: query error"
		return
	}
	m.statsBrief = formatStatsBrief(summary)
}

// formatStatsBrief is the TUI's only stats responsibility: turning the numbers
// into a display string. The aggregation lives in internal/stats.
func formatStatsBrief(s stats.Summary) string {
	costStr := "-"
	if s.Requests > 0 && s.Cost != nil && *s.Cost != 0 {
		switch {
		case *s.Cost < 0.01:
			costStr = fmt.Sprintf("$%.4f", *s.Cost)
		case *s.Cost < 1000:
			costStr = fmt.Sprintf("$%.2f", *s.Cost)
		default:
			costStr = fmt.Sprintf("$%.1fK", *s.Cost/1000)
		}
	} else if s.Requests == 0 {
		costStr = "$0.00"
	}
	cacheRate := "-"
	if s.PromptTokens > 0 {
		if s.CachedTokens == 0 {
			cacheRate = "0.0%"
		} else {
			cacheRate = fmt.Sprintf("%.1f%%", float64(s.CachedTokens)/float64(s.PromptTokens)*100)
		}
	}
	return fmt.Sprintf("Requests: %d  Tokens: %d in / %d out  Cost: %s  Cache: %s",
		s.Requests, s.PromptTokens, s.CompletionTokens, costStr, cacheRate)
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
			current, err := gateway.ReadCurrent()
			if err != nil {
				m.statusMsg = "gateway publish failed: " + err.Error()
				return m, nil
			}
			// Same enriched generated flow as the WebUI and CLI.
			plan, _ := gateway.BuildEnrichedGeneratedPlan(m.cfg, current)
			if err := gateway.PublishPlan(plan); err != nil {
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
			if m.list.FilterState() == list.Filtering {
				break
			}
			if m.tab == 0 {
				if it, ok := m.list.SelectedItem().(profileItem); ok && it.name != "(no profiles)" {
					cfgPath := config.ResolvePath()
					cfg := m.cfg
					name := it.name
					cfg.Current = &name
					if err := config.SaveAtPath(cfg, cfgPath); err != nil {
						m.statusMsg = "switch failed: " + err.Error()
					} else {
						m.cfg.Current = &name
						newItems := []list.Item{}
						cur := name
						for n, prof := range m.cfg.Profiles {
							models, exposed := channelCounts(prof)
							desc := fmt.Sprintf("api=%s models=%d exposed=%d", prof.API, models, exposed)
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

func channelCounts(prof config.ProviderProfile) (models, exposed int) {
	for _, channel := range prof.Upstreams {
		models += len(channel.Models)
		exposed += len(channel.ExposedModels)
	}
	return models, exposed
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
