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
	maxChildren      = 32
	maxMetadataBytes = 32 * 1024
)

// Metadata is an observation of the directory entry, not a recursive scan.
// Both size fields for a directory describe its entry, not its contents.
type metadata struct {
	Path               string   `json:"path"`
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	LogicalSizeBytes   int64    `json:"entry_logical_size_bytes"`
	AllocatedSizeBytes *int64   `json:"entry_allocated_size_bytes"`
	ModifiedAt         string   `json:"modified_at"`
	Children           []string `json:"immediate_child_names,omitempty"`
	ChildrenTruncated  bool     `json:"child_list_truncated,omitempty"`
}

func collectMetadata(ctx context.Context, req Request) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, errors.New("no item selected for explanation")
	}
	path, err := filepath.Abs(req.Path)
	if err != nil {
		return nil, errors.New("cannot resolve the selected item's path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect the selected item: %w", err)
	}
	m := metadata{Path: path, Name: filepath.Base(path), Kind: itemKind(info.Mode()), LogicalSizeBytes: info.Size(), AllocatedSizeBytes: allocatedEntryBytes(info), ModifiedAt: info.ModTime().Format(time.RFC3339Nano)}
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
		entries, err := f.ReadDir(maxChildren + 1)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("cannot list the selected folder: %w", err)
		}
		m.ChildrenTruncated = len(entries) > maxChildren
		if m.ChildrenTruncated {
			entries = entries[:maxChildren]
		}
		for _, entry := range entries {
			m.Children = append(m.Children, entry.Name())
		}
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
