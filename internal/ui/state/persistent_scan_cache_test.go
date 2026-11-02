package state

import (
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentScanCacheRejectsChangedEntriesAcrossSessions(t *testing.T) {
	m := fixture(t)
	root := m.Root
	for _, name := range []string{"keep", "modified", "replaced", "removed"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("before"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, m)
	keep := m.Snapshot.Entries[filepath.Join(root, "keep")]
	// Preserve the replaced inode, even on filesystems that quickly reuse them.
	if err := os.Rename(filepath.Join(root, "replaced"), filepath.Join(root, "cache", "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "removed")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"modified", "replaced", "added"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("changed content and size"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || fresh.Snapshot.Entries[filepath.Join(root, "keep")] != keep {
		t.Fatal("new session discarded unchanged measurements")
	}
	if _, exists := fresh.Snapshot.Entries[filepath.Join(root, "removed")]; exists {
		t.Fatal("removed entry survived persistent restore")
	}
	for _, name := range []string{"modified", "replaced", "added"} {
		got := fresh.Snapshot.Entries[filepath.Join(root, name)]
		if got.State != scan.Unknown || got.Bytes != 0 {
			t.Fatal("changed entry inherited persistent measurement", name, got)
		}
	}
}

func TestPersistentScanCacheRejectsReplacedRootAcrossSessions(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	root := m.Root
	old := root + "-old"
	if err := os.Rename(root, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
	if fresh.Scanned || !fresh.LastScanAt.IsZero() || len(fresh.Snapshot.Entries) != 0 {
		t.Fatal("replacement root inherited persisted measurements")
	}
}

func TestPersistentScanCacheRejectsDifferentExclusions(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	settings := m.Settings
	settings.Masks = append(append([]string(nil), settings.Masks...), "cache")
	fresh := New(m.Root, settings, m.Guard, m.ConfigPath)
	if fresh.Scanned || !fresh.LastScanAt.IsZero() || len(fresh.Snapshot.Entries) != 0 {
		t.Fatal("new exclusion rules reused measurements computed with different rules")
	}
}

func TestPersistentScanCacheIgnoresUnreadableFormats(t *testing.T) {
	for _, contents := range []string{"not a SQLite database", "SQLite format 3\x00truncated"} {
		t.Run(contents, func(t *testing.T) {
			m := fixture(t)
			scanNow(t, m)
			path := filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			if fresh.Scanned || !fresh.LastScanAt.IsZero() || len(fresh.Entries) == 0 {
				t.Fatal("invalid cache prevented ordinary unscanned browsing")
			}
			// A damaged cache must not prevent explicitly requested new scans.
			// Leave the database intact because another process may still use it.
			scanNow(t, fresh)
			after, err := os.ReadFile(path)
			if err != nil || string(after) != contents {
				t.Fatal("cache failure removed or rewrote the damaged database", err)
			}
			if !strings.Contains(strings.ToLower(fresh.View()), "cache") {
				t.Fatal("failed persistence did not explain that scan results were not saved", fresh.View())
			}
			again := New(m.Root, fresh.Settings, fresh.Guard, fresh.ConfigPath)
			if again.Scanned {
				t.Fatal("unrepaired damaged database supplied a saved scan")
			}
		})
	}
}

func TestPersistentScanCacheWarnsAboutUnobservedDescendantChanges(t *testing.T) {
	m := fixture(t)
	file := filepath.Join(m.Root, "cache", "nested")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	original := m.Snapshot.Entries[filepath.Dir(file)]
	if err := os.WriteFile(file, []byte("changed nested content and size"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || fresh.Snapshot.Entries[filepath.Dir(file)] != original {
		t.Fatal("restore should retain an unchanged directory object's snapshot")
	}
	view := strings.ToLower(fresh.View())
	if !strings.Contains(view, "may have changed") {
		t.Fatal("restored directory measurement lacks a visible stale-size warning", fresh.View())
	}
	fresh.navigate(filepath.Dir(file))
	if fresh.Scanned || strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("unscanned root inherited another root's saved-size warning")
	}
	scanNow(t, fresh)
	fresh.navigate(m.Root)
	if !strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("navigation lost disk-restored snapshot provenance")
	}
	fresh.Key("v")
	if !strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("details view hid saved-size warning")
	}
	fresh.Key("esc")
	scanNow(t, fresh)
	if strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("successful rescan retained saved-size warning")
	}
	fresh.navigate(filepath.Dir(file))
	fresh.navigate(m.Root)
	if strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("navigation revived saved-size warning after successful rescan")
	}
}

func TestPersistentScanCacheIsPrivateAndOptional(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	path := filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("saved filesystem metadata is not private", info.Mode().Perm())
	}
	fresh := New(m.Root, m.Settings, m.Guard, "")
	if fresh.Scanned {
		t.Fatal("disabled persistence loaded a saved scan")
	}
	scanNow(t, fresh)
	if strings.Contains(strings.ToLower(fresh.View()), "saved sizes may have changed") {
		t.Fatal("explicit scan without persistence was marked as restored")
	}
}

func TestPersistentScanCacheWriteFailureDoesNotPreventScan(t *testing.T) {
	m := fixture(t)
	m.ConfigPath = filepath.Join(t.TempDir(), "config.json")
	path := filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	// A directory at the database path cannot be opened as a cache database.
	scanNow(t, m)
	if m.LastScanAt.IsZero() || len(m.Snapshot.Entries) == 0 {
		t.Fatal("cache write failure discarded a successful explicit scan")
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if fresh.Scanned || len(fresh.Entries) == 0 {
		t.Fatal("unreadable cache did not fall back to ordinary browsing")
	}
}
