package state

import (
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func entryNames(m *Model) string {
	names := make([]string, len(m.Entries))
	for i, entry := range m.Entries {
		names[i] = entry.Name
	}
	return strings.Join(names, ",")
}

func TestSortTogglePreservesTargetsAndSnapshot(t *testing.T) {
	m := fixture(t)
	listing := append([]scan.Entry(nil), m.Entries...)
	m.Apply(Event{Kind: "scan", Snapshot: scan.Snapshot{
		Listing: listing,
		Entries: map[string]scan.Measurement{
			listing[0].Path: {Bytes: 10, State: scan.Complete},
			listing[1].Path: {Bytes: 100, State: scan.Complete},
		},
	}})
	m.Key("space")
	target := m.Selected[listing[0].Path]
	if job := m.Key("t"); job != nil {
		t.Fatal("sorting started work")
	}
	current, _ := m.current()
	if !m.SortBySize || m.Entries[0].Path != listing[1].Path || current.Path != target.Path || m.Selected[target.Path] != target {
		t.Fatal("sorting changed the highlighted or selected target")
	}
	if m.Snapshot.Listing[0].Path != listing[0].Path || !m.Scanned {
		t.Fatal("sorting modified the scan snapshot")
	}
	m.Key("d")
	if len(m.Confirmed) != 1 || m.Confirmed[0] != target {
		t.Fatal("confirmation contains the wrong object after sorting")
	}
	m.Key("t")
	if !m.SortBySize {
		t.Fatal("sorting should only change from the browser")
	}
	m.Key("esc")
	m.Key("t")
	if m.SortBySize || m.Cursor != 0 || m.Entries[0].Path != target.Path {
		t.Fatal("alphabetical order or cursor was not restored")
	}
}

func TestSizeSortOrdersTiesAndUnmeasuredItems(t *testing.T) {
	m := fixture(t)
	m.Entries = nil
	m.Snapshot.Entries = map[string]scan.Measurement{}
	for _, item := range []struct {
		name  string
		size  uint64
		state scan.State
	}{
		{"unknown", 999, scan.Unknown},
		{"zero", 0, scan.Complete},
		{"beta", 50, scan.Complete},
		{"Alpha", 50, scan.Partial},
		{"excluded", 999, scan.Complete},
		{"pending", 999, scan.Pending},
		{"skipped", 999, scan.Skipped},
	} {
		path := filepath.Join(m.Root, item.name)
		m.Entries = append(m.Entries, scan.Entry{Path: path, Name: item.name})
		m.Snapshot.Entries[path] = scan.Measurement{Bytes: item.size, State: item.state}
	}
	m.Entries = append(m.Entries, scan.Entry{Path: filepath.Join(m.Root, "missing"), Name: "missing"})
	m.Settings.Masks[m.Root] = []string{"excluded"}
	m.Scanned = true
	m.Key("t")
	if got := entryNames(m); got != "Alpha,beta,zero,excluded,missing,pending,skipped,unknown" {
		t.Fatal(got)
	}
	m.invalidate()
	if got := entryNames(m); got != "Alpha,beta,excluded,missing,pending,skipped,unknown,zero" {
		t.Fatal("stale sizes used after invalidation:", got)
	}
	if !m.SortBySize || !strings.Contains(m.View(), "Size (scan needed)") {
		t.Fatal("size preference or pending status lost")
	}
}

func TestDefaultAlphabeticalAndSizeSortAcrossScans(t *testing.T) {
	m := fixture(t)
	for _, name := range []string{"Zebra", "apple", "Apple"} {
		if err := os.WriteFile(filepath.Join(m.Root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.navigate(m.Root)
	// On case-insensitive filesystems, Apple and apple may be the same entry.
	if m.SortBySize || m.Cursor != 0 || strings.ToLower(m.Entries[0].Name) != "apple" {
		t.Fatal("startup order is not alphabetical", entryNames(m))
	}
	if job := m.Key("t"); job != nil || m.Scanned {
		t.Fatal("sort triggered a scan")
	}
	listing := append([]scan.Entry(nil), m.Entries...)
	large := listing[len(listing)-1]
	m.Apply(Event{Kind: "scan", Snapshot: scan.Snapshot{Listing: listing, Entries: map[string]scan.Measurement{
		large.Path: {Bytes: 100, State: scan.Complete},
	}}})
	if m.Entries[0].Path != large.Path {
		t.Fatal("scan did not apply size ordering")
	}
	m.navigate(filepath.Join(m.Root, "cache"))
	if !m.SortBySize || m.Scanned || len(m.Entries) != 0 {
		t.Fatal("navigation lost the sort preference or kept stale sizes")
	}
	m.Key("t") // Empty lists must support toggling too.
	if m.SortBySize || m.Cursor != 0 {
		t.Fatal("empty-list sort failed")
	}
}
