//go:build darwin || linux

package explain

import (
	"os"
	"syscall"
)

// Reuse the Lstat observation; ownership hints at system or shared items.
func ownedByCurrentUser(info os.FileInfo) *bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return nil
	}
	owned := int(stat.Uid) == os.Getuid()
	return &owned
}
