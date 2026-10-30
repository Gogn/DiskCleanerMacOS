package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"diskcleaner/internal/config"
	"diskcleaner/internal/explain"
	"diskcleaner/internal/policy"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBubbleTeaAdapterAIExplanationAndReturn(t *testing.T) {
	d, err := os.MkdirTemp(".", "adapter-ai-test-")
	if err != nil {
		t.Fatal(err)
	}
	d, _ = filepath.Abs(d)
	defer os.RemoveAll(d)
	path := filepath.Join(d, "highlighted-file")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	guard, err := policy.NewGuard(home, filepath.Join(d, "config"), d, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := New(d, config.Default(), guard).(*model)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.state.Offset = 6
	calls := 0
	m.state.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		calls++
		if request.Path != path {
			t.Fatal("adapter sent the wrong highlighted path", request.Path)
		}
		return "An inferred explanation.\nReview before cleanup.", nil
	}
	_, command := m.Update(tea.KeyPressMsg{Code: 'i'})
	if command != nil || calls != 0 || m.state.Screen != "ai-confirm" {
		t.Fatal("adapter skipped AI confirmation")
	}
	_, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil || !m.state.Busy || calls != 0 {
		t.Fatal("adapter did not schedule an asynchronous AI job")
	}
	m.Update(command())
	if m.state.Screen != "ai-result" || m.state.Busy || calls != 1 || !strings.Contains(m.state.View(), "An inferred explanation.") {
		t.Fatal("adapter did not deliver the AI result")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state.Screen != "list" || m.state.Offset != 6 || m.state.Scanned || len(m.state.Selected) != 0 {
		t.Fatal("adapter AI return changed browser state")
	}
}

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
	if m.state.Input != "" {
		t.Fatal("path input was prefilled before paste")
	}
	m.Update(tea.PasteMsg{Content: d})
	if m.state.Input != d {
		t.Fatal("paste lost")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.state.Root != d || m.state.Screen != "list" {
		t.Fatal("pasted path did not open directly without a scan")
	}
}
