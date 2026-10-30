package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"reflect"
	"strings"
	"testing"
)

func TestCancelCompletedScanBeforeDeliveryDiscardsResult(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("s")
	event := m.Key("enter")()
	m.Key("esc")
	m.Apply(event)
	if m.Busy || m.Scanned || len(m.Snapshot.Entries) != 0 || !strings.Contains(m.Message, "cancelled") || strings.Contains(m.Message, "waiting") {
		t.Fatal("cancelled scan completion revived sizes or left a waiting notification", m.Message)
	}
	m.navigate(m.Root)
	if m.Scanned {
		t.Fatal("cancelled completion survived in the session cache")
	}
}

func TestCleanupCompletionClearsWaitingMessageAndKeepsFinalReport(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Key("d")
	m.Key("enter") // Schedule only; no cleanup runner is executed by this test.
	m.Key("esc")
	results := []cleanup.Result{{Outcome: cleanup.Skipped, Err: context.Canceled}}
	m.Apply(Event{Kind: "cleanup", Results: results})
	if m.Busy || m.Screen != "results" || strings.Contains(m.Message, "waiting") || !reflect.DeepEqual(m.Results, results) {
		t.Fatal("final cleanup report lost outcomes or still claims to be waiting", m.Message, m.Results)
	}
}

func TestCancelCompletedPreviewBeforeDeliveryReturnsToBrowser(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	before := captureExplanationBrowser(m)
	m.Key("p")
	m.Key("esc")
	m.Apply(Event{Kind: "preview", Preview: cleanup.Preview{Paths: []string{"cancelled output"}}})
	if m.Busy || m.Screen != "list" || !strings.Contains(m.Message, "cancelled") || strings.Contains(m.Message, "waiting") {
		t.Fatal("cancelled preview displayed a late result or left a waiting notification", m.Screen, m.Message)
	}
	if !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("cancelling preview changed selection or scan state")
	}
}

func TestGenericWrapUsesTerminalCellsAndPreservesGraphemes(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
	}{
		{strings.Repeat("я", 20), 20},
		{"e\u0301", 1},
		{"👩‍💻", 2},
	} {
		got := wrap([]string{tc.text}, tc.width)
		if len(got) != 1 || got[0] != tc.text {
			t.Errorf("text that fits %d terminal cells was split: %q", tc.width, got)
		}
	}
	m := &Model{}
	text := strings.Repeat("я", 40)
	got := wrap([]string{m.paint(red, text)}, 20)
	if len(got) != 2 {
		t.Fatal("colored Cyrillic text wrapped at less than terminal width", got)
	}
	for _, line := range got {
		if cells(line) != 20 || !strings.HasPrefix(line, "\x1b[31m") || !strings.HasSuffix(line, reset) {
			t.Fatal("wrapped color or terminal width was lost", line)
		}
	}
}

func TestNarrowConfirmationNeverShowsIndicatorWithoutAnyContent(t *testing.T) {
	for _, width := range []int{12, 16, 20, 32} {
		for height := 8; height <= 12; height++ {
			m := &Model{Screen: "ai-confirm", Width: width, Height: height, NoColor: true}
			view, _ := m.render()
			if view != "Resize" && !strings.Contains(view, "AI EXPLAN") {
				t.Fatalf("%dx%d frame shows scrolling controls without any content:\n%s", width, height, view)
			}
		}
	}
}
