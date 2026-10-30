package cleanup

import (
	"context"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"fmt"
	"os/exec"
	"strings"
)

type Outcome uint8

const (
	Completed Outcome = iota
	Skipped
	Failed
)

func (o Outcome) String() string {
	switch o {
	case Completed:
		return "Completed"
	case Skipped:
		return "Skipped"
	default:
		return "Failed — changes may have occurred"
	}
}

type Result struct {
	Path    string
	Outcome Outcome
	Err     error
}
type Runner func(context.Context, string) error

func run(ctx context.Context, command string, args ...string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	out, e := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("%s: %w: %s", command, e, strings.TrimSpace(string(out)))
	}
	return nil
}
func SystemTrash(ctx context.Context, p string) error { return run(ctx, "/usr/bin/trash", "-s", p) }
func SystemRemove(ctx context.Context, p string) error {
	return run(ctx, "/bin/rm", "-r", "-x", "--", p)
}
func Execute(ctx context.Context, mode config.DeleteMode, selected []Target, guard *policy.Guard, trash Runner, remove Runner) []Result {
	results := make([]Result, 0, len(selected))
	for _, t := range selected {
		r := Result{Path: t.Path, Outcome: Skipped}
		var runner Runner
		switch mode {
		case config.Trash:
			runner = trash
		case config.Permanent:
			runner = remove
		}
		e := ctx.Err()
		if e == nil && runner == nil {
			e = fmt.Errorf("invalid mode or unavailable executor")
		}
		if e == nil {
			e = unchanged(t)
		}
		if e == nil {
			e = guard.CheckTree(ctx, t.Path)
		}
		if e == nil {
			e = unchanged(t)
		}
		if e == nil {
			e = ctx.Err()
		}
		if e != nil {
			r.Err = e
			results = append(results, r)
			continue
		}
		e = runner(ctx, t.Path)
		r.Outcome = Completed
		if e != nil {
			r.Outcome = Failed
			r.Err = e
		}
		results = append(results, r)
	}
	return results
}
