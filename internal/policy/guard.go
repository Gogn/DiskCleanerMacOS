// Package policy owns the session-long registry of protected filesystem objects.
// A Guard has one owner: do not change its context while a check is running.
package policy

import (
	"context"
	"diskcleaner/internal/ignore"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type identity struct{ dev, ino uint64 }
type rule struct {
	path string
	tree bool
}
type Guard struct {
	ScanRoot     string
	IgnoredMasks []string
	rules        []rule
	known        map[identity]bool
	initialized  bool
	discover     func() ([]rule, error)
}

func id(p string) (identity, error) {
	i, e := os.Lstat(p)
	if e != nil {
		return identity{}, e
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return identity{}, fmt.Errorf("filesystem identity unavailable")
	}
	return identity{uint64(s.Dev), s.Ino}, nil
}
func within(p, root string) bool {
	return p == root || strings.HasPrefix(p, root+string(os.PathSeparator)) || root == "/"
}
func canonical(p string) (string, error) {
	a, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	return filepath.EvalSymlinks(a)
}
func newGuard(root string, masks []string, rules []rule) (*Guard, error) {
	g := &Guard{rules: rules, known: map[identity]bool{}}
	if e := g.SetContext(root, masks); e != nil {
		return nil, e
	}
	if e := g.Refresh(); e != nil {
		return nil, e
	}
	g.initialized = true
	return g, nil
}
func NewGuard(home, configDir, scanRoot string, masks []string) (*Guard, error) {
	home, e := canonical(home)
	if e != nil {
		return nil, e
	}
	configDir, e = filepath.Abs(configDir)
	if e != nil {
		return nil, e
	}
	rs := []rule{}
	for _, p := range []string{"/", home, "/Applications", "/Library", "/Users", "/Volumes", "/private", "/opt"} {
		rs = append(rs, rule{p, false})
	}
	for _, p := range []string{"/System", "/usr", "/bin", "/sbin", "/dev", "/private/etc", "/private/var", "/Library/Apple", "/Library/Extensions", "/Library/Keychains", "/opt/homebrew", "/Applications/Finder.app", "/Applications/Safari.app", filepath.Join(home, ".Trash"), configDir} {
		rs = append(rs, rule{p, true})
	}
	for _, p := range []string{"Desktop", "Documents", "Downloads", "Library", "Movies", "Music", "Pictures"} {
		rs = append(rs, rule{filepath.Join(home, p), false})
	}
	g, e := newGuard(scanRoot, masks, rs)
	if e != nil {
		return nil, e
	}
	g.discover = func() ([]rule, error) { return systemRules(home) }
	if e = g.Refresh(); e != nil {
		return nil, e
	}
	return g, nil
}
func systemRules(home string) ([]rule, error) {
	rs := []rule{}
	users, e := os.ReadDir("/Users")
	if e != nil {
		return nil, fmt.Errorf("read user roots: %w", e)
	}
	hi, e := id(home)
	if e != nil {
		return nil, e
	}
	for _, u := range users {
		if u.Name() == "Shared" {
			continue
		}
		p := filepath.Join("/Users", u.Name())
		info, e := os.Stat(p)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() {
			continue
		}
		resolved, e := canonical(p)
		if e != nil {
			return nil, e
		}
		i, e := id(resolved)
		if e != nil {
			return nil, e
		}
		if i != hi {
			rs = append(rs, rule{resolved, true})
		}
	}
	mounts, e := mountedRoots()
	if e != nil {
		return nil, e
	}
	for _, mount := range mounts {
		rs = append(rs, rule{mount, false}, rule{filepath.Join(mount, ".Trashes"), true})
	}
	return rs, nil
}

func (g *Guard) SetContext(root string, masks []string) error {
	if g == nil {
		return fmt.Errorf("protection unavailable")
	}
	r, e := canonical(root)
	if e != nil {
		return e
	}
	i, e := os.Stat(r)
	if e != nil {
		return e
	}
	if !i.IsDir() {
		return fmt.Errorf("root is not a directory")
	}
	for _, m := range masks {
		if e := ignore.Validate(m); e != nil {
			return e
		}
	}
	g.ScanRoot = r
	g.IgnoredMasks = append([]string(nil), masks...)
	return nil
}

// Refresh retains historical identities, including objects renamed this session.
func (g *Guard) Refresh() error {
	if g == nil || g.known == nil {
		return fmt.Errorf("protection unavailable")
	}
	if g.discover != nil {
		rules, e := g.discover()
		if e != nil {
			return e
		}
		seen := map[rule]bool{}
		for _, r := range g.rules {
			seen[r] = true
		}
		for _, r := range rules {
			if !seen[r] {
				g.rules = append(g.rules, r)
				seen[r] = true
			}
		}
	}
	for _, r := range g.rules {
		p, e := canonical(r.path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return fmt.Errorf("inspect protected path %q: %w", r.path, e)
		}
		i, e := id(p)
		if e != nil {
			return e
		}
		tree, exists := g.known[i]
		g.known[i] = r.tree || (exists && tree)
	}
	return nil
}
func (g *Guard) checkObject(p string) error {
	if ignore.Any(g.IgnoredMasks, filepath.Base(p)) {
		return fmt.Errorf("excluded name: %q", filepath.Base(p))
	}
	for _, r := range g.rules {
		if p == r.path || (r.tree && within(p, r.path)) {
			return fmt.Errorf("protected path: %q", r.path)
		}
	}
	original := p
	for {
		i, e := id(p)
		if e != nil {
			return fmt.Errorf("inspect ancestor: %w", e)
		}
		if tree, ok := g.known[i]; ok && (p == original || tree) {
			return fmt.Errorf("protected object or ancestor: %q", p)
		}
		next := filepath.Dir(p)
		if next == p {
			break
		}
		p = next
	}
	mount, e := mountRoot(original)
	if e != nil {
		return e
	}
	mi, e := id(mount)
	if e != nil {
		return e
	}
	oi, e := id(original)
	if e != nil {
		return e
	}
	if oi == mi {
		return fmt.Errorf("volume root is protected")
	}
	trash := filepath.Join(mount, ".Trashes")
	if within(original, trash) {
		return fmt.Errorf("volume Trash is protected")
	}
	if ti, e := id(trash); e == nil {
		g.known[ti] = true
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}
func (g *Guard) checkPath(p string) error {
	if e := g.checkObject(p); e != nil {
		return e
	}
	info, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, e := canonical(p)
		if e != nil {
			return fmt.Errorf("resolve symbolic link: %w", e)
		}
		if !within(resolved, g.ScanRoot) {
			return fmt.Errorf("symbolic link points outside the scan root")
		}
		for ancestor := resolved; ancestor != g.ScanRoot; ancestor = filepath.Dir(ancestor) {
			if ignore.Any(g.IgnoredMasks, filepath.Base(ancestor)) {
				return fmt.Errorf("symbolic link points into an excluded subtree")
			}
		}
		if e = g.checkObject(resolved); e != nil {
			return e
		}
	}
	return nil
}
func (g *Guard) CheckTarget(p string) error {
	if g == nil || !g.initialized {
		return fmt.Errorf("protection is not initialized")
	}
	if e := g.Refresh(); e != nil {
		return e
	}
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return fmt.Errorf("target must be a clean absolute path")
	}
	parent, e := canonical(filepath.Dir(p))
	if e != nil {
		return e
	}
	parentID, e := id(parent)
	if e != nil {
		return e
	}
	rootID, e := id(g.ScanRoot)
	if e != nil {
		return e
	}
	if parentID != rootID {
		return fmt.Errorf("target is not an immediate child of the scan root")
	}
	return g.checkPath(filepath.Join(parent, filepath.Base(p)))
}
func (g *Guard) CheckTree(ctx context.Context, p string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := g.CheckTarget(p); e != nil {
		return e
	}
	rootID, e := id(g.ScanRoot)
	if e != nil {
		return e
	}
	return filepath.WalkDir(p, func(path string, d fs.DirEntry, walkErr error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if walkErr != nil {
			return walkErr
		}
		i, e := id(path)
		if e != nil {
			return e
		}
		if i.dev != rootID.dev {
			return fmt.Errorf("different volume inside selection: %q", path)
		}
		return g.checkPath(path)
	})
}
