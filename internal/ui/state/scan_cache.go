package state

import (
	"diskcleaner/internal/scan"
	"os"
	"time"
)

type cachedScan struct {
	snapshot  scan.Snapshot
	root      os.FileInfo
	entries   map[string]os.FileInfo
	scannedAt time.Time
}

func copySnapshot(s scan.Snapshot) scan.Snapshot {
	copy := s
	copy.Listing = append([]scan.Entry(nil), s.Listing...)
	copy.Entries = make(map[string]scan.Measurement, len(s.Entries))
	for path, measurement := range s.Entries {
		copy.Entries[path] = measurement
	}
	return copy
}

func (m *Model) rememberScan(observed *scanObservation) {
	if observed == nil || observed.root == nil {
		return
	}
	cached := cachedScan{snapshot: copySnapshot(m.Snapshot), root: observed.root, entries: map[string]os.FileInfo{}, scannedAt: m.LastScanAt}
	for path, info := range observed.entries {
		cached.entries[path] = info
	}
	if m.scanCache == nil {
		m.scanCache = map[string]cachedScan{}
	}
	m.scanCache[m.Root] = cached
}

func (m *Model) restoreScan() {
	cached, ok := m.scanCache[m.Root]
	if !ok {
		return
	}
	root, err := os.Stat(m.Root)
	if err != nil || !os.SameFile(root, cached.root) {
		delete(m.scanCache, m.Root)
		return
	}
	previous := make(map[string]scan.Entry, len(cached.snapshot.Listing))
	for _, entry := range cached.snapshot.Listing {
		previous[entry.Path] = entry
	}
	snapshot := scan.Snapshot{Listing: append([]scan.Entry(nil), m.Entries...), Entries: map[string]scan.Measurement{}, State: cached.snapshot.State}
	infos := map[string]os.FileInfo{}
	for _, entry := range m.Entries {
		old, exists := previous[entry.Path]
		before := cached.entries[entry.Path]
		now, err := os.Lstat(entry.Path)
		measurement, measured := cached.snapshot.Entries[entry.Path]
		if exists && measured && err == nil && old.IsDir == entry.IsDir && old.IsSymlink == entry.IsSymlink && sameScanEntry(before, now) {
			snapshot.Entries[entry.Path] = measurement
			infos[entry.Path] = before
		} else {
			snapshot.Entries[entry.Path] = scan.Measurement{State: scan.Unknown}
			snapshot.State = scan.Partial
		}
	}
	// Keep refreshed listings and forget invalid measurements permanently, so a
	// later visit cannot revive a removed or replaced entry's old size.
	cached.snapshot, cached.entries = copySnapshot(snapshot), infos
	m.scanCache[m.Root] = cached
	m.Snapshot, m.Scanned = snapshot, true
	m.LastScanAt = cached.scannedAt
	m.sortEntries()
}
