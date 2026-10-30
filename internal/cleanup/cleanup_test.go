package cleanup

import (
	"context"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) (string, *policy.Guard, Target) {
	t.Helper()
	d, err := os.MkdirTemp(".", "cleanup-test-")
	if err != nil {
		t.Fatal(err)
	}
	d, err = filepath.Abs(d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	home, _ := os.UserHomeDir()
	root := filepath.Join(d, "work")
	os.MkdirAll(root, 0700)
	p := filepath.Join(root, "selected")
	os.Mkdir(p, 0700)
	os.WriteFile(filepath.Join(p, "file"), []byte("test"), 0600)
	g, e := policy.NewGuard(home, filepath.Join(d, "config"), root, nil)
	if e != nil {
		t.Fatal(e)
	}
	target, e := Capture(p)
	if e != nil {
		t.Fatal(e)
	}
	return root, g, target
}
func TestPreviewIsReadOnly(t *testing.T) {
	_, g, target := setup(t)
	s := scan.Snapshot{Entries: map[string]scan.Measurement{target.Path: {Bytes: 42, State: scan.Partial}}}
	p, e := BuildPreview(context.Background(), []Target{target, target}, g, s)
	if e != nil || len(p.Paths) != 2 || p.EstimatedBytes != 42 || p.EstimateState != scan.Partial {
		t.Fatalf("%+v %v", p, e)
	}
	if _, e = os.Stat(filepath.Join(target.Path, "file")); e != nil {
		t.Fatal(e)
	}
}
func TestExecuteRechecksAndReportsEachTarget(t *testing.T) {
	root, g, target := setup(t)
	os.Rename(target.Path, target.Path+"-old")
	os.Mkdir(target.Path, 0700)
	calls := 0
	r := func(context.Context, string) error { calls++; return nil }
	out := Execute(context.Background(), config.Permanent, []Target{target}, g, r, r)
	if calls != 0 || out[0].Outcome != Skipped {
		t.Fatal(out)
	}
	target, _ = Capture(target.Path)
	os.WriteFile(filepath.Join(target.Path, "cache"), nil, 0600)
	g.SetContext(root, []string{"cache"})
	out = Execute(context.Background(), config.Permanent, []Target{target}, g, r, r)
	if calls != 0 || out[0].Outcome != Skipped {
		t.Fatal(out)
	}
	g.SetContext(root, nil)
	out = Execute(context.Background(), config.Permanent, []Target{target}, g, r, SystemRemove)
	if out[0].Outcome != Completed {
		t.Fatal(out)
	}
	if _, e := os.Stat(target.Path); !os.IsNotExist(e) {
		t.Fatal("still exists", e)
	}
}
func TestTrashFailureNeverPermanentlyDeletes(t *testing.T) {
	_, g, target := setup(t)
	removed := false
	out := Execute(context.Background(), config.Trash, []Target{target}, g, func(context.Context, string) error { return errors.New("trash failed") }, func(context.Context, string) error { removed = true; return nil })
	if removed || out[0].Outcome != Failed {
		t.Fatal(out)
	}
	if _, e := os.Stat(target.Path); e != nil {
		t.Fatal(e)
	}
}
func TestExecuteOutcomesAndCancellation(t *testing.T) {
	root, g, target := setup(t)
	p := filepath.Join(root, "second")
	os.WriteFile(p, nil, 0600)
	second, _ := Capture(p)
	ctx, c := context.WithCancel(context.Background())
	out := Execute(ctx, config.Permanent, []Target{target, second}, g, nil, func(ctx context.Context, p string) error {
		os.Remove(filepath.Join(p, "file"))
		c()
		return errors.New("interrupted")
	})
	if len(out) != 2 || out[0].Outcome != Failed || out[1].Outcome != Skipped {
		t.Fatal(out)
	}
	if _, e := os.Stat(second.Path); e != nil {
		t.Fatal(e)
	}
}
func TestSystemRemove(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "-file")
	os.WriteFile(p, nil, 0600)
	link := filepath.Join(d, "link")
	os.Symlink(p, link)
	if e := SystemRemove(context.Background(), link); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(p); e != nil {
		t.Fatal(e)
	}
	if e := SystemRemove(context.Background(), p); e != nil {
		t.Fatal(e)
	}
}
func TestSystemTrashCancelledKeepsFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	os.WriteFile(p, nil, 0600)
	ctx, c := context.WithCancel(context.Background())
	c()
	for _, r := range []Runner{SystemTrash, SystemRemove} {
		if r(ctx, p) == nil {
			t.Fatal("cancel ignored")
		}
		if _, e := os.Stat(p); e != nil {
			t.Fatal(e)
		}
	}
}

func TestUnicodeExclusionsAgreeWithCleanupInCLocale(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	root, guard, target := setup(t)
	if err := os.WriteFile(filepath.Join(target.Path, "éache"), make([]byte, 65536), 0600); err != nil {
		t.Fatal(err)
	}
	baseline := scan.Analyze(context.Background(), root, nil).Entries[target.Path]
	for _, tc := range []struct {
		mask     string
		excluded bool
	}{{"?ache", true}, {"??ache", false}} {
		t.Run(tc.mask, func(t *testing.T) {
			masks := []string{tc.mask}
			if err := guard.SetContext(root, masks); err != nil {
				t.Fatal(err)
			}
			measured := scan.Analyze(context.Background(), root, masks).Entries[target.Path]
			if baseline.State != scan.Complete || measured.State != scan.Complete || (measured.Bytes < baseline.Bytes) != tc.excluded {
				t.Fatalf("unexpected measurement: baseline=%+v filtered=%+v", baseline, measured)
			}
			called := false
			results := Execute(context.Background(), config.Permanent, []Target{target}, guard, nil, func(context.Context, string) error {
				called = true
				return nil
			})
			if called == tc.excluded || (results[0].Outcome == Skipped) != tc.excluded {
				t.Fatalf("scan and cleanup disagree: called=%v result=%+v", called, results[0])
			}
		})
	}
}
