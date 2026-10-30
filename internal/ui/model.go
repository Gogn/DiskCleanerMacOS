package ui

import (
	"charm.land/bubbletea/v2"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/ui/state"
	"os"
	"time"
)

type model struct {
	state       *state.Model
	tickPending bool
}

type progressTick time.Time

func New(root string, settings config.Settings, guard *policy.Guard) tea.Model {
	p, _ := config.Path()
	s := state.New(root, settings, guard, p)
	s.NoColor = os.Getenv("NO_COLOR") != ""
	return &model{state: s}
}
func (m *model) Init() tea.Cmd { return nil }
func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var job state.Job
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.state.Width = msg.Width
		m.state.Height = msg.Height
	case state.Event:
		m.state.Apply(msg)
	case progressTick:
		m.tickPending = false
		m.state.RefreshProgress(time.Time(msg))
	case tea.PasteMsg:
		m.state.Paste(msg.Content)
	case tea.KeyPressMsg:
		job = m.state.Key(msg.String())
	}
	if m.state.Quit {
		return m, tea.Quit
	}
	var commands []tea.Cmd
	if job != nil {
		commands = append(commands, func() tea.Msg { return job() })
	}
	if m.state.Busy && m.state.Screen == "scanning" && !m.tickPending {
		m.tickPending = true
		commands = append(commands, tea.Tick(250*time.Millisecond, func(now time.Time) tea.Msg { return progressTick(now) }))
	}
	return m, tea.Batch(commands...)
}
func (m *model) View() tea.View { v := tea.NewView(m.state.View()); v.AltScreen = true; return v }
