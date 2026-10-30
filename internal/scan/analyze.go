package scan

import (
	"context"
	"diskcleaner/internal/ignore"
	"fmt"
	"os"
	"syscall"
)

func device(p string) (uint64, error) {
	i, e := os.Lstat(p)
	if e != nil {
		return 0, e
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("device metadata unavailable")
	}
	return uint64(s.Dev), nil
}
func Analyze(ctx context.Context, root string, masks []string) Snapshot {
	return analyzeWith(ctx, root, masks, measure, device)
}

// AnalyzeWithProgress reports immediate-item progress synchronously. Item count
// is not a time or byte percentage: one directory can take much longer to scan.
func AnalyzeWithProgress(ctx context.Context, root string, masks []string, report func(Progress)) Snapshot {
	return analyzeWith(ctx, root, masks, measure, device, report)
}

func analyzeWith(ctx context.Context, root string, masks []string, measureFn func(context.Context, string, []string) Measurement, deviceFn func(string) (uint64, error), reports ...func(Progress)) Snapshot {
	report := func(p Progress) {
		if len(reports) > 0 && reports[0] != nil {
			reports[0](p)
		}
	}
	fail := func(e error) Snapshot { return Snapshot{State: Unknown, Err: e} }
	if e := ctx.Err(); e != nil {
		return fail(e)
	}
	for _, m := range masks {
		if e := ignore.Validate(m); e != nil {
			return fail(e)
		}
	}
	es, e := List(root)
	if e != nil {
		return fail(e)
	}
	dev, e := deviceFn(root)
	if e != nil {
		return fail(e)
	}
	s := Snapshot{Listing: es, Entries: map[string]Measurement{}, State: Complete}
	report(Progress{Total: len(es)})
	for index, v := range es {
		if e = ctx.Err(); e != nil {
			return fail(e)
		}
		report(Progress{Current: v.Path, Completed: index, Total: len(es)})
		now, e := deviceFn(root)
		if e != nil {
			return fail(e)
		}
		if now != dev {
			return fail(fmt.Errorf("scan root device changed"))
		}
		d, e := deviceFn(v.Path)
		var m Measurement
		switch {
		case e != nil:
			m = Measurement{State: Unknown, Err: e}
		case d != dev:
			m = Measurement{State: Skipped, Err: fmt.Errorf("different volume")}
		case ignore.Any(masks, v.Name):
			m = Measurement{State: Skipped, Err: fmt.Errorf("excluded name")}
		default:
			m = measureFn(ctx, v.Path, masks)
		}
		if e = ctx.Err(); e != nil {
			return fail(e)
		}
		s.Entries[v.Path] = m
		if m.State == Partial || m.State == Unknown {
			s.State = Partial
		}
		report(Progress{Completed: index + 1, Total: len(es)})
	}
	if e = ctx.Err(); e != nil {
		return fail(e)
	}
	return s
}
