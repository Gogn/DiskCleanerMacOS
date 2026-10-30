package state

import (
	"context"
	"diskcleaner/internal/config"
	"diskcleaner/internal/scan"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFilterKeepsHiddenSelectionsAndConfirmsExactPaths(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	target := m.targets()[0]
	m.Key("/")
	m.Paste("FILE")
	m.Key("enter")
	if len(m.Entries) != 2 || len(m.visibleIndices()) != 1 || m.hiddenSelections() != 1 || !m.Scanned {
		t.Fatal("filter changed scan/selection or did not ignore case")
	}
	if !strings.Contains(m.View(), "1 hidden") {
		t.Fatal("hidden selection is not visible")
	}
	m.Key("t")
	current, ok := m.current()
	if !ok || !strings.Contains(current.Name, "file") {
		t.Fatal("sort moved cursor outside filter")
	}
	m.Key("d")
	if len(m.Confirmed) != 1 || m.Confirmed[0] != target || !strings.Contains(m.View(), "Hidden selected items") {
		t.Fatal("confirmation lost hidden target or its warning")
	}
	m.Key("esc")
	m.Key("/")
	m.Paste("-no-match")
	m.Key("enter")
	if _, ok := m.current(); ok {
		t.Fatal("empty filter exposes a selectable item")
	}
	m.Key("space")
	m.Key("right")
	if m.Selected[target.Path] != target || !strings.Contains(m.View(), "No matches") {
		t.Fatal("no-match actions changed the selection")
	}
	m.Key("esc")
	if m.Filter != "" || len(m.Selected) != 1 || len(m.visibleIndices()) != 2 {
		t.Fatal("clearing filter lost selection")
	}
	m.Key("/")
	m.Paste("é")
	m.Key("backspace")
	if m.Filter != "" {
		t.Fatal("backspace split a Unicode character")
	}
	m.Key("esc")
}

func TestUnifiedMasksAddToggleAndKeepUncheckedCustomPattern(t *testing.T) {
	m := fixture(t)
	m.Key("e")
	if !strings.Contains(m.View(), "node_modules") || !strings.Contains(m.View(), "CloudStorage") || !strings.Contains(m.View(), "Add pattern") {
		t.Fatal("unified editor missing controls")
	}
	m.Key("a")
	m.Paste("cache?")
	m.Key("enter")
	if !m.hasMask("cache?") || m.MaskInput {
		t.Fatal("custom pattern not added")
	}
	m.Key("pgdown")
	m.Key("space")
	if m.hasMask("cache?") || m.maskOptions[m.maskCursor] != "cache?" {
		t.Fatal("unchecked custom option disappeared")
	}
	m.Key("space")
	saved, err := config.Load(m.ConfigPath)
	if err != nil || len(saved.Masks[m.Root]) != 1 || saved.Masks[m.Root][0] != "cache?" {
		t.Fatal("toggle not persisted", err)
	}
	m.Key("a")
	m.Paste("bad/path")
	m.Key("enter")
	if m.Input != "bad/path" || !m.MaskInput || m.hasMask("bad/path") {
		t.Fatal("invalid input accepted or discarded")
	}
	m.Key("esc")
	m.Key("?")
	if m.Screen != "help" {
		t.Fatal("help not available from checkbox list")
	}
}

func TestBrowserFixedFrameColumnsColorsAndContext(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 80, 24
	m.Entries = nil
	m.Scanned = true
	m.Snapshot.Entries = map[string]scan.Measurement{}
	for i := 0; i < 40; i++ {
		entry := scan.Entry{Name: fmt.Sprintf("item-%02d", i), Path: fmt.Sprintf("%s/item-%02d", m.Root, i)}
		m.Entries = append(m.Entries, entry)
		m.Snapshot.Entries[entry.Path] = scan.Measurement{Bytes: uint64(i+1) * 4096, State: scan.Complete}
	}
	m.Cursor = 39
	view := m.View()
	lines := strings.Split(plainView(view), "\n")
	if len(lines) != 24 || !strings.Contains(lines[0], "Sort:") || !strings.HasSuffix(lines[4], "/root") || !strings.Contains(lines[2], "Selected size") || !strings.Contains(lines[3], "No exclusions") {
		t.Fatal("header moved with cursor", view)
	}
	for _, want := range []string{"Name", "Size", "Status", "[######]", "\x1b[37;48;2;38;52;69m", "Space select"} {
		if !strings.Contains(view, want) {
			t.Fatal("missing table element", want)
		}
	}
	if strings.Contains(view, "d cleanup") {
		t.Fatal("cleanup advertised without selection")
	}
	m.Snapshot.Entries[m.Entries[39].Path] = scan.Measurement{Bytes: 4096, State: scan.Partial}
	if !strings.Contains(m.View(), "\x1b[33;48;2;38;52;69m") {
		t.Fatal("cursor hides warning color")
	}
	for _, width := range []int{32, 40, 64, 80, 120} {
		m.Width, m.Height = width, 12
		m.setFilter("item-39")
		colored := m.View()
		if strings.Contains(colored, "Resize") {
			t.Fatal("valid small terminal cannot show filtered list", width)
		}
		if len(strings.Split(colored, "\n")) > m.Height {
			t.Fatal("frame too tall", width)
		}
		for _, line := range strings.Split(colored, "\n") {
			if cells(plainView(line)) > width {
				t.Fatal("frame too wide", width, line)
			}
		}
		m.NoColor = true
		if m.View() != plainView(colored) {
			t.Fatal("color changes layout")
		}
		m.NoColor = false
	}
}

func TestScanProgressAndCancellationStayOnUIThread(t *testing.T) {
	m := fixture(t)
	started := make(chan struct{})
	m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
		report(scan.Progress{Current: root + "/slow-folder", Completed: 2, Total: 5})
		close(started)
		<-ctx.Done()
		return scan.Snapshot{Err: ctx.Err()}
	}
	m.Key("s")
	job := m.Key("enter")
	finished := make(chan Event, 1)
	go func() { finished <- job() }()
	<-started
	m.RefreshProgress(m.startedAt.Add(3 * time.Second))
	if !m.Busy || m.Progress.Completed != 2 || m.Elapsed != 3*time.Second {
		t.Fatal("progress ended work or lost values")
	}
	view := m.View()
	for _, want := range []string{"2 / 5 items", "Elapsed: 3s", "slow-folder"} {
		if !strings.Contains(view, want) {
			t.Fatal("progress not displayed", view)
		}
	}
	if strings.Contains(view, "%") {
		t.Fatal("misleading percent shown")
	}
	old := m.progress
	m.Key("esc")
	m.Apply(<-finished)
	old.publish(scan.Progress{Completed: 999})
	m.RefreshProgress(time.Now())
	if m.Busy || m.Screen != "list" || m.Scanned || m.Progress.Completed == 999 {
		t.Fatal("stale progress revived cancelled scan")
	}
}
