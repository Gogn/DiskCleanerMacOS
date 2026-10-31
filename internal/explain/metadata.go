package explain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	maxChildren        = 32
	maxCountedChildren = 10000
	maxSkipReasonBytes = 200
	maxMetadataBytes   = 32 * 1024
)

// Metadata is an observation of the directory entry, not a recursive scan.
// Both entry size fields for a directory describe its entry, not its contents;
// a recursive size is present only as the app's earlier scan result.
type metadata struct {
	Path                   string        `json:"path"`
	Name                   string        `json:"name"`
	Kind                   string        `json:"kind"`
	SymlinkTarget          string        `json:"symlink_target,omitempty"`
	OwnedByCurrentUser     *bool         `json:"owned_by_current_user"`
	LogicalSizeBytes       int64         `json:"entry_logical_size_bytes"`
	AllocatedSizeBytes     *int64        `json:"entry_allocated_size_bytes"`
	ModifiedAt             string        `json:"modified_at"`
	LastScan               *scanMetadata `json:"last_scan"`
	DeleteMode             string        `json:"delete_mode,omitempty"`
	ChildCount             *int          `json:"immediate_child_count,omitempty"`
	ChildCountIsLowerBound bool          `json:"immediate_child_count_is_lower_bound,omitempty"`
	Children               []child       `json:"immediate_children,omitempty"`
	ChildrenTruncated      bool          `json:"child_list_truncated,omitempty"`
}

type child struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type scanMetadata struct {
	AllocatedSizeBytes *int64 `json:"allocated_size_bytes"`
	State              string `json:"state"`
	SkipReason         string `json:"skip_reason,omitempty"`
	ScannedAt          string `json:"scanned_at"`
}

func scanResultMetadata(result *ScanResult) (*scanMetadata, error) {
	if result == nil {
		return nil, nil
	}
	switch result.State {
	case "complete", "partial", "unknown", "skipped":
	default:
		return nil, errors.New("unsupported scan state for the explanation")
	}
	if result.AllocatedBytes != nil && *result.AllocatedBytes < 0 || len(result.SkipReason) > maxSkipReasonBytes || result.ScannedAt.IsZero() {
		return nil, errors.New("invalid scan result for the explanation")
	}
	return &scanMetadata{AllocatedSizeBytes: result.AllocatedBytes, State: result.State, SkipReason: result.SkipReason, ScannedAt: result.ScannedAt.Format(time.RFC3339)}, nil
}

func collectMetadata(ctx context.Context, req Request) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, errors.New("no item selected for explanation")
	}
	if req.DeleteMode != "" && req.DeleteMode != "trash" && req.DeleteMode != "permanent" {
		return nil, errors.New("unsupported deletion mode for the explanation")
	}
	lastScan, err := scanResultMetadata(req.LastScan)
	if err != nil {
		return nil, err
	}
	path, err := filepath.Abs(req.Path)
	if err != nil {
		return nil, errors.New("cannot resolve the selected item's path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect the selected item: %w", err)
	}
	m := metadata{Path: path, Name: filepath.Base(path), Kind: itemKind(info.Mode()), OwnedByCurrentUser: ownedByCurrentUser(info), LogicalSizeBytes: info.Size(), AllocatedSizeBytes: allocatedEntryBytes(info), ModifiedAt: info.ModTime().Format(time.RFC3339Nano), LastScan: lastScan, DeleteMode: req.DeleteMode}
	if info.Mode()&os.ModeSymlink != 0 {
		// Readlink reports the stored target text without following it.
		if target, err := os.Readlink(path); err == nil {
			m.SymlinkTarget = target
		}
	}
	if info.IsDir() {
		f, err := openDirectory(path)
		if err != nil {
			return nil, fmt.Errorf("cannot list the selected folder: %w", err)
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return nil, errors.New("selected folder changed while inspecting it; try again")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Count a bounded number of immediate entries; the names and types of
		// only the first few are sent. Nothing below this folder is read.
		count := 0
		for count <= maxCountedChildren {
			entries, err := f.ReadDir(256)
			for _, entry := range entries {
				if len(m.Children) < maxChildren {
					m.Children = append(m.Children, child{Name: entry.Name(), Kind: itemKind(entry.Type())})
				}
				count++
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("cannot list the selected folder: %w", err)
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		m.ChildCount = &count
		m.ChildCountIsLowerBound = count > maxCountedChildren
		m.ChildrenTruncated = count > len(m.Children)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(b) > maxMetadataBytes {
		return nil, errors.New("selected item's metadata exceeds the explanation limit")
	}
	return b, nil
}

func itemKind(mode os.FileMode) string {
	switch {
	case mode&os.ModeSymlink != 0:
		return "symbolic link"
	case mode.IsDir():
		return "directory"
	case mode.IsRegular():
		return "regular file"
	default:
		return "special file"
	}
}
