package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/heihei0299/pi-switch/internal/config"
)

func Run() error {
	cfgPath := config.ResolvePath()
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	if len(cfg.Profiles) == 0 && os.Getenv("PI_SWITCH_CONFIG") == "" {
		// still allow TUI with empty config
	}
	m := New(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// RunNonInteractive returns the rendered view without starting tea (for tests).
func RenderView(cfg config.PiSwitchConfig) string {
	m := New(cfg)
	return m.View()
}
