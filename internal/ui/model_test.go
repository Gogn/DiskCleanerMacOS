package ui

import (
	tea "charm.land/bubbletea/v2"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"os"
	"path/filepath"
	"testing"
)

func TestBubbleTeaAdapterScanAndSelection(t *testing.T) {
	d, e := os.MkdirTemp(".", "adapter-test-")
	if e != nil {
		t.Fatal(e)
	}
	d, _ = filepath.Abs(d)
	defer os.RemoveAll(d)
	p := filepath.Join(d, "file")
	os.WriteFile(p, []byte("fixture"), 0600)
	home, _ := os.UserHomeDir()
	g, e := policy.NewGuard(home, filepath.Join(d, "config"), d, nil)
	if e != nil {
		t.Fatal(e)
	}
	m := New(d, config.Default(), g).(*model)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.KeyPressMsg{Code: 's'})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("scan not started")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, command := range batch {
			m.Update(command())
		}
	} else {
		t.Fatal("scan should schedule work and progress updates")
	}
	if !m.state.Scanned {
		t.Fatal(m.state.Message)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if len(m.state.Selected) != 1 {
		t.Fatal("space not recognized")
	}
	m.Update(tea.KeyPressMsg{Code: 'd'})
	if m.state.Screen != "confirm" {
		t.Fatal(m.state.Screen)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state.Screen != "list" || len(m.state.Selected) != 1 {
		t.Fatal("confirmation cancel")
	}
	if !m.View().AltScreen {
		t.Fatal("alternate screen disabled")
	}
	m.Update(tea.KeyPressMsg{Code: 'r'})
	m.state.Input = ""
	m.Update(tea.PasteMsg{Content: d})
	if m.state.Input != d {
		t.Fatal("paste lost")
	}
}
