package scan

import (
	"context"
	"diskcleaner/internal/ignore"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestListImmediateChildren(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, ".hidden\n"), nil, 0600)
	os.Mkdir(filepath.Join(d, "nested"), 0700)
	os.WriteFile(filepath.Join(d, "nested", "child"), nil, 0600)
	es, e := List(d)
	if e != nil || len(es) != 2 || es[0].Name != ".hidden\n" {
		t.Fatalf("%+v %v", es, e)
	}
}
func TestDuSummary(t *testing.T) {
	for _, tc := range []struct {
		out   string
		err   error
		state State
		bytes uint64
	}{{"8\todd\nname\n", nil, Complete, 4096}, {"8\tx", errors.New("denied"), Partial, 4096}, {"", errors.New("denied"), Unknown, 0}, {"bad\tx", nil, Unknown, 0}, {"18446744073709551615\tx", nil, Unknown, 0}} {
		m := parseSummary([]byte(tc.out), tc.err)
		if m.State != tc.state || m.Bytes != tc.bytes {
			t.Fatalf("%+v", m)
		}
	}
}
func TestDuIgnoresInheritedBlocksize(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "file\nname")
	os.WriteFile(p, []byte("data"), 0600)
	i, _ := os.Lstat(p)
	want := uint64(i.Sys().(*syscall.Stat_t).Blocks) * 512
	for _, v := range []string{"512", "1024", "1m"} {
		t.Setenv("BLOCKSIZE", v)
		m := measure(context.Background(), p, nil)
		if m.State != Complete || m.Bytes != want {
			t.Fatalf("%s: %+v want %d", v, m, want)
		}
		if os.Getenv("BLOCKSIZE") != v {
			t.Fatal("changed environment")
		}
	}
}
func TestAnalyzeOneLevelAndCancel(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "file"), []byte("a"), 0600)
	os.Mkdir(filepath.Join(d, "cache"), 0700)
	s := Analyze(context.Background(), d, []string{"cache"})
	if len(s.Entries) != 2 || s.Entries[filepath.Join(d, "cache")].State != Skipped {
		t.Fatalf("%+v", s)
	}
	ctx, c := context.WithCancel(context.Background())
	c()
	s = Analyze(ctx, d, nil)
	if s.Err == nil || len(s.Entries) != 0 {
		t.Fatal("cancelled snapshot exposed")
	}
}

func TestDuSparseHardlinksAndSymlink(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "sparse")
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	f.Truncate(16 << 20)
	f.Close()
	info, _ := os.Lstat(p)
	if m := measure(context.Background(), p, nil); m.State != Complete || m.Bytes != uint64(info.Sys().(*syscall.Stat_t).Blocks)*512 {
		t.Fatal(m)
	}
	os.WriteFile(p, []byte("data"), 0600)
	os.Link(p, filepath.Join(d, "hardlink"))
	m := measure(context.Background(), d, nil)
	if m.State != Complete {
		t.Fatal(m)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "large"), make([]byte, 1<<20), 0600)
	link := filepath.Join(d, "link")
	os.Symlink(outside, link)
	li, _ := os.Lstat(link)
	m = measure(context.Background(), link, nil)
	if m.State != Complete || m.Bytes != uint64(li.Sys().(*syscall.Stat_t).Blocks)*512 {
		t.Fatal(m)
	}
}
func TestMaskParityWithSystemDu(t *testing.T) {
	for _, name := range []string{"cache", ".cache", "a*b?[]\\", "café"} {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			p := filepath.Join(d, name)
			os.WriteFile(p, make([]byte, 65536), 0600)
			m := measure(context.Background(), d, []string{ignore.Literal(name)})
			os.Remove(p)
			empty := measure(context.Background(), d, nil)
			if m.State != Complete || m.Bytes != empty.Bytes {
				t.Fatalf("%q: %+v vs %+v", name, m, empty)
			}
		})
	}
}

func TestUnicodeMaskParityIgnoresInheritedLocale(t *testing.T) {
	for _, locale := range []string{"C", "en_US.UTF-8", "invalid-locale"} {
		t.Run(locale, func(t *testing.T) {
			t.Setenv("LC_ALL", locale)
			t.Setenv("LANG", locale)
			t.Setenv("LC_CTYPE", "C")
			for _, name := range []string{"éache", "😀ache", "e\u0301ache"} {
				d := t.TempDir()
				if err := os.WriteFile(filepath.Join(d, name), make([]byte, 65536), 0600); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(d)
				if err != nil {
					t.Fatal(err)
				}
				actualName := entries[0].Name()
				baseline := measure(context.Background(), d, nil)
				for _, mask := range []string{"?ache", "??ache", "*ache"} {
					got := measure(context.Background(), d, []string{mask})
					if baseline.State != Complete || got.State != Complete {
						t.Fatalf("baseline=%+v filtered=%+v", baseline, got)
					}
					if excluded := got.Bytes < baseline.Bytes; excluded != ignore.Match(mask, actualName) {
						t.Errorf("name=%q mask=%q: du excluded=%v, Go matched=%v", actualName, mask, excluded, ignore.Match(mask, actualName))
					}
				}
			}
			if os.Getenv("LC_ALL") != locale || os.Getenv("LANG") != locale || os.Getenv("LC_CTYPE") != "C" {
				t.Fatal("changed parent locale")
			}
		})
	}
}
func TestAnalyzeDeviceBoundaryAndError(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"mount", "unknown", "ok"} {
		os.WriteFile(filepath.Join(d, n), nil, 0600)
	}
	calls := 0
	s := analyzeWith(context.Background(), d, nil, func(_ context.Context, p string, _ []string) Measurement {
		calls++
		if filepath.Base(p) == "unknown" {
			return Measurement{State: Unknown, Err: errors.New("denied")}
		}
		return Measurement{Bytes: 512, State: Complete}
	}, func(p string) (uint64, error) {
		if filepath.Base(p) == "mount" {
			return 2, nil
		}
		return 1, nil
	})
	if calls != 2 || s.Entries[filepath.Join(d, "mount")].State != Skipped || s.Entries[filepath.Join(d, "ok")].Bytes != 512 || s.State != Partial {
		t.Fatal(s, calls)
	}
}
