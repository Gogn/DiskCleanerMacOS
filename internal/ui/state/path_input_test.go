package state

import (
	"context"
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRootInputPastesCopiedPathsWithoutAppendingCurrentFolder(t *testing.T) {
	for _, variant := range []string{"absolute", "quoted", "relative", "literal quotes", "literal spaces"} {
		t.Run(variant, func(t *testing.T) {
			m := fixture(t)
			name := "data space é"
			if variant == "literal quotes" {
				name = "'quoted'"
			} else if variant == "literal spaces" {
				name = " data "
			}
			target := filepath.Join(m.Root, name)
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			input := target
			switch variant {
			case "absolute":
				input += "/"
			case "quoted":
				input = `"` + target + `/"` + "\r\n"
			case "relative":
				input = "./" + name
			case "literal quotes":
				input = name
			}
			m.Analyze = func(context.Context, string, []string, func(scan.Progress)) scan.Snapshot {
				t.Fatal("opening a pasted directory started a scan")
				return scan.Snapshot{}
			}
			m.Offset, m.Message = 99, "Previous notification"
			m.Key("r")
			if m.Input != "" || m.Offset != 0 || m.Message != "" || m.Screen != "root" {
				t.Fatal("path entry retained the old folder or browser viewport")
			}
			oldRoot := m.Root
			m.Paste(input)
			if m.Input != input || m.Root != oldRoot {
				t.Fatal("pasting appended the old folder or opened before Enter")
			}
			if job := m.Key("enter"); job != nil || m.Root != target || m.Screen != "list" || m.Scanned {
				t.Fatal("pasted path did not open without a scan", m.Root, m.Message)
			}
		})
	}
}

func TestInvalidRootInputPreservesCurrentBrowserState(t *testing.T) {
	m := fixture(t)
	file := filepath.Join(m.Root, "ordinary-file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	m.Key("space")
	m.setFilter("cache")
	m.Key("r")
	before := captureExplanationBrowser(m)
	for _, input := range []string{"", filepath.Join(m.Root, "missing"), file, m.Root + "\n" + m.Root, `""`} {
		m.Input = input
		m.Key("enter")
		if m.Screen != "root" || m.Message == "" || m.Input != input || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
			t.Fatal("failed path entry reset browser state or lost its editable input", input, m.Message)
		}
	}
	m.Input, m.Offset = "unfinished path", 9
	m.Key("ctrl+u")
	if m.Input != "" || m.Offset != 0 || m.Message != "" || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("Ctrl+U did not clear only the path field")
	}
}
