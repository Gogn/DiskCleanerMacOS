package state

import (
	"context"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCompletedScanDoesNotBindSizesToChangedEntries(t *testing.T) {
	for _, change := range []string{"replace", "modify", "remove", "retype"} {
		t.Run(change, func(t *testing.T) {
			m := fixture(t)
			root := m.Root
			path := filepath.Join(root, "measured")
			keep := filepath.Join(root, "unchanged")
			for _, p := range []string{path, keep} {
				if err := os.WriteFile(p, []byte("old data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m.Key("s")
			event := m.Key("enter")()
			if event.Snapshot.Err != nil || event.Snapshot.Entries[path].State != scan.Complete {
				t.Fatal("fixture scan failed", event.Snapshot)
			}
			wantKeep := event.Snapshot.Entries[keep]
			switch change {
			case "replace":
				// Preserve the old inode, so a new inode cannot reuse its number.
				if err := os.Rename(path, path+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("new data"), 0600); err != nil {
					t.Fatal(err)
				}
			case "modify":
				if err := os.WriteFile(path, make([]byte, 65536), 0600); err != nil {
					t.Fatal(err)
				}
			case "remove", "retype":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if change == "retype" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			m.Apply(event)
			got := m.Snapshot.Entries[path]
			if !m.Scanned || got.State != scan.Unknown || got.Bytes != 0 || m.Snapshot.Entries[keep] != wantKeep {
				t.Fatalf("applying scan attached an old size to a changed object: changed=%+v unchanged=%+v", got, m.Snapshot.Entries[keep])
			}
			m.navigate(filepath.Join(root, "cache"))
			if change == "replace" {
				// Once rejected, even restoring the old object cannot revive the
				// discarded measurement on a subsequent visit.
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+"-original", path); err != nil {
					t.Fatal(err)
				}
			}
			m.navigate(root)
			got, exists := m.Snapshot.Entries[path]
			if change == "remove" {
				if exists {
					t.Fatal("removed entry survived refreshed listing")
				}
			} else if !exists || got.State != scan.Unknown || got.Bytes != 0 {
				t.Fatalf("cache revived a rejected measurement: %+v", got)
			}
			if m.Snapshot.Entries[keep] != wantKeep {
				t.Fatal("cache discarded an unchanged measurement")
			}
		})
	}
}

func TestScanObservationPrecedesMeasurementAndIncludesNewEntries(t *testing.T) {
	m := fixture(t)
	root := m.Root
	path := filepath.Join(root, "added-before-scan")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	// The browser has not refreshed its listing since this entry was added.
	scanNow(t, m)
	if m.Snapshot.Entries[path].State != scan.Complete {
		t.Fatal("pre-scan observation incorrectly used the stale browser listing")
	}
	m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
		snapshot := scan.AnalyzeWithProgress(ctx, root, masks, report)
		if err := os.Rename(path, path+"-original"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("after!"), 0600); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	scanNow(t, m)
	if got := m.Snapshot.Entries[path]; got.State != scan.Unknown || got.Bytes != 0 {
		t.Fatal("replacement during worker execution inherited the previous object's size", got)
	}
	m.navigate(filepath.Join(root, "cache"))
	m.navigate(root)
	if m.Snapshot.Entries[path].State != scan.Unknown {
		t.Fatal("replacement during worker execution was cached as measured")
	}
}

func TestCompletedScanRejectsReplacedRoot(t *testing.T) {
	m := fixture(t)
	root := m.Root
	m.Key("s")
	event := m.Key("enter")()
	if err := os.Rename(root, root+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	m.Apply(event)
	if m.Scanned || len(m.Snapshot.Entries) != 0 || !m.LastScanAt.IsZero() || m.Message == "" {
		t.Fatal("replaced root inherited a completed scan", m.Snapshot, m.Message)
	}
	if _, exists := m.scanCache[root]; exists {
		t.Fatal("replaced root retained cached measurements")
	}
}

func TestScanObservationCancellationAndMissingEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if observed, err := captureScanObservation(ctx, t.TempDir()); !errors.Is(err, context.Canceled) || observed != nil {
		t.Fatal("cancelled observation was published", observed, err)
	}
	var observed *scanObservation
	if _, _, err := observed.validate(t.TempDir(), scan.Snapshot{}); err == nil {
		t.Fatal("missing identity evidence was accepted by validation")
	}
	m := fixture(t)
	m.Snapshot = scan.Snapshot{State: scan.Complete}
	m.rememberScan(nil)
	if len(m.scanCache) != 0 {
		t.Fatal("cache fabricated current identity for an unobserved scan")
	}
}
