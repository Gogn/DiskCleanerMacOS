package state

import (
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

const maxScanCacheBytes = 16 << 20

// File identity is persisted as values, never as an os.FileInfo or a path
// whose identity could silently change between measurement and serialization.
type fileIdentity struct {
	Device   uint64    `json:"device"`
	Inode    uint64    `json:"inode"`
	Type     uint32    `json:"type"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Valid    bool      `json:"valid"`
}

func identityOf(info os.FileInfo) fileIdentity {
	if info == nil {
		return fileIdentity{}
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}
	}
	return fileIdentity{uint64(stat.Dev), uint64(stat.Ino), uint32(info.Mode().Type()), info.Size(), info.ModTime(), true}
}
func (a fileIdentity) sameObject(b fileIdentity) bool {
	return a.Valid && b.Valid && a.Device == b.Device && a.Inode == b.Inode && a.Type == b.Type
}
func (a fileIdentity) sameEntry(b fileIdentity) bool {
	return a.sameObject(b) && a.Size == b.Size && a.Modified.Equal(b.Modified)
}

type diskMeasurement struct {
	Bytes    uint64     `json:"bytes"`
	State    scan.State `json:"state"`
	HadError bool       `json:"had_error,omitempty"`
}
type diskScan struct {
	Root         fileIdentity               `json:"root"`
	Listing      []scan.Entry               `json:"listing"`
	Identities   map[string]fileIdentity    `json:"identities"`
	Measurements map[string]diskMeasurement `json:"measurements"`
	State        scan.State                 `json:"state"`
	ScannedAt    time.Time                  `json:"scanned_at"`
}

func normalizedMasks(masks []string) []string {
	result := append([]string{}, masks...)
	slices.Sort(result)
	return slices.Compact(result)
}

func decodeDiskScan(root string, saved diskScan) (cachedScan, bool) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !saved.Root.Valid || saved.Root.Type != uint32(os.ModeDir) || saved.ScannedAt.IsZero() || saved.State > scan.Skipped {
		return cachedScan{}, false
	}
	cached := cachedScan{root: saved.Root, entries: map[string]fileIdentity{}, scannedAt: saved.ScannedAt, restored: true, snapshot: scan.Snapshot{State: saved.State, Entries: map[string]scan.Measurement{}}}
	seen := map[string]bool{}
	for _, entry := range saved.Listing {
		if entry.Path != filepath.Join(root, entry.Name) || entry.Name == "." || entry.Name == ".." || filepath.Base(entry.Name) != entry.Name || entry.Name == "" || seen[entry.Path] {
			return cachedScan{}, false
		}
		seen[entry.Path] = true
		value, ok := saved.Measurements[entry.Path]
		if !ok || value.State > scan.Skipped {
			return cachedScan{}, false
		}
		measurement := scan.Measurement{Bytes: value.Bytes, State: value.State}
		if value.HadError {
			measurement.Err = errors.New("Saved scan reported an error; rescan for details")
		}
		cached.snapshot.Entries[entry.Path] = measurement
		cached.entries[entry.Path] = saved.Identities[entry.Path]
	}
	cached.snapshot.Listing = saved.Listing
	return cached, true
}
func encodeDiskScan(cached cachedScan) diskScan {
	saved := diskScan{Root: cached.root, Listing: cached.snapshot.Listing, Identities: cached.entries, Measurements: map[string]diskMeasurement{}, State: cached.snapshot.State, ScannedAt: cached.scannedAt}
	for path, measurement := range cached.snapshot.Entries {
		saved.Measurements[path] = diskMeasurement{measurement.Bytes, measurement.State, measurement.Err != nil}
	}
	return saved
}
