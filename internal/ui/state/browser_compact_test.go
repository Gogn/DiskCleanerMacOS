package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBrowserCompactFrameAndStableWholeCommandFooters(t *testing.T) {
	for _, width := range []int{32, 40, 64, 80, 120} {
		for _, height := range []int{12, 24} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				m := fixture(t)
				m.Width, m.Height = width, height
				m.Entries = nil
				for i := 0; i < 40; i++ {
					m.Entries = append(m.Entries, scan.Entry{Path: fmt.Sprintf("/item-%02d", i), Name: fmt.Sprintf("item-%02d", i)})
				}
				m.Cursor = 20
				nav := m.browserFooter()[0]
				for _, phase := range []string{"unscanned", "scanned", "selected", "filter"} {
					m.Scanned = phase != "unscanned"
					if m.Scanned {
						m.LastScanAt = time.Date(2026, 10, 30, 14, 32, 0, 0, time.Local)
					}
					if phase == "selected" || phase == "filter" {
						m.Selected["/item-00"] = cleanup.Target{Path: "/item-00"}
						m.setFilter("item-2")
						m.Message = strings.Repeat("Important diagnostic ", 20)
						m.SortBySize = true
					}
					if phase == "filter" {
						m.Screen = "filter"
					}
					view := m.View()
					lines := strings.Split(plainView(view), "\n")
					if len(lines) != height || strings.Contains(view, "Resize") {
						t.Fatalf("%s frame does not fit:\n%s", phase, view)
					}
					for _, line := range lines {
						if cells(line) > width {
							t.Fatalf("%s line exceeds width: %q", phase, line)
						}
					}
					footer := lines[len(lines)-2:]
					if strings.Contains(strings.Join(footer, "\n"), "...") {
						t.Fatal("footer clipped a command", footer)
					}
					if phase != "filter" {
						if footer[0] != nav || !strings.Contains(footer[0], "? help") {
							t.Fatal("navigation hints changed with scan/selection", phase, footer)
						}
					} else if !strings.Contains(footer[0], "Enter apply | Esc clear") || strings.Contains(strings.Join(footer, "\n"), "? help") {
						t.Fatal("filter input hints advertise the wrong keys", footer)
					}
					if phase == "selected" && (!strings.Contains(footer[1], "p preview") || !strings.Contains(footer[1], "d clean")) {
						t.Fatal("selection actions disappeared", footer)
					}
					layout := m.browserLayout()
					header := strings.Join(layout.header, "\n")
					if !strings.Contains(plainView(header), "No exclusions") {
						t.Fatal("compact header lost global exclusion warning", header)
					}
					if phase == "selected" || phase == "filter" {
						for _, want := range []string{"1 selected", "1 hidden", "Selected", "unknown", "Filter:", "v details"} {
							if !strings.Contains(plainView(header), want) {
								t.Fatal("crowded header lost essential selection/filter/diagnostic", want, header)
							}
						}
					}
					pathRow := len(layout.header) - 2
					if !strings.Contains(lines[pathRow+1], "Name") || lines[pathRow] != tailFit(Safe(m.Root), width) {
						t.Fatal("highlighted path is not immediately above columns", lines[:len(layout.header)])
					}
					span := 1
					if width < 64 {
						span = 2
					}
					if height-len(layout.header)-3 < span {
						t.Fatal("header displaced a complete entry", phase, view)
					}
					var start, end, total int
					if _, err := fmt.Sscanf(lines[len(lines)-3], "Rows %d-%d / %d", &start, &end, &total); err != nil {
						t.Fatal("missing row range", err, view)
					}
					if (end-start+1)%span != 0 || (end-start+1)/span != min(m.browserPageSize(), len(m.visibleIndices())) {
						t.Fatal("rendered complete entries differ from page size", phase, view, m.browserPageSize())
					}
					if width < 64 {
						for i, line := range lines[:len(lines)-3] {
							if strings.HasPrefix(line, "› ") && (i+1 >= len(lines)-3 || !strings.HasPrefix(lines[i+1], "      ")) {
								t.Fatal("highlighted narrow entry lost its metadata row", view)
							}
						}
					}
					m.NoColor = true
					if m.View() != plainView(view) {
						t.Fatal("color changes compact layout")
					}
					m.NoColor = false
				}
			})
		}
	}
}

func TestOrdinaryWideBrowserHeaderUsesFewerRows(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 80, 24
	layout := m.browserLayout()
	if len(layout.header) >= 6 || strings.Contains(strings.Join(layout.header, "\n"), "DISKCLEANER") || strings.Contains(strings.Join(layout.header, "\n"), "0 selected") {
		t.Fatal("ordinary browser header retains redundant rows", layout.header)
	}
}
