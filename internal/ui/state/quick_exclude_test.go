package state

import (
	"diskcleaner/internal/config"
	"diskcleaner/internal/ignore"
	"os"
	"path/filepath"
	"testing"
)

func TestQuickExcludePersistsLiteralFileAndFolderNames(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "folder"}[directory], func(t *testing.T) {
			m := fixture(t)
			name := `report*?[1]\draft`
			path := filepath.Join(m.Root, name)
			var err error
			if directory {
				err = os.Mkdir(path, 0700)
			} else {
				err = os.WriteFile(path, []byte("fixture"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			m.navigate(m.Root)
			scanNow(t, m)
			m.setFilter("report")
			m.Key("space")
			if len(m.Selected) != 1 {
				t.Fatal("fixture not selected")
			}
			m.Key("E")
			saved, err := config.Load(m.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			masks := saved.Masks
			if len(masks) != 1 || !ignore.Any(masks, name) || ignore.Any(masks, "reportZZ1draft") {
				t.Fatalf("name not saved literally: %q", masks)
			}
			if m.Screen != "list" || m.Scanned || len(m.Selected) != 0 {
				t.Fatal("exclusion did not invalidate scan and selection")
			}
			if m.Guard.CheckTarget(path) == nil {
				t.Fatal("excluded target still eligible for cleanup")
			}
			scanNow(t, m)
			m.Key("shift+e")
			if len(m.Masks()) != 0 || m.Scanned {
				t.Fatal("repeat did not remove exclusion and invalidate scan")
			}
			saved, err = config.Load(m.ConfigPath)
			if err != nil || len(saved.Masks) != 0 {
				t.Fatal("removal not persisted", err)
			}
			if err := m.Guard.CheckTarget(path); err != nil {
				t.Fatal("removed exclusion still blocks cleanup", err)
			}
			m.Key("E")
			if !m.hasMask(ignore.Literal(name)) {
				t.Fatal("third press did not restore exclusion")
			}
		})
	}
}

func TestQuickExcludeRespectsFilterAndTextInput(t *testing.T) {
	m := fixture(t)
	m.setFilter("no-matching-entry")
	m.Key("E")
	if len(m.Masks()) != 0 {
		t.Fatal("excluded an invisible item")
	}
	m.Key("/")
	m.Key("E")
	if m.Filter != "no-matching-entryE" {
		t.Fatal("uppercase E not typed in filter")
	}
	m.Key("esc")
	m.Key("e")
	m.Key("a")
	m.Key("E")
	if m.Input != "E" || len(m.Masks()) != 0 {
		t.Fatal("uppercase E not typed in pattern input")
	}
	m.Key("esc")
	m.Key("esc")
	m.toggleMask("c*")
	m.setFilter("cache")
	m.Key("E")
	if len(m.Masks()) != 1 || m.Masks()[0] != "c*" {
		t.Fatal("existing wildcard exclusion changed")
	}
}
