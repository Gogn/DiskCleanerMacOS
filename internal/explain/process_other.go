//go:build !darwin && !linux

package explain

import (
	"errors"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd) error {
	return errors.New("Codex explanation is unavailable on this platform")
}

func cleanupProcess(cmd *exec.Cmd) {}
