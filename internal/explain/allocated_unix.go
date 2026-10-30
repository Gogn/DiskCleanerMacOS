//go:build darwin || linux

package explain

import (
	"os"
	"syscall"
)

// Reuse the Lstat observation; no file reads or recursive measurement are needed.
func allocatedEntryBytes(info os.FileInfo) *int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Blocks < 0 || stat.Blocks > (1<<63-1)/512 {
		return nil
	}
	bytes := stat.Blocks * 512
	return &bytes
}
