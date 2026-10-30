package scan

import (
	"os"
	"path/filepath"
)

func List(dir string) ([]Entry, error) {
	ds, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	es := make([]Entry, 0, len(ds))
	for _, d := range ds {
		p := filepath.Join(dir, d.Name())
		i, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		es = append(es, Entry{p, d.Name(), i.IsDir(), i.Mode()&os.ModeSymlink != 0})
	}
	return es, nil
}
