package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"strings"
	"testing"
	"time"
)

func TestLastScanStatusRequiresTimestamp(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 80, 24
	m.Scanned = true
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{}}
	withoutTimestamp := m.View()
	if strings.Contains(withoutTimestamp, "Last scan") {
		t.Fatal("synthetic scan without timestamp showed a last-scan status", withoutTimestamp)
	}

	m.LastScanAt = time.Date(2026, time.October, 30, 14, 32, 0, 0, time.Local)
	withTimestamp := m.View()
	if !strings.Contains(withTimestamp, "Last scan 14:32 | 2 unknown") {
		t.Fatal("timestamped scan status missing", withTimestamp)
	}
	if strings.Contains(withTimestamp, "Selected size") {
		t.Fatal("empty estimate was retained when there is no selection", withTimestamp)
	}
}

func TestLastScanCompletenessCountsAllEntriesNotOnlyVisible(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 80, 24
	m.Entries = []scan.Entry{
		{Path: "/complete", Name: "complete"},
		{Path: "/unknown", Name: ".hidden-unknown"},
		{Path: "/partial", Name: "filtered-partial"},
		{Path: "/excluded", Name: "ignored-excluded"},
		{Path: "/skipped", Name: "skipped"},
	}
	m.Settings.Masks = []string{"ignored-excluded"}
	m.Scanned = true
	m.LastScanAt = time.Date(2026, time.October, 30, 14, 32, 0, 0, time.Local)
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{
		"/complete": {State: scan.Complete},
		"/unknown":  {State: scan.Unknown},
		"/partial":  {State: scan.Partial},
		"/excluded": {State: scan.Skipped},
		"/skipped":  {State: scan.Skipped},
	}}
	m.Filter = "complete"
	view := m.View()
	if !strings.Contains(view, "Last scan 14:32 | 1 unknown | 1 partial") {
		t.Fatal("completeness did not include hidden/filtered entries or ignored entries were counted", view)
	}
}

func TestLastScanStatusFitsTinyBrowserWithSelectionAndFullRow(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 32, 12
	m.Scanned = true
	m.LastScanAt = time.Date(2026, time.October, 30, 14, 32, 0, 0, time.Local)
	m.Selected[m.Entries[0].Path] = cleanup.Target{Path: m.Entries[0].Path}
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{
		m.Entries[0].Path: {State: scan.Partial},
		m.Entries[1].Path: {State: scan.Unknown},
	}}
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.Height {
		t.Fatalf("frame has %d lines, want %d:\n%s", len(lines), m.Height, view)
	}
	if !strings.Contains(view, "Scan 14:32 | Incomplete") {
		t.Fatal("last scan status missing beside selection summary", view)
	}
	fullRowVisible := false
	for i := 0; i+1 < len(lines); i++ {
		if strings.HasPrefix(plainView(lines[i]), "› [x]") && !strings.Contains(plainView(lines[i+1]), "Rows ") {
			fullRowVisible = true
			break
		}
	}
	if !fullRowVisible {
		t.Fatalf("tiny viewport did not preserve a complete two-line file row:\n%s", view)
	}
	if !strings.Contains(view, "Rows ") || !strings.Contains(view, "Space mark") {
		t.Fatal("fixed row counter or footer missing", view)
	}
}

func TestLastScanDetailsShowFullDateAndCompleteness(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height, m.Screen, m.Scanned = 100, 24, "details", true
	m.LastScanAt = time.Date(2026, time.October, 30, 14, 32, 18, 0, time.Local)
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{
		m.Entries[0].Path: {State: scan.Unknown},
		m.Entries[1].Path: {State: scan.Partial},
	}}
	view := m.View()
	want := "Last scan: " + m.LastScanAt.Local().Format("2006-01-02 15:04:05 MST") + " | 1 unknown | 1 partial"
	if !strings.Contains(view, want) {
		t.Fatalf("full last scan details missing %q:\n%s", want, view)
	}
}
