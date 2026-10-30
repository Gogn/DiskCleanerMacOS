package state

import (
	"context"
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func highlightPath(t *testing.T, m *Model, path string) {
	t.Helper()
	for i, entry := range m.Entries {
		if entry.Path == path {
			m.Cursor = i
			return
		}
	}
	t.Fatal("entry missing", path)
}

func requireHighlightedPath(t *testing.T, m *Model, path string) {
	t.Helper()
	entry, ok := m.current()
	if !ok || entry.Path != path {
		t.Fatal("wrong highlighted entry", entry.Path, "want", path)
	}
}

func TestNavigationRestoresHighlightedPathAfterSortingAndListingChanges(t *testing.T) {
	m := fixture(t)
	a := m.Root
	b := filepath.Join(filepath.Dir(a), "other-root")
	for _, path := range []string{b, filepath.Join(a, "z-folder"), filepath.Join(a, ".hidden")} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
		s := scan.AnalyzeWithProgress(ctx, root, masks, report)
		for path, measurement := range s.Entries {
			measurement.Bytes = 100
			if filepath.Base(path) == "z-folder" {
				measurement.Bytes = 1000
			}
			s.Entries[path] = measurement
		}
		return s
	}
	scanNow(t, m)
	m.Key("t")
	want := filepath.Join(a, "cache")
	highlightPath(t, m, want)
	m.Key("space")
	m.setFilter("cache")
	m.navigate(b)
	if err := os.WriteFile(filepath.Join(a, "aaa-new"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.navigate(a)
	requireHighlightedPath(t, m, want)
	if !m.SortBySize || m.Entries[0].Name != "z-folder" || len(m.Selected) != 0 || m.Filter != "" {
		t.Fatal("restoring cursor lost sorting or retained selection/filter")
	}
	// Disappearance falls back to a visible entry instead of an old index.
	m.navigate(b)
	if err := os.Remove(want); err != nil {
		t.Fatal(err)
	}
	m.navigate(a)
	if entry, ok := m.current(); !ok || entry.Path == want {
		t.Fatal("removed highlight was not normalized", entry)
	}
	hidden := filepath.Join(a, ".hidden")
	highlightPath(t, m, hidden)
	m.navigate(b)
	m.HideHidden = true
	m.navigate(a)
	if entry, ok := m.current(); !ok || entry.Path == hidden || !m.matchesFilter(entry) {
		t.Fatal("hidden history path was not normalized", entry)
	}
}

func TestParentNavigationHighlightsFolderJustLeftWithoutPriorHistory(t *testing.T) {
	m := fixture(t)
	parent := m.Root
	child := filepath.Join(parent, "cache")
	m = New(child, m.Settings, m.Guard, m.ConfigPath)
	if len(m.navigationHistory) != 0 {
		t.Fatal("new model inherited navigation history")
	}
	m.Key("left")
	requireHighlightedPath(t, m, child)
	// An older parent highlight must not override the child we just exited.
	m.navigationHistory = map[string]string{}
	for _, entry := range m.Entries {
		if entry.Path != child {
			m.navigationHistory[parent] = entry.Path
			break
		}
	}
	m.navigate(child)
	m.Key("left")
	requireHighlightedPath(t, m, child)
	if len(m.Selected) != 0 || m.Filter != "" {
		t.Fatal("returning to parent retained selection or filter")
	}
}

func TestFailedNavigationPreservesCursorAndHistory(t *testing.T) {
	m := fixture(t)
	root := m.Root
	highlightPath(t, m, filepath.Join(root, "cache"))
	m.navigationHistory = map[string]string{root: "previous-history-path"}
	before := map[string]string{root: "previous-history-path"}
	m.navigate(filepath.Join(root, "missing-directory"))
	if m.Root != root || !reflect.DeepEqual(m.navigationHistory, before) {
		t.Fatal("failed navigation changed current folder or history")
	}
	requireHighlightedPath(t, m, filepath.Join(root, "cache"))
	// A fresh model has no remembered highlight from the previous session.
	m.navigationHistory[root] = filepath.Join(root, "file\n\x1b")
	fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
	if len(fresh.navigationHistory) != 0 {
		t.Fatal("history survived model lifetime")
	}
	requireHighlightedPath(t, fresh, filepath.Join(root, "cache"))
}
