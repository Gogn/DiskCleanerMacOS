package cleanup

import (
	"bytes"
	"context"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"errors"
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

// ErrNotStarted marks a runner error returned before the system command began,
// so no change can have occurred and the item is reported as skipped.
var ErrNotStarted = errors.New("cleanup command was not started")

const maxCommandOutput = 4096

// commandOutput keeps only the beginning of command output, so a tree with many
// errors cannot grow the per-item report without bound.
type commandOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (o *commandOutput) Write(p []byte) (int, error) {
	if room := maxCommandOutput - o.buf.Len(); len(p) > room {
		o.buf.Write(p[:max(room, 0)])
		o.truncated = true
	} else {
		o.buf.Write(p)
	}
	return len(p), nil
}

func (o *commandOutput) String() string {
	text := strings.TrimSpace(strings.ToValidUTF8(o.buf.String(), ""))
	if o.truncated {
		text += " [output truncated]"
	}
	return text
}

func run(ctx context.Context, command string, args ...string) error {
	if e := ctx.Err(); e != nil {
		return fmt.Errorf("%w: %w", ErrNotStarted, e)
	}
	out := &commandOutput{}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout, cmd.Stderr = out, out
	if e := cmd.Start(); e != nil {
		return fmt.Errorf("%w: %s: %w", ErrNotStarted, command, e)
	}
	if e := cmd.Wait(); e != nil {
		return fmt.Errorf("%s: %w: %s", command, e, out)
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
		if errors.Is(e, ErrNotStarted) {
			r.Outcome = Skipped
			r.Err = e
		} else if e != nil {
			r.Outcome = Failed
			r.Err = e
		}
		results = append(results, r)
	}
	return results
}
