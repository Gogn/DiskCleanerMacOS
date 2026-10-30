package policy

import (
	"os"
	"path/filepath"
	"syscall"
)

func mountRoot(p string) (string, error) {
	i, e := os.Lstat(p)
	if e != nil {
		return "", e
	}
	if i.Mode()&os.ModeSymlink != 0 {
		p = filepath.Dir(p)
	}
	var s syscall.Statfs_t
	if e := syscall.Statfs(p, &s); e != nil {
		return "", e
	}
	b := make([]byte, 0, len(s.Mntonname))
	for _, c := range s.Mntonname {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b), nil
}

// mountNoWait is MNT_NOWAIT from Darwin sys/mount.h.
const mountNoWait = 2

func mountedRoots() ([]string, error) {
	n, e := syscall.Getfsstat(nil, mountNoWait)
	if e != nil {
		return nil, e
	}
	// Retry if a volume is mounted while querying the table.
	for {
		buf := make([]syscall.Statfs_t, n+8)
		count, e := syscall.Getfsstat(buf, mountNoWait)
		if e != nil {
			return nil, e
		}
		if count >= len(buf) {
			n = count
			continue
		}
		roots := make([]string, 0, count)
		for _, s := range buf[:count] {
			b := []byte{}
			for _, c := range s.Mntonname {
				if c == 0 {
					break
				}
				b = append(b, byte(c))
			}
			roots = append(roots, string(b))
		}
		return roots, nil
	}
}
