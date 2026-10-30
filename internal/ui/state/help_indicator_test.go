package state

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func helpBodyLineCount(topic int, width int) int {
	count := 0
	for _, line := range helpTopics[topic].lines {
		heading := line == strings.ToUpper(line) && !strings.Contains(line, "  ")
		if heading && count > 0 {
			count++
		}
		count += len(strings.Split(ansi.Wrap(line, width, ""), "\n"))
	}
	return count
}

func TestHelpHidesLineIndicatorWhenAllContentFits(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height, m.helpTopic, m.Screen = 200, 40, 3, "help"
	view, limit := m.renderHelp()
	if limit != 0 || strings.Contains(view, "Lines ") {
		t.Fatalf("non-overflowing help should not show a line indicator (limit %d):\n%s", limit, view)
	}
	if got := len(strings.Split(view, "\n")); got != m.Height {
		t.Fatalf("help frame has %d lines, want %d", got, m.Height)
	}
}

func TestHelpExactFitDoesNotCreateArtificialOverflow(t *testing.T) {
	m := fixture(t)
	m.Width, m.helpTopic, m.Screen = 200, 3, "help"
	bodyLines := helpBodyLineCount(m.helpTopic, m.Width)
	m.Height = 2 + bodyLines + 2 // two header and two footer lines
	if m.Height < 12 {
		t.Fatalf("test topic unexpectedly too short: height %d", m.Height)
	}
	view, limit := m.renderHelp()
	if limit != 0 || strings.Contains(view, "Lines ") {
		t.Fatalf("exact-fit help should not overflow (limit %d):\n%s", limit, view)
	}
	if got := len(strings.Split(view, "\n")); got != m.Height {
		t.Fatalf("help frame has %d lines, want %d", got, m.Height)
	}
}

func TestHelpOverflowKeepsIndicatorAndScrollsToEnd(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height, m.helpTopic, m.Screen = 200, 12, 3, "help"
	initial, limit := m.renderHelp()
	if limit <= 0 || !strings.Contains(initial, "Lines ") {
		t.Fatalf("overflowing help should show a scroll indicator (limit %d):\n%s", limit, initial)
	}
	if got := len(strings.Split(initial, "\n")); got != m.Height {
		t.Fatalf("help frame has %d lines, want %d", got, m.Height)
	}
	m.Key("end")
	view := m.View()
	if m.Offset != limit || !strings.Contains(view, "Lines ") {
		t.Fatalf("End did not reach the final page: offset %d, limit %d\n%s", m.Offset, limit, view)
	}
	indicator := strings.Split(strings.Split(view, "Lines ")[1], "\n")[0]
	if !strings.HasSuffix(indicator, fmt.Sprintf("/ %d | PgUp/PgDn scroll", helpBodyLineCount(m.helpTopic, m.Width))) {
		t.Fatalf("End indicator does not show the last body line: %q", indicator)
	}
	if got := len(strings.Split(view, "\n")); got != m.Height {
		t.Fatalf("help frame has %d lines, want %d", got, m.Height)
	}
}
