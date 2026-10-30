//go:build !darwin && !linux

package explain

import "os"

func allocatedEntryBytes(info os.FileInfo) *int64 { return nil }
