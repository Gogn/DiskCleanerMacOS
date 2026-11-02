package state

import (
	"context"
	"database/sql"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lockReviewCache(t *testing.T, m *Model) func() {
	t.Helper()
	db, err := sql.Open("sqlite", m.scanCachePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("BEGIN IMMEDIATE"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_, err := db.Exec("ROLLBACK")
			db.Close()
			if err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(release)
	return release
}

func TestSQLiteTransientReadFailureDoesNotEvictSavedRoot(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	root := m.Root
	when := m.LastScanAt
	release := lockReviewCache(t, m)
	m.navigate(root)
	if m.Scanned {
		t.Fatal("unverified cache was displayed during read failure")
	}
	release()
	m.navigate(filepath.Join(root, "cache"))
	scanNow(t, m)
	fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !fresh.LastScanAt.Equal(when) {
		t.Fatal("temporary read failure erased another root's saved snapshot")
	}
}

func TestSQLiteUnsavedSuccessfulScanSurvivesNavigationAfterLock(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("s")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("missing scan job")
	}
	event := job()
	release := lockReviewCache(t, m)
	m.Apply(event)
	if !m.Scanned || !m.cacheWriteFailed {
		t.Fatal("write failure lost successful in-memory scan")
	}
	when := m.LastScanAt
	release()
	m.navigate(m.Root)
	if !m.Scanned || !m.LastScanAt.Equal(when) {
		t.Fatal("navigation replaced unsaved successful scan with older disk state")
	}
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !fresh.LastScanAt.Equal(when) {
		t.Fatal("recovered storage did not save the retained scan")
	}
}

func TestSQLiteLateFailedScanCannotDeleteNewerSnapshot(t *testing.T) {
	for _, failure := range []error{context.Canceled, errors.New("worker failed")} {
		t.Run(failure.Error(), func(t *testing.T) {
			m := fixture(t)
			scanNow(t, m)
			late := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			late.Key("s")
			job := late.Key("enter")
			if job == nil {
				t.Fatal("missing scan job")
			}
			event := job()
			event.Snapshot.Err = failure
			newer := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			scanNow(t, newer)
			when := newer.LastScanAt
			late.navigate(late.Root)
			late.Apply(event)
			fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
			if !fresh.Scanned || !fresh.LastScanAt.Equal(when) {
				t.Fatal("late failure removed the newer authoritative snapshot")
			}
		})
	}
}

func TestSQLiteCleanupLockBlocksConcurrentPersistenceWhenFinalInvalidationFails(t *testing.T) {
	m := fixture(t)
	victim := filepath.Join(m.Root, "cache")
	child := filepath.Join(victim, "nested")
	if err := os.WriteFile(child, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	concurrent := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	highlightPath(t, m, victim)
	m.Key("space")
	var releaseDB func()
	m.Trash = func(context.Context, string) error {
		scanNow(t, concurrent)
		if !concurrent.scanCache[concurrent.Root].volatile {
			t.Fatal("scan started during cleanup was persistable")
		}
		if err := os.WriteFile(child, []byte("deep content changed after concurrent measurement"), 0600); err != nil {
			t.Fatal(err)
		}
		releaseDB = lockReviewCache(t, m)
		return errors.New("runner changed a descendant and failed")
	}
	m.Key("d")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("missing cleanup job")
	}
	m.Apply(job())
	if m.cleanupCacheLock != nil {
		t.Fatal("completion leaked exclusive cache lock")
	}
	if !strings.Contains(m.Message, "cache refresh failed") {
		t.Fatal("failed final invalidation was not reported", m.Message)
	}
	releaseDB()
	// A successful but unverified measurement must not become durable after the
	// runner finishes merely because storage is available again.
	concurrent.persistScanCache()
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || fresh.Snapshot.Entries[victim].State != scan.Unknown || fresh.Snapshot.Entries[victim].Bytes != 0 {
		t.Fatal("concurrent scan revived invalidated size", fresh.Snapshot)
	}
	lock, err := fresh.lockScanCache(true)
	if err != nil {
		t.Fatal("cleanup lock was not released", err)
	}
	unlockScanCache(lock)
	info, err := os.Stat(m.scanCachePath() + ".lock")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("lock file is not private", err)
	}
}

func TestSQLiteLateCaptureFailureCannotDeleteNewerSnapshot(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("s")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("missing scan job")
	}
	root := m.Root
	if err := os.Rename(root, root+"-held"); err != nil {
		t.Fatal(err)
	}
	event := job()
	if err := os.Rename(root+"-held", root); err != nil {
		t.Fatal(err)
	}
	if event.Snapshot.Err == nil || event.cacheToken == nil {
		t.Fatal("capture failure lost immutable token")
	}
	newer := New(root, m.Settings, m.Guard, m.ConfigPath)
	scanNow(t, newer)
	when := newer.LastScanAt
	m.navigate(root)
	m.Apply(event)
	fresh := New(root, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.Scanned || !fresh.LastScanAt.Equal(when) {
		t.Fatal("capture failure removed newer saved scan")
	}
}
