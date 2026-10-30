package scan

import (
	"context"
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
