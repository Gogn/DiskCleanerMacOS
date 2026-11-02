package cleanup

import (
	"context"
	"diskcleaner/internal/config"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusionOverrideIsIdentityBoundAndNeverFallsBack(t *testing.T) {
	for _, scenario := range []string{"approved", "unapproved", "replacement", "cancelled", "trash-failed"} {
		t.Run(scenario, func(t *testing.T) {
			root, guard, target := setup(t)
			if err := os.Mkdir(filepath.Join(target.Path, "cache"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := guard.SetContext(root, []string{"cache"}); err != nil {
				t.Fatal(err)
			}
			approved := map[Target]bool{target: true}
			ctx := context.Background()
			if scenario == "unapproved" {
				approved = nil
			}
			if scenario == "replacement" {
				if err := os.Rename(target.Path, target.Path+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(target.Path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			calls, removes := 0, 0
			trash := func(context.Context, string) error {
				calls++
				if scenario == "trash-failed" {
					return errors.New("failure")
				}
				return nil
			}
			remove := func(context.Context, string) error { removes++; return nil }
			results := ExecuteWithExclusionOverrides(ctx, config.Trash, []Target{target}, guard, trash, remove, approved)
			wantCalls := 0
			wantOutcome := Skipped
			if scenario == "approved" {
				wantCalls = 1
				wantOutcome = Completed
			}
			if scenario == "trash-failed" {
				wantCalls = 1
				wantOutcome = Failed
			}
			if calls != wantCalls || removes != 0 || len(results) != 1 || results[0].Outcome != wantOutcome {
				t.Fatal(calls, removes, results)
			}
		})
	}
}
