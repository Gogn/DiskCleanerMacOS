package state

import (
	"context"
	"diskcleaner/internal/scan"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// scanObservation records the immediate objects before measurement. Keeping
// it with the worker result prevents old sizes from acquiring a replacement
// object's identity while that result waits for the UI to apply it.
type scanObservation struct {
	root    os.FileInfo
	entries map[string]os.FileInfo
}

func captureScanObservation(ctx context.Context, root string) (*scanObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("scan root is not a directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list scan root: %w", err)
	}
	observed := &scanObservation{root: info, entries: make(map[string]os.FileInfo, len(entries))}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect scan entry: %w", err)
		}
		observed.entries[path] = info
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return observed, nil
}

func sameScanEntry(before, now os.FileInfo) bool {
	return before != nil && now != nil && os.SameFile(before, now) &&
		before.Mode().Type() == now.Mode().Type() && before.Size() == now.Size() && before.ModTime().Equal(now.ModTime())
}

func (observed *scanObservation) validate(root string, snapshot scan.Snapshot) (scan.Snapshot, *scanObservation, error) {
	if observed == nil || observed.root == nil {
		return scan.Snapshot{}, nil, errors.New("scan identity information is unavailable; rescan")
	}
	now, err := os.Stat(root)
	if err != nil {
		return scan.Snapshot{}, nil, fmt.Errorf("inspect completed scan root: %w", err)
	}
	if !now.IsDir() || !os.SameFile(observed.root, now) {
		return scan.Snapshot{}, nil, errors.New("scan root was replaced while scanning; rescan")
	}
	validated := copySnapshot(snapshot)
	validObservation := &scanObservation{root: observed.root, entries: make(map[string]os.FileInfo, len(observed.entries))}
	for _, entry := range snapshot.Listing {
		before := observed.entries[entry.Path]
		now, err := os.Lstat(entry.Path)
		_, measured := snapshot.Entries[entry.Path]
		if err == nil && measured && sameScanEntry(before, now) &&
			before.IsDir() == entry.IsDir && (before.Mode()&os.ModeSymlink != 0) == entry.IsSymlink {
			validObservation.entries[entry.Path] = before
			continue
		}
		validated.Entries[entry.Path] = scan.Measurement{State: scan.Unknown}
		validated.State = scan.Partial
	}
	return validated, validObservation, nil
}
