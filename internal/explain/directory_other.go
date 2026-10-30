//go:build !darwin && !linux

package explain

import (
	"errors"
	"os"
)

func openDirectory(path string) (*os.File, error) {
	return nil, errors.New("safe folder inspection is unavailable on this platform")
}
