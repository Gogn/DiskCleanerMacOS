package state

import (
	"context"
	"database/sql"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exclusionFixture(t *testing.T) (*Model, string, string, *int) {
	t.Helper()
	m := fixture(t)
	folder := filepath.Join(m.Root, "folder")
	if err := os.MkdirAll(filepath.Join(folder, "excluded"), 0700); err != nil {
		t.Fatal(err)
	}
	m.toggleMask("excluded")
	scanNow(t, m)
	m.Selected = map[string]cleanup.Target{}
	sibling := filepath.Join(m.Root, "file\n\x1b")
	for _, path := range []string{folder, sibling} {
		target, err := cleanup.Capture(path)
		if err != nil {
			t.Fatal(err)
		}
		m.Selected[path] = target
	}
	calls := new(int)
	m.Trash = func(context.Context, string) error { *calls++; return nil }
	m.Remove = func(context.Context, string) error { t.Error("unexpected permanent runner"); return nil }
	m.Key("d")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("missing cleanup job")
	}
	m.Apply(job())
	if m.Screen != "exclusion-confirm" || *calls != 1 {
		t.Fatal("missing separate confirmation", m.Screen, *calls, m.Results)
	}
	return m, folder, sibling, calls
}

func TestExcludedFolderConfirmationDefaultsToLeave(t *testing.T) {
	for _, key := range []string{"enter", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, _, _, calls := exclusionFixture(t)
			if !strings.Contains(m.View(), "[x] Leave folder untouched") || !strings.Contains(m.View(), "ALL excluded contents") {
				t.Fatal(m.View())
			}
			if job := m.Key(key); job != nil {
				t.Fatal("default leave started cleanup")
			}
			if *calls != 1 || m.Screen != "results" {
				t.Fatal(m.Screen, *calls)
			}
		})
	}
}

func TestExcludedFolderRetryIsScopedAndIdentityChecked(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "replacement"}[replace], func(t *testing.T) {
			m, folder, sibling, calls := exclusionFixture(t)
			if replace {
				if err := os.Rename(folder, folder+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(folder, 0700); err != nil {
					t.Fatal(err)
				}
			}
			m.Key("right")
			job := m.Key("enter")
			if job == nil {
				t.Fatal("missing retry")
			}
			m.Apply(job())
			want := 2
			if replace {
				want = 1
			}
			if *calls != want || m.Screen != "results" || len(m.Results) != 2 {
				t.Fatal(*calls, m.Screen, m.Results)
			}
			for _, r := range m.Results {
				if r.Path == sibling && r.Outcome != cleanup.Completed {
					t.Fatal("completed sibling lost", r)
				}
			}
			if len(m.Masks()) != 1 || m.Masks()[0] != "excluded" || m.ConfirmedMode != config.Trash {
				t.Fatal("override changed mode or masks")
			}
		})
	}
}

func TestCancelledCleanupDoesNotOfferExclusionOverride(t *testing.T) {
	m, folder, _, _ := exclusionFixture(t)
	// Reapply the already computed skipped result as a cancelled worker result.
	var skipped cleanup.Result
	for _, r := range m.Results {
		if r.Path == folder {
			skipped = r
		}
	}
	m.cancelRequested = true
	m.Apply(Event{Kind: "cleanup", Results: []cleanup.Result{skipped}})
	if m.Screen != "results" || len(m.exclusionQueue) != 0 {
		t.Fatal("cancelled operation offered a retry")
	}
}

func TestExcludedFolderChoiceStaysVisibleWhileScrolling(t *testing.T) {
	m, folder, _, _ := exclusionFixture(t)
	m.Width = 200
	if !strings.Contains(m.View(), Safe(filepath.Join(folder, "excluded"))) {
		t.Fatal("missing triggering exclusion", m.View())
	}
	m.Width, m.Height = 32, 12
	for _, key := range []string{"", "pgdown", "end", "home"} {
		if key != "" {
			m.Key(key)
		}
		if !strings.Contains(m.View(), "Choice: LEAVE folder untouched") {
			t.Fatal("default choice hidden", m.View())
		}
	}
	m.Key("right")
	m.Key("end")
	if !strings.Contains(m.View(), "Choice: RETRY ALL (TRASH)") {
		t.Fatal("retry choice hidden", m.View())
	}
}

func TestNonExclusionFailureDoesNotOfferOverride(t *testing.T) {
	m, folder, _, _ := exclusionFixture(t)
	m.Apply(Event{Kind: "cleanup", Results: []cleanup.Result{{Path: folder, Outcome: cleanup.Skipped, Err: context.DeadlineExceeded}}})
	if m.Screen != "results" || len(m.exclusionQueue) != 0 {
		t.Fatal("non-exclusion failure offered retry")
	}
}

func TestExcludedFolderQueueLeavesOneAndRetriesOnlyNext(t *testing.T) {
	m := fixture(t)
	folders := []string{filepath.Join(m.Root, "first"), filepath.Join(m.Root, "second")}
	for _, folder := range folders {
		if err := os.MkdirAll(filepath.Join(folder, "excluded"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	m.toggleMask("excluded")
	scanNow(t, m)
	sibling := filepath.Join(m.Root, "file\n\x1b")
	for _, path := range append(folders, sibling) {
		target, err := cleanup.Capture(path)
		if err != nil {
			t.Fatal(err)
		}
		m.Selected[path] = target
	}
	calls := map[string]int{}
	m.Trash = func(_ context.Context, path string) error { calls[path]++; return nil }
	m.Key("d")
	job := m.Key("enter")
	m.Apply(job())
	if len(m.exclusionQueue) != 2 {
		t.Fatal("missing folder questions", m.Results)
	}
	first := m.exclusionQueue[0].Path
	second := m.exclusionQueue[1].Path
	if job := m.Key("enter"); job != nil {
		t.Fatal("leave ran cleanup")
	}
	if m.Screen != "exclusion-confirm" || m.exclusionDelete || m.exclusionQueue[0].Path != second {
		t.Fatal("next question did not default to leave")
	}
	m.Key("right")
	job = m.Key("enter")
	m.Apply(job())
	if calls[first] != 0 || calls[second] != 1 || calls[sibling] != 1 || m.Screen != "results" {
		t.Fatal(calls, m.Screen)
	}
	if len(m.Results) != 3 {
		t.Fatal("lost per-item results", m.Results)
	}
}

func TestExcludedFolderBusyCacheShowsReasonAndRetainsScopedRetry(t *testing.T) {
	m, folder, sibling, calls := exclusionFixture(t)
	m.Width, m.Height = 32, 12
	m.Key("end")
	db, err := sql.Open("sqlite", m.scanCachePath())
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
	m.Key("right")
	if job := m.Key("enter"); job != nil {
		t.Fatal("locked cache started an override worker")
	}
	if m.Busy || *calls != 1 || len(m.exclusionQueue) != 1 || m.exclusionQueue[0].Path != folder || m.exclusionDelete {
		t.Fatal("failed override lost its original target or default-leave choice", m.Screen, m.exclusionQueue, *calls)
	}
	// Exercise rendering a deeply nested root without changing the worker's
	// real fixture paths or guard context.
	root := m.Root
	m.Root = filepath.Join(root, strings.Repeat("deep-directory/", 8))
	view := m.View()
	m.Root = root
	if !strings.Contains(plainView(view), "cleanup was not started") {
		t.Fatal("busy-cache reason is hidden in the narrow confirmation viewport", view)
	}
	if _, err := conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	var retried []string
	m.Trash = func(_ context.Context, path string) error { retried = append(retried, path); return nil }
	m.Key("right")
	job := m.Key("enter")
	if job == nil {
		t.Fatal("released cache lock still blocked override retry", m.Message)
	}
	m.Apply(job())
	if len(retried) != 1 || retried[0] != folder || m.Screen != "results" {
		t.Fatal("retry was not scoped to original folder", retried, m.Screen)
	}
	for _, result := range m.Results {
		if (result.Path == folder || result.Path == sibling) && result.Outcome != cleanup.Completed {
			t.Fatal("retry lost per-item report", result)
		}
	}
}
