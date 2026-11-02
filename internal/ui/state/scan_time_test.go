package state

import (
	"context"
	"diskcleaner/internal/scan"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLastScanTimeRecordsSuccessAndRestoresOriginalCachedTime(t *testing.T) {
	m := fixture(t)
	a, b := m.Root, filepath.Join(m.Root, "cache")
	if !m.LastScanAt.IsZero() {
		t.Fatal("unscanned model has a scan time")
	}
	before := time.Now()
	scanNow(t, m)
	aTime := m.LastScanAt
	if aTime.Before(before) || aTime.After(time.Now()) || aTime.IsZero() {
		t.Fatal("successful scan did not record its completion time", aTime)
	}
	m.navigate(b)
	if !m.LastScanAt.IsZero() {
		t.Fatal("unscanned folder inherited another root's time")
	}
	scanNow(t, m)
	bTime := m.LastScanAt
	m.navigate(a)
	if !m.LastScanAt.Equal(aTime) {
		t.Fatal("navigation changed the original scan time")
	}
	before = time.Now()
	scanNow(t, m)
	if !m.LastScanAt.After(aTime) || m.LastScanAt.Before(before) || m.LastScanAt.After(time.Now()) {
		t.Fatal("rescan did not replace its completion time")
	}
	aTime = m.LastScanAt
	m.navigate(b)
	if !m.LastScanAt.Equal(bTime) {
		t.Fatal("rescan changed another root's completion time")
	}
	m.navigate(a)
	if !m.LastScanAt.Equal(aTime) {
		t.Fatal("rescan time did not survive a later cache restore")
	}
	fresh := New(a, m.Settings, m.Guard, m.ConfigPath)
	if !fresh.LastScanAt.Equal(aTime) {
		t.Fatal("persistent cache did not preserve original completion time", fresh.LastScanAt, aTime)
	}
}

func TestLastScanTimeClearsWithInvalidatedMeasurements(t *testing.T) {
	for _, action := range []string{"failure", "cancellation", "masks"} {
		t.Run(action, func(t *testing.T) {
			m := fixture(t)
			a, b := m.Root, filepath.Join(m.Root, "cache")
			scanNow(t, m)
			aTime := m.LastScanAt
			m.navigate(b)
			scanNow(t, m)
			switch action {
			case "failure", "cancellation":
				m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
					err := errors.New("unreadable")
					if action == "cancellation" {
						err = ctx.Err()
					}
					return scan.Snapshot{Err: err}
				}
				m.Key("s")
				job := m.Key("enter")
				if action == "cancellation" {
					m.Key("esc")
				}
				m.Apply(job())
			case "masks":
				m.toggleMask("unused-pattern")
			}
			if !m.LastScanAt.IsZero() {
				t.Fatal("invalidated scan retained completion time")
			}
			m.navigate(a)
			if action == "failure" || action == "cancellation" {
				if !m.LastScanAt.Equal(aTime) {
					t.Fatal("failed rescan erased another root's time")
				}
			} else if !m.LastScanAt.IsZero() {
				t.Fatal("global invalidation retained another root's time")
			}
			m.navigate(b)
			if !m.LastScanAt.IsZero() {
				t.Fatal("revisit restored invalidated completion time")
			}
		})
	}
}
