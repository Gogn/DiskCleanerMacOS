package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (string, *Guard) {
	t.Helper()
	d := t.TempDir()
	g, e := newGuard(d, nil, []rule{{filepath.Join(d, "protected"), true}, {filepath.Join(d, "object"), false}})
	if e != nil {
		t.Fatal(e)
	}
	return d, g
}
func TestProtectedPathsAndDescendants(t *testing.T) {
	d, g := fixture(t)
	for _, p := range []string{"protected", "object", "ordinary"} {
		os.Mkdir(filepath.Join(d, p), 0700)
		os.WriteFile(filepath.Join(d, p, "child"), nil, 0600)
	}
	if e := g.Refresh(); e != nil {
		t.Fatal(e)
	}
	if g.CheckTarget(filepath.Join(d, "object")) == nil || g.CheckTree(context.Background(), filepath.Join(d, "protected")) == nil {
		t.Fatal("protected permitted")
	}
	if e := g.CheckTarget(filepath.Join(d, "ordinary")); e != nil {
		t.Fatal(e)
	}
	g.SetContext(filepath.Join(d, "object"), nil)
	if e := g.CheckTarget(filepath.Join(d, "object", "child")); e != nil {
		t.Fatal(e)
	}
	g.SetContext(filepath.Join(d, "protected"), nil)
	if g.CheckTarget(filepath.Join(d, "protected", "child")) == nil {
		t.Fatal("subtree permitted")
	}
	var zero Guard
	if zero.CheckTarget(d) == nil {
		t.Fatal("zero guard permitted")
	}
}
func TestProtectedObjectIdentity(t *testing.T) {
	d, g := fixture(t)
	p := filepath.Join(d, "protected")
	os.Mkdir(p, 0700)
	os.WriteFile(filepath.Join(p, "child"), nil, 0600)
	g.Refresh()
	os.Rename(p, filepath.Join(d, "renamed"))
	if g.CheckTarget(filepath.Join(d, "renamed")) == nil {
		t.Fatal("renamed object allowed")
	}
	g.SetContext(filepath.Join(d, "renamed"), nil)
	if g.CheckTarget(filepath.Join(d, "renamed", "child")) == nil {
		t.Fatal("renamed subtree allowed")
	}
}
func TestParentBlockedByProtectedIgnoredOrUnreadableChild(t *testing.T) {
	d, g := fixture(t)
	p := filepath.Join(d, "parent")
	os.MkdirAll(filepath.Join(p, "a"), 0700)
	os.WriteFile(filepath.Join(p, "a", "cache"), nil, 0600)
	g.SetContext(d, []string{"cache"})
	if g.CheckTree(context.Background(), p) == nil {
		t.Fatal("ignored descendant allowed")
	}
	if g.CheckTarget(filepath.Join(p, "a")) == nil {
		t.Fatal("grandchild allowed")
	}
	g.SetContext(d, nil)
	os.Chmod(filepath.Join(p, "a"), 0000)
	defer os.Chmod(filepath.Join(p, "a"), 0700)
	if g.CheckTree(context.Background(), p) == nil {
		t.Fatal("unreadable allowed")
	}
}
func TestSymlinkCannotBypassGuard(t *testing.T) {
	d, g := fixture(t)
	os.Mkdir(filepath.Join(d, "protected"), 0700)
	os.Symlink(filepath.Join(d, "protected"), filepath.Join(d, "link"))
	if g.CheckTree(context.Background(), filepath.Join(d, "link")) == nil {
		t.Fatal("protected link allowed")
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(d, "outside"))
	if g.CheckTarget(filepath.Join(d, "outside")) == nil {
		t.Fatal("outside link allowed")
	}
}

func TestDynamicProtectedRootsRetainedAcrossRename(t *testing.T) {
	d, g := fixture(t)
	p := filepath.Join(d, "volume-trash")
	os.Mkdir(p, 0700)
	os.WriteFile(filepath.Join(p, "child"), nil, 0600)
	g.discover = func() ([]rule, error) { return []rule{{p, true}}, nil }
	if e := g.Refresh(); e != nil {
		t.Fatal(e)
	}
	renamed := filepath.Join(d, "renamed-trash")
	os.Rename(p, renamed)
	if g.CheckTarget(renamed) == nil {
		t.Fatal("renamed dynamically discovered protected root permitted")
	}
	if e := g.SetContext(renamed, nil); e != nil {
		t.Fatal(e)
	}
	if g.CheckTarget(filepath.Join(renamed, "child")) == nil {
		t.Fatal("renamed subtree permitted")
	}
}
func TestProtectedIdentityCaseAndUnicode(t *testing.T) {
	for _, names := range [][2]string{{"Protected", "protected"}, {"caf\u00e9", "cafe\u0301"}} {
		t.Run(names[0], func(t *testing.T) {
			d := t.TempDir()
			p := filepath.Join(d, names[0])
			alias := filepath.Join(d, names[1])
			os.Mkdir(p, 0700)
			g, e := newGuard(d, nil, []rule{{p, true}})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = os.Stat(alias); e != nil {
				t.Skip("test volume does not provide this equivalent spelling")
			}
			if g.CheckTarget(alias) == nil {
				t.Fatal("alias bypass")
			}
		})
	}
}

func TestSymlinkIntoExcludedAncestorIsBlocked(t *testing.T) {
	d, g := fixture(t)
	os.Mkdir(filepath.Join(d, "cache"), 0700)
	os.WriteFile(filepath.Join(d, "cache", "child"), nil, 0600)
	os.Symlink(filepath.Join(d, "cache", "child"), filepath.Join(d, "link"))
	g.SetContext(d, []string{"cache"})
	if g.CheckTarget(filepath.Join(d, "link")) == nil {
		t.Fatal("link into excluded subtree allowed")
	}
}

func TestSystemProtectedRules(t *testing.T) {
	home, e := os.UserHomeDir()
	if e != nil {
		t.Fatal(e)
	}
	configDir := filepath.Join(home, "Library", "Application Support", "DiskCleaner")
	g, e := NewGuard(home, configDir, home, nil)
	if e != nil {
		t.Fatal(e)
	}
	paths := []string{"/", "/System", "/usr", "/bin", "/sbin", "/dev", "/private/etc", "/private/var", "/var", "/Library/Apple", "/Library/Extensions", "/Library/Keychains", "/opt/homebrew", "/System/Applications", "/Applications/Finder.app", "/Applications/Safari.app", "/Applications", "/Library", "/Users", "/Volumes", "/private", "/opt", home, configDir}
	for _, n := range []string{"Desktop", "Documents", "Downloads", "Library", "Movies", "Music", "Pictures", ".Trash"} {
		paths = append(paths, filepath.Join(home, n))
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			if _, e := os.Lstat(p); os.IsNotExist(e) {
				t.Skip("optional protected path absent")
			}
			if e := g.SetContext(filepath.Dir(p), nil); e != nil {
				t.Fatal(e)
			}
			if g.CheckTarget(p) == nil {
				t.Fatal("protected path accepted")
			}
		})
	}
}
func TestAllowedInRootSymlinkIsNotTraversed(t *testing.T) {
	d, g := fixture(t)
	os.Mkdir(filepath.Join(d, "normal"), 0700)
	os.WriteFile(filepath.Join(d, "normal", "child"), nil, 0600)
	link := filepath.Join(d, "link")
	os.Symlink(filepath.Join(d, "normal"), link)
	if e := g.CheckTree(context.Background(), link); e != nil {
		t.Fatal(e)
	}
}
