package state

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenericScrollIndicatorUsesOverflowRatherThanExactFit(t *testing.T) {
	m := &Model{Screen: "preview", Root: "/", Width: 100, Height: 10, NoColor: true}
	m.Preview.Paths = []string{"/one", "/two", "/three"}
	view, limit := m.render()
	if limit != 0 || strings.Contains(view, "Lines ") || !strings.Contains(view, "/three") || len(strings.Split(view, "\n")) != m.Height {
		t.Fatal("exactly fitting content lost a row or gained a scroll indicator", view, limit)
	}
	m.Preview.Paths = append(m.Preview.Paths, "/four")
	view, limit = m.render()
	if limit != 2 || !strings.Contains(view, "Lines 1–7 / 9") || strings.Contains(view, "/four") {
		t.Fatal("overflow did not reserve an indicator row", view, limit)
	}
	m.Key("down")
	if m.Offset != 1 || !strings.Contains(m.View(), "Lines 2–8 / 9") {
		t.Fatal("scroll did not update the displayed range", m.View())
	}
	m.Offset = 999
	view, _ = m.render()
	if !strings.Contains(view, "Lines 3–9 / 9") || !strings.Contains(view, "/four") || !strings.HasSuffix(view, "↑/↓ scroll · Esc back") {
		t.Fatal("end clamp lost the final content or footer", view)
	}
	m.Height = 20
	view, limit = m.render()
	if limit != 0 || strings.Contains(view, "Lines ") || !strings.Contains(view, "/one") || !strings.Contains(view, "/four") {
		t.Fatal("resizing to fit retained a stale offset or indicator", view, limit)
	}
	m.Height, m.Offset = 10, -10
	if view = m.View(); !strings.Contains(view, "Lines 1–7 / 9") {
		t.Fatal("negative offset was not clamped", view)
	}
}

func TestGenericIndicatorsOnlyAppearOnScrollableScreens(t *testing.T) {
	for _, screen := range []string{"details", "preview", "confirm", "results", "root", "scanning", "previewing", "cleaning", "scan-confirm", "settings"} {
		t.Run(screen, func(t *testing.T) {
			m := &Model{Screen: screen, Root: "/", Width: 100, Height: 30, NoColor: true}
			view, limit := m.render()
			if limit != 0 || strings.Contains(view, "Lines ") || strings.Contains(view, "More:") {
				t.Fatal("short screen has a scroll indicator", view, limit)
			}
			m.Message = strings.Repeat("long notification ", 100)
			m.Height = 8
			m.Offset = 999
			view, limit = m.render()
			scrollable := screen == "details" || screen == "preview" || screen == "confirm" || screen == "results" || screen == "root"
			if scrollable {
				if limit == 0 || !strings.Contains(view, "Lines ") {
					t.Fatal("scrollable overflow has no indicator", view, limit)
				}
			} else {
				if limit != 0 || strings.Contains(view, "Lines ") || strings.Contains(view, "More:") || !strings.Contains(view, "DISKCLEANER\n/\n") {
					t.Fatal("non-scrollable screen exposes scrolling or uses a stale offset", view, limit)
				}
			}
			if len(strings.Split(view, "\n")) > m.Height {
				t.Fatal("render exceeds terminal height", view)
			}
		})
	}
}

func TestGenericScrollingAtNarrowWidthKeepsFooterAndLastLine(t *testing.T) {
	m := &Model{Screen: "preview", Root: "/", Width: 12, Height: 12, NoColor: true}
	for i := 0; i < 20; i++ {
		m.Preview.Paths = append(m.Preview.Paths, fmt.Sprintf("/item-%02d", i))
	}
	view, limit := m.render()
	if limit == 0 || !strings.Contains(view, "More: ↑/↓") || len(strings.Split(view, "\n")) != m.Height {
		t.Fatal("narrow overflow has no usable indicator", view, limit)
	}
	m.Key("pgdown")
	if m.Offset == 0 {
		t.Fatal("narrow screen did not scroll")
	}
	m.Offset = limit + 100
	view, _ = m.render()
	if !strings.Contains(view, "/item-19\nMore: ↑/↓\n") || !strings.HasSuffix(view, "back") || len(strings.Split(view, "\n")) != m.Height {
		t.Fatal("narrow end clamp lost last line or wrapped footer", view)
	}
}
