package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultsAndRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings", "config.json")
	s, e := Load(p)
	if e != nil || s.Mode != Trash || len(s.Masks) != 0 {
		t.Fatal(s, e)
	}
	s.Mode = Permanent
	s.Masks["/one"] = []string{"cache"}
	s.Masks["/two"] = []string{"*.tmp"}
	if e = Save(p, s); e != nil {
		t.Fatal(e)
	}
	got, e := Load(p)
	if e != nil || !reflect.DeepEqual(got, s) {
		t.Fatal(got, e)
	}
	i, _ := os.Stat(p)
	if i.Mode().Perm() != 0600 {
		t.Fatal(i.Mode())
	}
	for _, v := range []string{`{`, `{"mode":"bad"}`, `{"mode":"trash","masks":{"/one":["a/b"]}}`} {
		os.WriteFile(p, []byte(v), 0600)
		if _, e = Load(p); e == nil {
			t.Fatal("accepted", v)
		}
	}
}
