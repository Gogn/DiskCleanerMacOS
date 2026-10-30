package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var browserRowsRE = regexp.MustCompile(`Rows (\d+)-(\d+) / (\d+)`)

func addNavigationEntries(t *testing.T, m *Model, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		name := "page-item-" + strconv.Itoa(1000+i)
		if err := os.WriteFile(filepath.Join(m.Root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(m.Root, ".page-hidden"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.navigate(m.Root)
}

func displayedBrowserPageSize(t *testing.T, m *Model) int {
	t.Helper()
	view := plainView(m.View())
	for _, line := range strings.Split(view, "\n") {
		match := browserRowsRE.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		start, _ := strconv.Atoi(match[1])
		end, _ := strconv.Atoi(match[2])
		physicalRows := end - start + 1
		if m.Width < 64 {
			return physicalRows / 2
		}
		return physicalRows
	}
	t.Fatalf("browser row counter missing:\n%s", view)
	return 0
}

func TestBrowserPageNavigationUsesVisibleViewportAndKeepsState(t *testing.T) {
	for _, size := range []struct{ width, height int }{{32, 12}, {80, 24}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			m := fixture(t)
			addNavigationEntries(t, m, 48)
			m.Width, m.Height = size.width, size.height
			m.Screen, m.Filter, m.HideHidden = "list", "page-", true
			m.Scanned, m.SortBySize = true, true
			m.Selected = map[string]cleanup.Target{m.Entries[0].Path: {Path: m.Entries[0].Path}}
			m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{m.Entries[0].Path: {State: scan.Complete}}}
			indices := m.visibleIndices()
			if len(indices) < 10 {
				t.Fatalf("fixture did not produce enough visible items: %d", len(indices))
			}
			pageSize := m.browserPageSize()
			if pageSize < 1 || pageSize != displayedBrowserPageSize(t, m) {
				t.Fatalf("page size %d differs from visible viewport %d", pageSize, displayedBrowserPageSize(t, m))
			}

			root, filter, sorting, screen, scanned := m.Root, m.Filter, m.SortBySize, m.Screen, m.Scanned
			selected := map[string]cleanup.Target{}
			for path, target := range m.Selected {
				selected[path] = target
			}
			snapshot := m.Snapshot
			m.Cursor = indices[0]
			m.Key("pgdown")
			if want := indices[min(pageSize, len(indices)-1)]; m.Cursor != want {
				t.Fatalf("PgDn landed at cursor %d, want %d", m.Cursor, want)
			}
			position := min(pageSize+2, len(indices)-1)
			m.Cursor = indices[position]
			m.Key("pgup")
			if want := indices[max(0, position-pageSize)]; m.Cursor != want {
				t.Fatalf("PgUp landed at cursor %d, want %d", m.Cursor, want)
			}
			if m.Root != root || m.Filter != filter || m.SortBySize != sorting || m.Screen != screen || m.Scanned != scanned || !reflect.DeepEqual(m.Selected, selected) || !reflect.DeepEqual(m.Snapshot, snapshot) {
				t.Fatal("page navigation changed browser state other than the cursor")
			}
			for i := 0; i < len(indices)+1; i++ {
				m.Key("pgdown")
			}
			if m.Cursor != indices[len(indices)-1] {
				t.Fatalf("repeated PgDn did not clamp at the last visible entry: %d", m.Cursor)
			}
			for i := 0; i < len(indices)+1; i++ {
				m.Key("pgup")
			}
			if m.Cursor != indices[0] {
				t.Fatalf("repeated PgUp did not clamp at the first visible entry: %d", m.Cursor)
			}
		})
	}
}

func TestHomeEndMoveToVisibleEdges(t *testing.T) {
	m := fixture(t)
	addNavigationEntries(t, m, 10)
	m.HideHidden = true
	m.Filter = "page-item"
	indices := m.visibleIndices()
	if len(indices) == 0 {
		t.Fatal("fixture has no visible filtered entries")
	}
	m.Cursor = indices[len(indices)/2]
	m.Key("home")
	if m.Cursor != indices[0] {
		t.Fatalf("Home landed at %d, want first visible index %d", m.Cursor, indices[0])
	}
	m.Key("end")
	if m.Cursor != indices[len(indices)-1] {
		t.Fatalf("End landed at %d, want last visible index %d", m.Cursor, indices[len(indices)-1])
	}
	for _, index := range indices {
		if m.Entries[index].Name == ".page-hidden" {
			t.Fatal("Home/End included a hidden entry")
		}
	}
}

func TestBrowserPagingIsSafeWithNoVisibleEntries(t *testing.T) {
	m := fixture(t)
	m.Filter = "no matches"
	for _, key := range []string{"pgup", "pgdown", "home", "end"} {
		m.Key(key)
		if m.Cursor < 0 || m.Cursor >= len(m.Entries) {
			t.Fatalf("%s moved cursor outside the entry slice: %d", key, m.Cursor)
		}
	}
	m.Entries = nil
	m.Cursor = 0
	m.Filter = ""
	for _, key := range []string{"pgup", "pgdown", "home", "end"} {
		m.Key(key)
		if m.Cursor != 0 {
			t.Fatalf("%s moved cursor with an empty entry list: %d", key, m.Cursor)
		}
	}
}

func TestHomeEndRemainLiteralNoOpsInTextInputs(t *testing.T) {
	m := fixture(t)
	m.setFilter("cache")
	m.Cursor = len(m.Entries) - 1
	if !m.matchesFilter(m.Entries[m.Cursor]) {
		m.normalizeCursor()
	}
	originalCursor := m.Cursor
	m.Screen = "filter"
	for _, key := range []string{"home", "end"} {
		m.Key(key)
		if m.Cursor != originalCursor || m.Filter != "cache" {
			t.Fatalf("%s unexpectedly moved the browser while filtering", key)
		}
	}
	m.Screen, m.Input = "root", "/tmp/path"
	for _, key := range []string{"home", "end"} {
		m.Key(key)
		if m.Cursor != originalCursor || m.Input != "/tmp/path" {
			t.Fatalf("%s unexpectedly moved the browser or changed path input", key)
		}
	}
}
