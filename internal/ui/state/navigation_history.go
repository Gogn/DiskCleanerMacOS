package state

import "path/filepath"

func (m *Model) rememberLocation() {
	if entry, ok := m.current(); ok && m.Root != "" {
		if m.navigationHistory == nil {
			m.navigationHistory = map[string]string{}
		}
		m.navigationHistory[m.Root] = entry.Path
	}
}

func (m *Model) restoreLocation(previousRoot string) {
	m.Cursor = 0
	if previousRoot == m.Root {
		m.normalizeCursor()
		return
	}
	path := m.navigationHistory[m.Root]
	// Going up should highlight the folder just left, including when the app
	// started in that folder and its parent has never been opened.
	if previousRoot != "" && previousRoot != m.Root && filepath.Dir(previousRoot) == m.Root {
		path = previousRoot
	}
	for i, entry := range m.Entries {
		if entry.Path == path && m.matchesFilter(entry) {
			m.Cursor = i
			break
		}
	}
	m.normalizeCursor()
}
