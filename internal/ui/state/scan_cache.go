package state

import (
	"diskcleaner/internal/scan"
	"os"
	"time"
)

type cachedScan struct {
	snapshot  scan.Snapshot
	root      fileIdentity
	entries   map[string]fileIdentity
	scannedAt time.Time
	restored  bool
	volatile  bool
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

func (m *Model) rememberScan(observed *scanObservation, tokens ...*scanCacheToken) {
	if observed == nil || observed.root == nil {
		return
	}
	cached := cachedScan{snapshot: copySnapshot(m.Snapshot), root: identityOf(observed.root), entries: map[string]fileIdentity{}, scannedAt: m.LastScanAt}
	if len(tokens) > 0 && tokens[0] != nil && !tokens[0].verified && m.scanCachePath() != "" {
		cached.volatile = true
	}
	for path, info := range observed.entries {
		cached.entries[path] = identityOf(info)
	}
	if m.scanCache == nil {
		m.scanCache = map[string]cachedScan{}
	}
	m.scanCache[m.Root] = cached
	m.persistScanCache()
}

func (m *Model) restoreScan() {
	if !m.refreshRootCache(m.Root) {
		m.resetScan()
		return
	}
	cached, ok := m.scanCache[m.Root]
	if !ok {
		return
	}
	root, err := os.Stat(m.Root)
	if err != nil || !cached.root.sameObject(identityOf(root)) {
		delete(m.scanCache, m.Root)
		m.persistScanCache()
		return
	}
	previous := make(map[string]scan.Entry, len(cached.snapshot.Listing))
	for _, entry := range cached.snapshot.Listing {
		previous[entry.Path] = entry
	}
	snapshot := scan.Snapshot{Listing: append([]scan.Entry(nil), m.Entries...), Entries: map[string]scan.Measurement{}, State: cached.snapshot.State}
	infos := map[string]fileIdentity{}
	for _, entry := range m.Entries {
		old, exists := previous[entry.Path]
		before := cached.entries[entry.Path]
		now, err := os.Lstat(entry.Path)
		measurement, measured := cached.snapshot.Entries[entry.Path]
		if exists && measured && err == nil && old.IsDir == entry.IsDir && old.IsSymlink == entry.IsSymlink && before.sameEntry(identityOf(now)) {
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
	m.RestoredScan = cached.restored
	m.persistScanCache()
	m.sortEntries()
}
