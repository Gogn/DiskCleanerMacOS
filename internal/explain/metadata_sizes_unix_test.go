//go:build darwin || linux

package explain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func requireEntrySizes(t *testing.T, path string) metadata {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("Lstat did not supply allocation metadata")
	}
	m, raw := readMetadata(t, path)
	if m.LogicalSizeBytes != info.Size() || m.AllocatedSizeBytes == nil || *m.AllocatedSizeBytes != stat.Blocks*512 {
		t.Fatalf("incorrect logical or allocated entry size: %+v, stat size=%d blocks=%d", m, info.Size(), stat.Blocks)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	if _, ambiguous := fields["entry_size_bytes"]; ambiguous || fields["entry_logical_size_bytes"] == nil || fields["entry_allocated_size_bytes"] == nil {
		t.Fatal("metadata retained the ambiguous size contract", raw)
	}
	return m
}

func TestMetadataSparseFileSeparatesLogicalCapacityFromAllocatedSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Docker.raw")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	const logicalCapacity = int64(1 << 30)
	if err := f.Truncate(logicalCapacity); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("small allocated extent"), 0); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	m := requireEntrySizes(t, path)
	if m.LogicalSizeBytes != logicalCapacity || *m.AllocatedSizeBytes >= logicalCapacity || *m.AllocatedSizeBytes <= 0 {
		t.Fatalf("sparse file did not preserve distinct logical capacity and allocated usage: %+v", m)
	}
}

func TestMetadataRegularAndEmptyFileSizesMatchFilesystemBlocks(t *testing.T) {
	for _, length := range []int{0, 8192} {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, make([]byte, length), 0600); err != nil {
			t.Fatal(err)
		}
		m := requireEntrySizes(t, path)
		if m.LogicalSizeBytes != int64(length) {
			t.Fatal("regular file logical size changed", m.LogicalSizeBytes)
		}
	}
}

type allocationFileInfo struct {
	os.FileInfo
	system any
}

func (info allocationFileInfo) Sys() any { return info.system }

func TestAllocatedEntryBytesDistinguishesZeroFromUnavailable(t *testing.T) {
	for _, system := range []any{nil, struct{}{}, (*syscall.Stat_t)(nil), &syscall.Stat_t{Blocks: -1}, &syscall.Stat_t{Blocks: 1 << 62}} {
		if got := allocatedEntryBytes(allocationFileInfo{system: system}); got != nil {
			t.Fatal("unavailable or invalid allocation became a measured value", system, *got)
		}
	}
	for _, blocks := range []int64{0, 3} {
		got := allocatedEntryBytes(allocationFileInfo{system: &syscall.Stat_t{Blocks: blocks}})
		if got == nil || *got != blocks*512 {
			t.Fatal("valid allocation was not preserved", blocks, got)
		}
		raw, err := json.Marshal(metadata{AllocatedSizeBytes: got})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || string(fields["entry_allocated_size_bytes"]) != strconv.FormatInt(blocks*512, 10) {
			t.Fatal("known allocation did not serialize as its measured byte count", string(raw), err)
		}
	}
	raw, err := json.Marshal(metadata{AllocatedSizeBytes: allocatedEntryBytes(allocationFileInfo{})})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || string(fields["entry_allocated_size_bytes"]) != "null" {
		t.Fatal("unavailable allocation was not serialized explicitly as null", string(raw), err)
	}
}

func TestMetadataDirectorySizesDescribeEntryRatherThanRecursiveContents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), make([]byte, 1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	m := requireEntrySizes(t, dir)
	if m.Kind != "directory" || len(m.Children) != 1 || m.Children[0] != "child" {
		t.Fatal("directory metadata did not describe the entry and immediate child sample", m)
	}
}
