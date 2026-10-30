// Package explain provides an optional explanation of the item under the cursor.
package explain

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

type Request struct {
	Path     string
	Language string // "en" (also the empty default) or "ru".
}

var ErrCLINotFound = errors.New("Codex CLI was not found; install Codex and sign in before retrying")

// Run starts the installed Codex CLI after the caller has obtained consent.
// The initial payload contains metadata only, but Codex may read file contents.
// Read-only command permissions are not a selected-path read boundary.
func Run(ctx context.Context, req Request) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	instructions, err := promptInstructions(req.Language)
	if err != nil {
		return "", err
	}
	binary, err := findCLI(exec.LookPath, []string{
		"/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex",
		"/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex",
	})
	if err != nil {
		return "", err
	}
	payload, err := collectMetadata(ctx, req)
	if err != nil {
		return "", err
	}
	prompt := append([]byte(instructions), payload...)
	return runCLI(ctx, binary, prompt, 2*time.Minute, maxOutputBytes)
}

func findCLI(lookup func(string) (string, error), candidates []string) (string, error) {
	if path, err := lookup("codex"); err == nil {
		return path, nil
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return path, nil
		}
	}
	return "", ErrCLINotFound
}
