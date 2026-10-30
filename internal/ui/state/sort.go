package state

import (
	"diskcleaner/internal/ignore"
	"diskcleaner/internal/scan"
	"sort"
	"strings"
)

func (m *Model) sortLabel() string {
	if !m.SortBySize {
		return "Alphabetical"
	}
	if !m.Scanned {
		return "Size (scan needed)"
	}
	return "Size (largest first)"
}

// Keep the cursor attached to its path, just like the selection. Entries with
// no measured size sort after measured entries, never as zero-byte files.
func (m *Model) sortEntries() {
	defer m.normalizeCursor()
	current, _ := m.current()
	sort.Slice(m.Entries, func(i, j int) bool {
		a, b := m.Entries[i], m.Entries[j]
		if m.SortBySize && m.Scanned {
			as, ak := m.sortSize(a)
			bs, bk := m.sortSize(b)
			if ak != bk {
				return ak
			}
			if ak && as != bs {
				return as > bs
			}
		}
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if an != bn {
			return an < bn
		}
		return a.Name < b.Name
	})
	m.Cursor = min(max(0, m.Cursor), max(0, len(m.Entries)-1))
	for i, entry := range m.Entries {
		if entry.Path == current.Path {
			m.Cursor = i
			break
		}
	}
}

func (m *Model) sortSize(entry scan.Entry) (uint64, bool) {
	measurement, ok := m.Snapshot.Entries[entry.Path]
	known := ok && !ignore.Any(m.Masks(), entry.Name) &&
		(measurement.State == scan.Complete || measurement.State == scan.Partial)
	return measurement.Bytes, known
}
