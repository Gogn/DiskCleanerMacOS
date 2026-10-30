package state

import (
	"diskcleaner/internal/scan"
	"strings"
	"unicode/utf8"
)

func (m *Model) matchesFilter(entry scan.Entry) bool {
	return (!m.HideHidden || !strings.HasPrefix(entry.Name, ".")) &&
		strings.Contains(strings.ToLower(entry.Name), strings.ToLower(m.Filter))
}

func (m *Model) visibleIndices() []int {
	var indices []int
	for i, entry := range m.Entries {
		if m.matchesFilter(entry) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *Model) normalizeCursor() {
	if _, ok := m.current(); ok {
		return
	}
	m.Cursor = 0
	if indices := m.visibleIndices(); len(indices) > 0 {
		m.Cursor = indices[0]
	}
}

func (m *Model) moveCursor(delta int) {
	indices := m.visibleIndices()
	for i, index := range indices {
		if index == m.Cursor {
			m.Cursor = indices[min(max(0, i+delta), len(indices)-1)]
			return
		}
	}
	m.normalizeCursor()
}

func (m *Model) setFilter(value string) {
	m.Filter = value
	m.normalizeCursor()
}

func (m *Model) hiddenSelections() int {
	visible := 0
	for _, entry := range m.Entries {
		if _, selected := m.Selected[entry.Path]; selected && m.matchesFilter(entry) {
			visible++
		}
	}
	return len(m.Selected) - visible
}

func typedCharacter(key string) string {
	if key == "space" {
		return " "
	}
	if utf8.RuneCountInString(key) == 1 {
		return key
	}
	return ""
}

func backspace(value string) string {
	_, n := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-n]
}

func (m *Model) Paste(value string) {
	if m.Busy {
		return
	}
	switch m.Screen {
	case "filter":
		m.setFilter(m.Filter + value)
	case "root":
		m.Input += value
	case "masks":
		m.MaskInput = true
		m.Input += value
	}
}
