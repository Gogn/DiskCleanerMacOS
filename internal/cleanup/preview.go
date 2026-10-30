package cleanup

import (
	"context"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/scan"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

type Target struct {
	Path     string
	Dev, Ino uint64
	IsDir    bool
}

func Capture(p string) (Target, error) {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return Target{}, fmt.Errorf("target must be a clean absolute path")
	}
	i, e := os.Lstat(p)
	if e != nil {
		return Target{}, e
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return Target{}, fmt.Errorf("identity unavailable")
	}
	return Target{p, uint64(s.Dev), s.Ino, i.IsDir()}, nil
}
func unchanged(t Target) error {
	current, e := Capture(t.Path)
	if e != nil {
		return e
	}
	if current != t {
		return fmt.Errorf("selected object was replaced")
	}
	return nil
}

type Preview struct {
	Targets        []Target
	Paths          []string
	EstimatedBytes uint64
	EstimateState  scan.State
}

func unique(ts []Target) []Target {
	out := []Target{}
	seen := map[string]bool{}
	for _, t := range ts {
		if !seen[t.Path] {
			out = append(out, t)
			seen[t.Path] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Estimate uses only the last scan; it never measures actual reclaimed space.
func Estimate(ts []Target, sizes scan.Snapshot) (uint64, scan.State) {
	var n uint64
	known, unknown, partial := false, false, false
	for _, t := range unique(ts) {
		m, ok := sizes.Entries[t.Path]
		if !ok || m.State == scan.Unknown || m.State == scan.Pending || m.State == scan.Skipped {
			unknown = true
			continue
		}
		known = true
		if m.State == scan.Partial {
			partial = true
		}
		if math.MaxUint64-n < m.Bytes {
			return 0, scan.Unknown
		}
		n += m.Bytes
	}
	if !known && unknown {
		return n, scan.Unknown
	}
	if unknown || partial {
		return n, scan.Partial
	}
	return n, scan.Complete
}
func BuildPreview(ctx context.Context, selected []Target, guard *policy.Guard, sizes scan.Snapshot) (Preview, error) {
	p := Preview{Targets: unique(selected)}
	p.EstimatedBytes, p.EstimateState = Estimate(p.Targets, sizes)
	for _, t := range p.Targets {
		if e := ctx.Err(); e != nil {
			return Preview{}, e
		}
		if e := unchanged(t); e != nil {
			return Preview{}, e
		}
		if e := guard.CheckTree(ctx, t.Path); e != nil {
			return Preview{}, e
		}
		e := filepath.WalkDir(t.Path, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			p.Paths = append(p.Paths, path)
			return nil
		})
		if e != nil {
			return Preview{}, e
		}
		if e = unchanged(t); e != nil {
			return Preview{}, e
		}
	}
	return p, nil
}
