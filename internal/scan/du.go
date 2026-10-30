package scan

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func parseSummary(out []byte, err error) Measurement {
	prefix, _, ok := bytes.Cut(out, []byte{'\t'})
	n, e := strconv.ParseUint(string(prefix), 10, 64)
	if !ok || e != nil || n > math.MaxUint64/512 {
		if err == nil {
			err = fmt.Errorf("invalid du summary")
		}
		return Measurement{State: Unknown, Err: err}
	}
	state := Complete
	if err != nil {
		state = Partial
	}
	return Measurement{n * 512, state, err}
}
func measure(ctx context.Context, p string, masks []string) Measurement {
	args := []string{"-s", "-x", "-P", "-B", "512"}
	for _, m := range masks {
		args = append(args, "-I", m)
	}
	args = append(args, "--", p)
	cmd := exec.CommandContext(ctx, "/usr/bin/du", args...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "BLOCKSIZE=") && !strings.HasPrefix(v, "LC_ALL=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	// BSD fnmatch must count Unicode characters like ignore.Match, even when
	// the terminal inherits the C locale. LC_ALL overrides LANG and LC_CTYPE.
	cmd.Env = append(cmd.Env, "BLOCKSIZE=512", "LC_ALL=en_US.UTF-8")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, e := cmd.Output()
	if e != nil {
		e = fmt.Errorf("du: %w: %s", e, strings.TrimSpace(stderr.String()))
	}
	return parseSummary(out, e)
}
