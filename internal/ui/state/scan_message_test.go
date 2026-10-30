package state

import (
	"context"
	"diskcleaner/internal/scan"
	"errors"
	"strings"
	"testing"
)

func TestScanConfirmationClearsQuickExclusionMessage(t *testing.T) {
	m := fixture(t)
	m.setFilter("cache")
	m.Key("E")
	message := m.Message
	if !strings.Contains(message, "Excluded all items named cache") || !strings.Contains(m.View(), message) {
		t.Fatal("quick exclusion did not display its notification", m.View())
	}
	if job := m.Key("s"); job != nil || m.Screen != "scan-confirm" {
		t.Fatal("opening scan confirmation started work or opened the wrong screen")
	}
	for _, key := range []string{"", "?", "esc", "esc"} {
		if key != "" {
			m.Key(key)
		}
		if m.Message != "" || strings.Contains(m.View(), message) {
			t.Fatal("stale exclusion notification survived scan confirmation", m.View())
		}
	}
	if m.Screen != "list" || m.Busy {
		t.Fatal("cancelling scan confirmation did not return to the browser")
	}
}

func TestScanStillDisplaysNewFailureAndCancellationMessages(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancellation"}[cancelled], func(t *testing.T) {
			m := fixture(t)
			m.Message = "Settings saved."
			failure := errors.New("cannot read directory")
			m.Analyze = func(ctx context.Context, root string, masks []string, report func(scan.Progress)) scan.Snapshot {
				if cancelled {
					return scan.Snapshot{Err: ctx.Err()}
				}
				return scan.Snapshot{Err: failure}
			}
			m.Key("s")
			job := m.Key("enter")
			if job == nil {
				t.Fatal("no scan job")
			}
			if cancelled {
				m.Key("esc")
				if !strings.Contains(m.View(), "Cancelling; waiting for the operation to finish...") {
					t.Fatal("cancellation notification missing", m.View())
				}
				failure = context.Canceled
			}
			m.Apply(job())
			want := "Scan failed or cancelled: " + failure.Error()
			if m.Screen != "list" || m.Message != want || !strings.Contains(m.View(), want) {
				t.Fatal("new scan notification missing", m.View())
			}
		})
	}
}
