package policy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExcludedContentsOverrideKeepsOtherGuards(t *testing.T) {
	for _, scenario := range []string{"ordinary", "protected", "outside-link", "excluded-sibling-link", "cancelled", "direct-exclusion"} {
		t.Run(scenario, func(t *testing.T) {
			root, guard := fixture(t)
			selected := filepath.Join(root, "selected")
			if err := os.MkdirAll(filepath.Join(selected, "cache"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := guard.SetContext(root, []string{"cache"}); err != nil {
				t.Fatal(err)
			}
			var excluded *ExcludedNameError
			if err := guard.CheckTree(context.Background(), selected); !errors.As(err, &excluded) || excluded.Path != filepath.Join(selected, "cache") {
				t.Fatal("missing typed descendant exclusion", err)
			}
			ctx := context.Background()
			switch scenario {
			case "protected":
				guard.rules = append(guard.rules, rule{filepath.Join(selected, "cache"), true})
			case "outside-link":
				if err := os.Symlink(t.TempDir(), filepath.Join(selected, "link")); err != nil {
					t.Fatal(err)
				}
			case "excluded-sibling-link":
				if err := os.MkdirAll(filepath.Join(root, "cache"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "cache"), filepath.Join(selected, "link")); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "direct-exclusion":
				selected = filepath.Join(selected, "cache")
				if err := guard.SetContext(filepath.Dir(selected), []string{"cache"}); err != nil {
					t.Fatal(err)
				}
			}
			err := guard.CheckTreeIncludingExcludedContents(ctx, selected)
			if (err == nil) != (scenario == "ordinary") {
				t.Fatalf("unexpected override result: %v", err)
			}
			if !reflect.DeepEqual(guard.IgnoredMasks, []string{"cache"}) {
				t.Fatal("override changed global masks")
			}
		})
	}
}
