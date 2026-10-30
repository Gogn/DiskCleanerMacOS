package config

import (
	"diskcleaner/internal/ignore"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type DeleteMode string

const (
	Trash     DeleteMode = "trash"
	Permanent DeleteMode = "permanent"
)

type Settings struct {
	Mode  DeleteMode          `json:"mode"`
	Masks map[string][]string `json:"masks"`
}

func Default() Settings { return Settings{Mode: Trash, Masks: map[string][]string{}} }
func Path() (string, error) {
	h, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(h, "Library", "Application Support", "DiskCleaner", "config.json"), nil
}
func validate(s Settings) error {
	if s.Mode != Trash && s.Mode != Permanent {
		return fmt.Errorf("invalid deletion mode %q", s.Mode)
	}
	for root, ms := range s.Masks {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return fmt.Errorf("invalid root %q", root)
		}
		for _, m := range ms {
			if e := ignore.Validate(m); e != nil {
				return e
			}
		}
	}
	return nil
}
func Load(p string) (Settings, error) {
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return Default(), nil
	}
	if e != nil {
		return Settings{}, e
	}
	var s Settings
	if e = json.Unmarshal(b, &s); e != nil {
		return Settings{}, fmt.Errorf("invalid settings: %w", e)
	}
	if e = validate(s); e != nil {
		return Settings{}, e
	}
	if s.Masks == nil {
		s.Masks = map[string][]string{}
	}
	return s, nil
}
func Save(p string, s Settings) error {
	if e := validate(s); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Dir(p)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".config-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, p)
}
