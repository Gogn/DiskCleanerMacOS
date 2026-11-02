package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"errors"
	"path/filepath"
	"strings"
)

// pathWithin uses path components so a cleanup of /a never invalidates /ab.
func pathWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == "." || rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// beginCleanupCache keeps other sessions out of the cache until cleanup and
// its final refresh complete, including when that final refresh cannot write.
func (m *Model) beginCleanupCache(targets []cleanup.Target) bool {
	lock, err := m.lockScanCache(true)
	if err != nil {
		m.cacheWriteFailed = true
		return false
	}
	m.cleanupCacheLock = lock
	if !m.invalidateCleanupCache(targets) {
		m.releaseCleanupCacheLock()
		return false
	}
	return true
}

// invalidateCleanupCache persists before the runner starts: interruption or a
// partially successful command must never revive an affected old measurement.
// Unrelated snapshots and sibling measurements keep their original scan time.
func (m *Model) invalidateCleanupCache(targets []cleanup.Target) bool {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		if filepath.IsAbs(target.Path) {
			paths = append(paths, filepath.Clean(target.Path))
		}
	}
	if err := m.invalidateSavedPaths(paths); err != nil {
		m.cacheWriteFailed = true
		return false
	}
	for root, cached := range m.scanCache {
		evict := false
		for _, target := range paths {
			if pathWithin(root, target) {
				evict = true
				break
			}
		}
		if evict {
			delete(m.scanCache, root)
			continue
		}
		for path := range cached.snapshot.Entries {
			for _, target := range paths {
				if pathWithin(path, target) || pathWithin(target, path) {
					cached.snapshot.Entries[path] = scan.Measurement{State: scan.Unknown}
					cached.snapshot.State = scan.Partial
					delete(cached.entries, path)
					break
				}
			}
		}
		m.scanCache[root] = cached
	}
	return true
}

// refreshAfterCleanup only lists immediate entries; it never measures sizes.
// Every attempted path is conservative because a failed runner may have made
// changes before reporting an error or cancellation.
func (m *Model) refreshAfterCleanup(targets []cleanup.Target, results []cleanup.Result) error {
	attempted := append([]cleanup.Target(nil), targets...)
	for _, result := range results {
		attempted = append(attempted, cleanup.Target{Path: result.Path})
	}
	cacheOK := m.invalidateCleanupCache(attempted)
	m.resetScan()
	entries, err := scan.List(m.Root)
	m.Entries = entries
	if err != nil {
		delete(m.scanCache, m.Root)
		m.persistScanCache()
		return err
	}
	m.restoreScan()
	m.sortEntries()
	if !cacheOK {
		return errors.New("saved-size cache refresh failed")
	}
	return nil
}
