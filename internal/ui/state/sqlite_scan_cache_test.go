package state

import (
	"context"
	"database/sql"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScanCacheIsSQLiteWithNormalizedTables(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	path := filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 16 || string(data[:16]) != "SQLite format 3\x00" {
		t.Fatal("cache is not a real SQLite database")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"meta", "roots", "entries"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count == 0 {
			t.Fatal("SQLite cache is missing populated normalized table", table, count, err)
		}
		if table == "entries" && count != len(m.Snapshot.Entries) {
			t.Fatal("SQLite cache did not store one row per measured entry", count, len(m.Snapshot.Entries))
		}
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("SQLite integrity check failed", integrity, err)
	}
}

func TestSQLiteConcurrentModelsPreserveDifferentRoots(t *testing.T) {
	m := fixture(t)
	child := filepath.Join(m.Root, "cache")
	if err := os.WriteFile(filepath.Join(child, "data"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	other := New(child, m.Settings, m.Guard, m.ConfigPath)
	var workers sync.WaitGroup
	workers.Add(2)
	for _, model := range []*Model{m, other} {
		go func(model *Model) { defer workers.Done(); scanNow(t, model) }(model)
	}
	workers.Wait()
	for _, model := range []*Model{m, other} {
		fresh := New(model.Root, model.Settings, model.Guard, model.ConfigPath)
		if !fresh.Scanned || !reflect.DeepEqual(fresh.Snapshot, model.Snapshot) || !fresh.LastScanAt.Equal(model.LastScanAt) {
			t.Fatal("concurrent scan of another root overwrote a completed snapshot", model.Root, fresh.Snapshot)
		}
	}
}

func TestSQLiteStaleModelCannotResurrectCleanupMeasurements(t *testing.T) {
	m := fixture(t)
	victim := filepath.Join(m.Root, "cache")
	child := filepath.Join(victim, "nested")
	if err := os.WriteFile(child, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	stale := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !m.invalidateCleanupCache([]cleanup.Target{{Path: child}}) {
		t.Fatal("fixture cleanup invalidation failed")
	}
	// A nested content change need not change the measured directory's own
	// size/mtime. Database invalidation must beat an older model's memory.
	if err := os.WriteFile(child, []byte("changed nested bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	stale.navigate(stale.Root)
	if stale.Snapshot.Entries[victim].State != scan.Unknown {
		t.Fatal("stale model revived a cleanup-invalidated ancestor size")
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if fresh.Snapshot.Entries[victim].State != scan.Unknown {
		t.Fatal("stale model rewrote invalidated measurements into SQLite")
	}
}

func TestSQLiteLateScanCompletionCannotOverwriteCleanupInvalidation(t *testing.T) {
	for _, readNewerScan := range []bool{false, true} {
		name := "invalidated"
		if readNewerScan {
			name = "newer-scan-already-read"
		}
		t.Run(name, func(t *testing.T) {
			m := fixture(t)
			victim := filepath.Join(m.Root, "cache")
			child := filepath.Join(victim, "nested")
			if err := os.WriteFile(child, []byte("before"), 0600); err != nil {
				t.Fatal(err)
			}
			scanNow(t, m)
			late := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			late.Key("s")
			job := late.Key("enter")
			if job == nil {
				t.Fatal("no scan job")
			}
			event := job()
			if !m.invalidateCleanupCache([]cleanup.Target{{Path: child}}) {
				t.Fatal("fixture cleanup invalidation failed")
			}
			if err := os.WriteFile(child, []byte("nested mutation after scan completed"), 0600); err != nil {
				t.Fatal(err)
			}
			if readNewerScan {
				m.navigate(m.Root)
				scanNow(t, m)
				late.navigate(late.Root)
			}
			late.Apply(event)
			fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			if readNewerScan {
				if !fresh.Scanned || !reflect.DeepEqual(fresh.Snapshot, m.Snapshot) || !fresh.LastScanAt.Equal(m.LastScanAt) {
					t.Fatal("late scan discarded or overwrote the newer authoritative snapshot")
				}
			} else if got, exists := fresh.Snapshot.Entries[victim]; exists && (got.State != scan.Unknown || got.Bytes != 0) {
				t.Fatal("late scan overwrote a later cleanup invalidation", got)
			}
		})
	}
}

func TestSQLiteCleanupInvalidatesAffectedRootsAbsentFromLocalModel(t *testing.T) {
	m := fixture(t)
	victim := filepath.Join(m.Root, "cache")
	if err := os.WriteFile(filepath.Join(victim, "data"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	other := New(victim, m.Settings, m.Guard, m.ConfigPath)
	scanNow(t, other)
	// This model was loaded before the other root was scanned. Cleanup must
	// invalidate the database's affected roots, not just this model's map.
	if !m.invalidateCleanupCache([]cleanup.Target{{Path: victim}}) {
		t.Fatal("fixture cleanup invalidation failed")
	}
	fresh := New(victim, m.Settings, m.Guard, m.ConfigPath)
	if fresh.Scanned || !fresh.LastScanAt.IsZero() {
		t.Fatal("cleanup retained an affected root written by another model")
	}
	other.navigate(victim)
	if other.Scanned {
		t.Fatal("other model revived its invalidated target-root snapshot")
	}
}

func TestSQLiteLockedInvalidationPreventsCleanupAndPreservesDatabase(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	want := copySnapshot(m.Snapshot)
	path := filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	calls := 0
	m.Trash = func(context.Context, string) error { calls++; return nil }
	highlightPath(t, m, filepath.Join(m.Root, "cache"))
	m.Key("space")
	m.Key("d")
	started := time.Now()
	job := m.Key("enter")
	if time.Since(started) > 5*time.Second {
		t.Fatal("locked cache invalidation did not return within its bounded wait")
	}
	if job != nil || m.Busy || calls != 0 || !strings.Contains(strings.ToLower(m.Message), "not started") {
		t.Fatal("cleanup proceeded without durable invalidation or lacked a clear cache error", m.Message, m.Busy, calls)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("locked invalidation removed or replaced the valid database", err)
	}
	if _, err := conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !reflect.DeepEqual(fresh.Snapshot, want) {
		t.Fatal("failed invalidation damaged the previously committed snapshot")
	}
	// The failed attempt keeps the user's explicit selection available to retry.
	m.Key("esc")
	m.Key("d")
	job = m.Key("enter")
	if job == nil {
		t.Fatal("cleanup could not be retried after releasing the database lock", m.Message)
	}
	m.Apply(job())
	if calls != 1 || len(m.Results) != 1 || m.Results[0].Outcome != cleanup.Completed {
		t.Fatal("unlocked cleanup retry did not execute exactly once", calls, m.Results)
	}
}

func TestCleanupWithDisabledSQLiteCacheStillRuns(t *testing.T) {
	m := fixture(t)
	m.ConfigPath = ""
	scanNow(t, m)
	calls := 0
	m.Trash = func(context.Context, string) error { calls++; return nil }
	highlightPath(t, m, filepath.Join(m.Root, "cache"))
	m.Key("space")
	m.Key("d")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("disabled persistence blocked an authorized memory-only cleanup", m.Message)
	}
	m.Apply(job())
	if calls != 1 || len(m.Results) != 1 || m.Results[0].Outcome != cleanup.Completed {
		t.Fatal("memory-only cleanup did not execute", calls, m.Results)
	}
}
