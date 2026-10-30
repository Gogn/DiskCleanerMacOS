package state

import (
	"diskcleaner/internal/config"
	"diskcleaner/internal/scan"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGlobalExclusionsAtStartupNavigationAndAfterEdits(t *testing.T) {
	m := fixture(t)
	parent := m.Root
	child := filepath.Join(parent, "project")
	sibling := filepath.Join(parent, "sibling")
	unrelated := filepath.Join(filepath.Dir(parent), "unrelated")
	roots := []string{parent, child, sibling, unrelated}
	for _, root := range roots {
		if err := os.MkdirAll(filepath.Join(root, "cache"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	masks := []string{"cache"}
	for i := 1; i < 18; i++ {
		masks = append(masks, fmt.Sprintf("excluded-%02d", i))
	}
	m.Settings.Masks = masks
	if err := config.Save(m.ConfigPath, m.Settings); err != nil {
		t.Fatal(err)
	}
	reload := func(root string) {
		t.Helper()
		settings, err := config.Load(m.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		m = New(root, settings, m.Guard, m.ConfigPath)
		if m.Root != root || m.Message != "" {
			t.Fatal("could not start in folder", root, m.Message)
		}
	}
	check := func(excluded bool, count int) {
		t.Helper()
		for _, root := range roots {
			m.navigate(root)
			if len(m.Masks()) != count || !strings.Contains(m.View(), fmt.Sprintf("%d exclusions", count)) || !reflect.DeepEqual(m.Guard.IgnoredMasks, m.Masks()) {
				t.Fatal("navigation changed global rules or guard", root, m.View())
			}
			cache := filepath.Join(root, "cache")
			if blocked := m.Guard.CheckTarget(cache) != nil; blocked != excluded {
				t.Fatal("guard disagrees with global rules", cache, blocked)
			}
			scanNow(t, m)
			measurement := m.Snapshot.Entries[cache]
			if excluded {
				if measurement.State != scan.Skipped || measurement.Err == nil || measurement.Err.Error() != "excluded name" {
					t.Fatal("scan ignored global exclusion", cache, measurement)
				}
			} else if measurement.State != scan.Complete {
				t.Fatal("scan retained removed exclusion", cache, measurement)
			}
		}
	}
	reload(unrelated)
	if !reflect.DeepEqual(m.Masks(), masks) || !strings.Contains(m.View(), "18 exclusions") {
		t.Fatal("startup in unrelated folder lost global exclusions", m.View())
	}
	check(true, 18)
	m.setFilter("cache")
	m.Key("E")
	if m.hasMask("cache") || !strings.Contains(m.Message, "globally") {
		t.Fatal("global exclusion removal failed", m.Message)
	}
	reload(child)
	check(false, 17)
	m.setFilter("cache")
	m.Key("E")
	if !m.hasMask("cache") || !strings.Contains(m.Message, "globally") {
		t.Fatal("global exclusion addition failed", m.Message)
	}
	reload(unrelated)
	check(true, 18)
}
