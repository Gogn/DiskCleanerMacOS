package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProgressCountsProcessedItemsIncludingExclusions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var updates []Progress
	measured := 0
	snapshot := analyzeWith(context.Background(), root, []string{"b"}, func(_ context.Context, path string, _ []string) Measurement {
		measured++
		last := updates[len(updates)-1]
		if last.Current != path || last.Completed != 0 || last.Total != 2 {
			t.Fatal("current item not reported before measuring", last)
		}
		return Measurement{State: Complete}
	}, device, func(value Progress) { updates = append(updates, value) })
	if snapshot.Err != nil || measured != 1 || len(updates) != 5 || updates[4].Completed != 2 || updates[4].Current != "" {
		t.Fatal(snapshot, updates)
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates = nil
	snapshot = analyzeWith(ctx, root, nil, func(context.Context, string, []string) Measurement { cancel(); return Measurement{State: Unknown} }, device, func(value Progress) { updates = append(updates, value) })
	if snapshot.Err == nil || updates[len(updates)-1].Completed != 0 || len(snapshot.Entries) != 0 {
		t.Fatal("cancelled item reported complete", snapshot, updates)
	}
}

func TestCancellationAtFinalProgressDiscardsSnapshot(t *testing.T) {
	for _, itemCount := range []int{0, 1} {
		name := "empty root"
		if itemCount != 0 {
			name = "completed final item"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if itemCount != 0 {
				if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			snapshot := analyzeWith(ctx, root, nil, func(context.Context, string, []string) Measurement {
				return Measurement{Bytes: 512, State: Complete}
			}, device, func(progress Progress) {
				if progress.Completed == progress.Total {
					cancel()
				}
			})
			if !errors.Is(snapshot.Err, context.Canceled) || snapshot.State != Unknown || len(snapshot.Entries) != 0 || len(snapshot.Listing) != 0 {
				t.Fatalf("cancelled scan published a completed snapshot: %+v", snapshot)
			}
		})
	}
}
