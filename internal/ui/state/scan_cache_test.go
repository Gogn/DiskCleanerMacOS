package state

import (
	"context"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanCacheRestoresCanonicalRootsWithoutScanningOrSelections(t *testing.T) {
	m := fixture(t)
	a, b := m.Root, filepath.Join(m.Root, "cache")
	if err := os.WriteFile(filepath.Join(b, "nested"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
		calls++
		s := scan.AnalyzeWithProgress(ctx, root, masks, report)
		for path, measurement := range s.Entries {
			measurement.Bytes = 100
			if filepath.Base(path) == "cache" {
				measurement.Bytes = 200
			}
			s.Entries[path] = measurement
		}
		return s
	}
	scanNow(t, m)
	want := copySnapshot(m.Snapshot)
	m.Key("t")
	m.Key("space")
	if len(m.Selected) != 1 {
		t.Fatal("test did not select a scanned entry")
	}
	m.setFilter("cache")
	m.navigate(b)
	if m.Scanned || calls != 1 || m.Filter != "" || len(m.Selected) != 0 {
		t.Fatal("new root scanned automatically or retained browser state")
	}
	scanNow(t, m)
	m.navigate(a)
	if !m.Scanned || calls != 2 || !m.SortBySize || m.Entries[0].Name != "cache" || len(m.Selected) != 0 || !reflect.DeepEqual(m.Snapshot, want) {
		t.Fatal("return did not restore measured sizes and sorting independently", m.Snapshot)
	}
	// The displayed snapshot and sorted listing must not mutate cached data.
	delete(m.Snapshot.Entries, filepath.Join(a, "cache"))
	m.Snapshot.Listing[0].Name = "changed only in UI"
	m.navigate(b)
	if !m.Scanned || len(m.Snapshot.Entries) != 1 || calls != 2 {
		t.Fatal("second root cache was lost")
	}
	alias := filepath.Join(filepath.Dir(a), "alias")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	m.navigate(alias)
	if m.Root != a || !reflect.DeepEqual(m.Snapshot, want) || calls != 2 {
		t.Fatal("canonical alias lost cached data or cache shared UI storage")
	}
	fresh := New(a, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !reflect.DeepEqual(fresh.Snapshot, want) || len(fresh.Selected) != 0 {
		t.Fatal("new model did not restore previous measurements independently of selection")
	}
}

func TestScanCacheRefreshesEntriesAndRejectsChangedObjects(t *testing.T) {
	m := fixture(t)
	a, b := m.Root, filepath.Join(m.Root, "cache")
	for _, name := range []string{"keep", "removed", "retyped", "replaced", "modified"} {
		if err := os.WriteFile(filepath.Join(a, name), []byte("before"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, m)
	keep := m.Snapshot.Entries[filepath.Join(a, "keep")]
	m.navigate(b)
	for _, name := range []string{"removed", "retyped"} {
		if err := os.Remove(filepath.Join(a, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(a, "retyped"), 0700); err != nil {
		t.Fatal(err)
	}
	// Keep the old inode alive so replacement cannot accidentally reuse it.
	if err := os.Rename(filepath.Join(a, "replaced"), filepath.Join(b, "old-replaced")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"added", "modified", "replaced"} {
		if err := os.WriteFile(filepath.Join(a, name), []byte("different content and size"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.navigate(a)
	if !m.Scanned || len(m.Snapshot.Listing) != len(m.Entries) || m.Snapshot.Entries[filepath.Join(a, "keep")] != keep {
		t.Fatal("fresh listing discarded unchanged measurements")
	}
	if _, ok := m.Snapshot.Entries[filepath.Join(a, "removed")]; ok {
		t.Fatal("removed entry survived in cached snapshot")
	}
	for _, name := range []string{"added", "modified", "replaced", "retyped"} {
		measurement := m.Snapshot.Entries[filepath.Join(a, name)]
		if measurement.State != scan.Unknown || measurement.Bytes != 0 {
			t.Fatal("changed object retained stale measurement", name, measurement)
		}
	}
	for i, entry := range m.Snapshot.Listing {
		if entry != m.Entries[i] {
			t.Fatal("snapshot listing did not refresh with browser listing")
		}
	}
	m.navigate(b)
	backup := a + "-old"
	if err := os.Rename(a, backup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(backup) })
	if err := os.Mkdir(a, 0700); err != nil {
		t.Fatal(err)
	}
	m.navigate(a)
	if m.Scanned {
		t.Fatal("replaced root inherited another filesystem object's scan")
	}
}

func TestRescanReplacesOnlyItsRootAndFailureCannotRestoreOldCache(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancellation"}[cancelled], func(t *testing.T) {
			m := fixture(t)
			a, b := m.Root, filepath.Join(m.Root, "cache")
			scanNow(t, m)
			m.navigate(b)
			scanNow(t, m)
			m.navigate(a)
			added := filepath.Join(a, "new-after-first-scan")
			if err := os.WriteFile(added, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			scanNow(t, m)
			m.navigate(b)
			if !m.Scanned {
				t.Fatal("rescan evicted another root")
			}
			m.navigate(a)
			if m.Snapshot.Entries[added].State != scan.Complete {
				t.Fatal("rescan did not replace this root's old snapshot")
			}
			m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
				err := errors.New("scan failed")
				if cancelled {
					err = ctx.Err()
				}
				return scan.Snapshot{Err: err, Entries: map[string]scan.Measurement{added: {Bytes: 999, State: scan.Partial}}}
			}
			m.Key("s")
			job := m.Key("enter")
			if cancelled {
				m.Key("esc")
			}
			m.Apply(job())
			if m.Scanned || len(m.Snapshot.Entries) != 0 {
				t.Fatal("failed rescan published partial results")
			}
			m.navigate(b)
			if !m.Scanned {
				t.Fatal("failed rescan evicted another root")
			}
			m.navigate(a)
			if m.Scanned {
				t.Fatal("failed rescan resurrected old measurements on revisit")
			}
			fresh := New(a, m.Settings, m.Guard, m.ConfigPath)
			if fresh.Scanned {
				t.Fatal("new session resurrected measurements invalidated by a failed rescan")
			}
			fresh.navigate(b)
			if !fresh.Scanned {
				t.Fatal("failed rescan erased another root from persistent cache")
			}
		})
	}
}

func TestGlobalMaskChangesEvictAllScans(t *testing.T) {
	m := fixture(t)
	a, b := m.Root, filepath.Join(m.Root, "cache")
	scanNow(t, m)
	m.navigate(b)
	scanNow(t, m)
	m.toggleMask("unused-global-pattern")
	if m.Scanned {
		t.Fatal("invalidation retained current measurements")
	}
	for _, root := range []string{a, b} {
		fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
		if fresh.Scanned || len(fresh.Snapshot.Entries) != 0 {
			t.Fatal("new session restored globally invalidated measurements", root)
		}
		m.navigate(root)
		if m.Scanned || len(m.Snapshot.Entries) != 0 {
			t.Fatal("invalidation retained cached measurements", root)
		}
	}
}
