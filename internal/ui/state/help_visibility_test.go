package state

import (
	"diskcleaner/internal/ignore"
	"diskcleaner/internal/scan"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHiddenFilesTogglePreservesScanAndSelectedTargets(t *testing.T) {
	m := fixture(t)
	secret := filepath.Join(m.Root, ".secret")
	if err := os.WriteFile(secret, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	m.navigate(m.Root)
	if m.HideHidden || len(m.visibleIndices()) != 3 {
		t.Fatal("dot-files should be visible by default")
	}
	scanNow(t, m)
	if current, _ := m.current(); current.Path != secret {
		t.Fatal("fixture cursor mismatch")
	}
	m.Key("space")
	target := m.Selected[secret]
	if job := m.Key("."); job != nil {
		t.Fatal("visibility toggle started work")
	}
	if !m.HideHidden || !m.Scanned || len(m.Entries) != 3 || len(m.visibleIndices()) != 2 || m.hiddenSelections() != 1 || m.Selected[secret] != target || m.Snapshot.Entries[secret].State != scan.Complete {
		t.Fatal("visibility changed scan or selection")
	}
	if current, ok := m.current(); !ok || current.Path == secret {
		t.Fatal("cursor remained on an invisible item")
	}
	m.Key("d")
	if len(m.Confirmed) != 1 || m.Confirmed[0] != target || !strings.Contains(m.View(), "Hidden selected items: 1") {
		t.Fatal("hidden target not explicitly confirmed")
	}
	m.Key("esc")
	m.Key("/")
	m.Key(".")
	if m.Filter != "." || !m.HideHidden {
		t.Fatal("dot did not remain literal in a text field")
	}
	m.Key("esc")
	m.Key(".")
	if m.HideHidden || m.hiddenSelections() != 0 || len(m.Selected) != 1 {
		t.Fatal("showing dot-files changed selection")
	}
	m.Key(".")
	m.navigate(filepath.Join(m.Root, "cache"))
	if !m.HideHidden {
		t.Fatal("folder navigation forgot visibility preference")
	}
	if err := os.WriteFile(filepath.Join(m.Root, ".only"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.navigate(m.Root)
	if _, ok := m.current(); ok {
		t.Fatal("all-hidden folder exposes an item")
	}
	if !strings.Contains(m.View(), "No visible items") {
		t.Fatal(m.View())
	}
	m.Key(".")
	if _, ok := m.current(); !ok {
		t.Fatal("could not reveal hidden-only folder")
	}
}

func TestHelpTopicsScrollAndRestoreContext(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 32, 12
	m.Key("e")
	m.Key("down")
	m.Key("?")
	if m.helpTopic != 3 {
		t.Fatal("exclusion help did not open relevant topic")
	}
	for i := range helpTopics {
		m.Key(string(rune('1' + i)))
		if m.helpTopic != i || m.Offset != 0 {
			t.Fatal("topic jump did not reset scrolling")
		}
		for _, key := range []string{"home", "pgdown", "end"} {
			m.Key(key)
			view := m.View()
			if len(strings.Split(view, "\n")) != m.Height {
				t.Fatal("help frame moved", view)
			}
			for _, line := range strings.Split(view, "\n") {
				if cells(plainView(line)) > m.Width {
					t.Fatal("help exceeds width", line)
				}
			}
		}
		if m.Offset == 0 {
			t.Fatal("topic cannot scroll")
		}
	}
	m.Key("right")
	if m.helpTopic != 0 {
		t.Fatal("next topic did not wrap")
	}
	m.Key("left")
	if m.helpTopic != len(helpTopics)-1 {
		t.Fatal("previous topic did not wrap")
	}
	m.Key("esc")
	if m.Screen != "masks" || m.maskCursor != 1 || len(m.Masks()) != 0 {
		t.Fatal("help changed editor context")
	}
}

func TestSuggestedPatternsRemainOptionalAndAccessible(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 32, 12
	m.Key("e")
	seen := map[string]bool{}
	for i, suggestion := range suggestedMasks {
		if err := ignore.Validate(suggestion.name); err != nil {
			t.Fatal(err)
		}
		if seen[suggestion.name] || suggestion.description == "" {
			t.Fatal("duplicate or unexplained suggestion")
		}
		seen[suggestion.name] = true
		m.maskCursor = i
		view := m.View()
		if !strings.Contains(plainView(view), suggestion.name) || len(strings.Split(view, "\n")) > m.Height {
			t.Fatal("suggestion not accessible", view)
		}
		compact := strings.Join(strings.Fields(plainView(view)), " ")
		if !strings.Contains(compact, suggestion.description) {
			t.Fatal("description is clipped", view)
		}
	}
	if len(m.Masks()) != 0 {
		t.Fatal("browsing enabled exclusions")
	}
	m.Key("enter")
	if !m.hasMask("build") {
		t.Fatal("new suggestion could not be enabled")
	}
}

func TestSafeNamesHaveNoWrapperAndEscapeTerminalControls(t *testing.T) {
	for _, input := range []string{"", "My folder", "/tmp/café/文件", "file.txt"} {
		if Safe(input) != input {
			t.Fatalf("unexpected decoration: %q", Safe(input))
		}
	}
	got := Safe("line\n\t\x1b[31m\\\"")
	if got != `line\n\t\x1b[31m\\\"` {
		t.Fatalf("unsafe or incorrect escaping: %q", got)
	}
	m := fixture(t)
	m.Width = 80
	m.Message = "Example diagnostic"
	m.setFilter("cache")
	lines := strings.Split(m.View(), "\n")
	for i, line := range lines {
		if strings.Contains(plainView(line), "Name") && strings.Contains(plainView(line), "Status") {
			if i == 0 || !strings.HasPrefix(lines[i-1], "\x1b[1;36m") || !strings.HasSuffix(plainView(lines[i-1]), "/root") {
				t.Fatal("colored current path is not immediately above the columns")
			}
			return
		}
	}
	t.Fatal("missing file columns")
}
