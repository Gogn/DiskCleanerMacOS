package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCleanupPreservesSiblingsAndInvalidatesAncestorAndDescendantScans(t *testing.T) {
	m := fixture(t)
	root, parent := m.Root, filepath.Join(m.Root, "cache")
	victim, unrelated := filepath.Join(parent, "victim"), filepath.Join(parent, "victim-other")
	rootSibling := filepath.Join(root, "file\n\x1b")
	for _, path := range []string{victim, unrelated} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(parent, "keep")
	for _, path := range []string{filepath.Join(victim, "data"), keep, filepath.Join(unrelated, "data")} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, m)
	rootSiblingSize := m.Snapshot.Entries[rootSibling]
	m.navigate(victim)
	scanNow(t, m)
	m.navigate(unrelated)
	scanNow(t, m)
	unrelatedSnapshot := copySnapshot(m.Snapshot)
	m.navigate(parent)
	scanNow(t, m)
	keepSize, scanTime := m.Snapshot.Entries[keep], m.LastScanAt
	m.Analyze = func(context.Context, string, []string, func(scan.Progress)) scan.Snapshot {
		t.Fatal("cleanup started an automatic scan")
		return scan.Snapshot{}
	}
	targets := []cleanup.Target{{Path: victim}}
	m.invalidateCleanupCache(targets)
	// Simulate restart between confirmation and the runner: affected sizes must
	// already be gone from disk, including scans rooted within the target.
	beforeRunner := New(root, m.Settings, m.Guard, m.ConfigPath)
	if beforeRunner.Snapshot.Entries[parent].State != scan.Unknown || beforeRunner.Snapshot.Entries[rootSibling] != rootSiblingSize {
		t.Fatal("pre-run persistence did not isolate ancestor invalidation from siblings")
	}
	beforeRunner.navigate(victim)
	if beforeRunner.Scanned {
		t.Fatal("pre-run cache retained a scan rooted under the target")
	}
	if err := os.RemoveAll(victim); err != nil {
		t.Fatal(err)
	}
	m.Confirmed = targets
	m.Selected[keep] = cleanup.Target{Path: keep}
	m.Apply(Event{Kind: "cleanup", Results: []cleanup.Result{{Path: victim, Outcome: cleanup.Completed}}})
	if !m.Scanned || len(m.Selected) != 0 || m.Snapshot.Entries[keep] != keepSize || !m.LastScanAt.Equal(scanTime) {
		t.Fatal("cleanup discarded unaffected sizes/time or retained selection", m.Snapshot)
	}
	if _, ok := m.Snapshot.Entries[victim]; ok {
		t.Fatal("deleted entry retained a measurement")
	}
	for _, entry := range m.Entries {
		if entry.Path == victim {
			t.Fatal("deleted entry retained in refreshed listing")
		}
	}
	m.navigate(root)
	if m.Snapshot.Entries[parent].State != scan.Unknown || m.Snapshot.Entries[rootSibling] != rootSiblingSize {
		t.Fatal("cleanup retained stale ancestor size or lost sibling size")
	}
	m.navigate(unrelated)
	if !reflect.DeepEqual(m.Snapshot, unrelatedSnapshot) {
		t.Fatal("cleanup erased an unrelated root scan")
	}
	fresh := New(parent, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || fresh.Snapshot.Entries[keep] != keepSize || !fresh.LastScanAt.Equal(scanTime) {
		t.Fatal("surviving measurements/time did not persist after cleanup")
	}
	if _, ok := fresh.Snapshot.Entries[victim]; ok {
		t.Fatal("restart revived deleted measurements")
	}
}

func TestFailedAndCancelledCleanupKeepSiblingsButRejectPartiallyChangedTarget(t *testing.T) {
	for _, failure := range []error{errors.New("runner failed after a change"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			m := fixture(t)
			victim, keep := filepath.Join(m.Root, "cache"), filepath.Join(m.Root, "file\n\x1b")
			child := filepath.Join(victim, "nested")
			if err := os.WriteFile(child, []byte("before"), 0600); err != nil {
				t.Fatal(err)
			}
			scanNow(t, m)
			keepSize, scanTime := m.Snapshot.Entries[keep], m.LastScanAt
			highlightPath(t, m, victim)
			m.Key("space")
			m.Trash = func(context.Context, string) error {
				if err := os.WriteFile(child, []byte("partially changed descendant with directory identity unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
				return failure
			}
			m.Key("d")
			job := m.Key("enter")
			if job == nil {
				t.Fatal("no cleanup job")
			}
			pending := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			if pending.Scanned || len(pending.Snapshot.Entries) != 0 {
				t.Fatal("another session read cached measurements during cleanup")
			}
			m.Apply(job())
			if len(m.Results) != 1 || m.Results[0].Outcome != cleanup.Failed {
				t.Fatal("partial mutation was not reported as failed", m.Results)
			}
			if !m.Scanned || m.Snapshot.Entries[victim].State != scan.Unknown || m.Snapshot.Entries[keep] != keepSize || !m.LastScanAt.Equal(scanTime) || len(m.Selected) != 0 {
				t.Fatal("failed cleanup retained affected size or discarded unaffected measurements", m.Snapshot)
			}
			fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			if fresh.Snapshot.Entries[victim].State != scan.Unknown || fresh.Snapshot.Entries[keep] != keepSize {
				t.Fatal("restart revived partially mutated target size")
			}
		})
	}
}

func TestCleanupWithoutAttemptedPathsPreservesAllMeasurements(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	want, when := copySnapshot(m.Snapshot), m.LastScanAt
	m.Apply(Event{Kind: "cleanup", Results: []cleanup.Result{{Outcome: cleanup.Skipped}}})
	if !m.Scanned || !reflect.DeepEqual(m.Snapshot, want) || !m.LastScanAt.Equal(when) {
		t.Fatal("cleanup without a mutated path erased existing scan")
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !reflect.DeepEqual(fresh.Snapshot, want) || !fresh.LastScanAt.Equal(when) {
		t.Fatal("cleanup without attempted paths erased persistent scan")
	}
}
