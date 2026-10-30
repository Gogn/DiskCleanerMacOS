package scan

import (
	"os"
	"path/filepath"
)

func List(dir string) ([]Entry, error) {
	return listWithLstat(dir, os.Lstat)
}

func listWithLstat(dir string, lstat func(string) (os.FileInfo, error)) ([]Entry, error) {
	ds, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	es := make([]Entry, 0, len(ds))
	for _, d := range ds {
		p := filepath.Join(dir, d.Name())
		i, e := lstat(p)
		if os.IsNotExist(e) {
			// A temporary entry can disappear after ReadDir; retain the rest
			// of the fresh listing instead of rejecting the entire directory.
			continue
		}
		if e != nil {
			return nil, e
		}
		es = append(es, Entry{p, d.Name(), i.IsDir(), i.Mode()&os.ModeSymlink != 0})
	}
	return es, nil
}
