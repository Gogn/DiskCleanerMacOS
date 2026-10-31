//go:build !darwin && !linux

package explain

import "os"

func ownedByCurrentUser(info os.FileInfo) *bool { return nil }
