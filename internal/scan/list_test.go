package scan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListSkipsEntryRemovedAfterReadingDirectory(t *testing.T) {
	root := t.TempDir()
	vanishing := filepath.Join(root, "a-vanishing")
	kept := filepath.Join(root, "b-kept")
	for _, path := range []string{vanishing, kept} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := listWithLstat(root, func(path string) (os.FileInfo, error) {
		// This callback runs after ReadDir captured the name, reproducing a
		// temporary file disappearing before its metadata can be inspected.
		if path == vanishing {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		return os.Lstat(path)
	})
	if err != nil || len(entries) != 1 || entries[0].Path != kept {
		t.Fatalf("a disappeared child prevented listing the surviving entry: %+v, %v", entries, err)
	}
}

func TestListPreservesOtherMetadataErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, cause := range []error{os.ErrPermission, errors.New("metadata unavailable")} {
		entries, err := listWithLstat(root, func(path string) (os.FileInfo, error) {
			return nil, &os.PathError{Op: "lstat", Path: path, Err: cause}
		})
		if !errors.Is(err, cause) || entries != nil {
			t.Fatalf("metadata failure was hidden: %+v, %v", entries, err)
		}
	}
}
