package config

import (
	"bytes"
	"diskcleaner/internal/ignore"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type DeleteMode string

const (
	Trash     DeleteMode = "trash"
	Permanent DeleteMode = "permanent"
)

type Settings struct {
	Mode            DeleteMode `json:"mode"`
	Masks           []string   `json:"masks"`
	ExplainLanguage string     `json:"explain_language"`
}

func Default() Settings { return Settings{Mode: Trash, Masks: []string{}, ExplainLanguage: "en"} }

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
	if s.ExplainLanguage != "en" && s.ExplainLanguage != "ru" {
		return fmt.Errorf("invalid AI answer language %q", s.ExplainLanguage)
	}
	for _, m := range s.Masks {
		if e := ignore.Validate(m); e != nil {
			return e
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
	var stored struct {
		Mode            DeleteMode      `json:"mode"`
		Masks           json.RawMessage `json:"masks"`
		ExplainLanguage string          `json:"explain_language"`
	}
	if e = json.Unmarshal(b, &stored); e != nil {
		return Settings{}, fmt.Errorf("invalid settings: %w", e)
	}
	s := Settings{Mode: stored.Mode, Masks: []string{}, ExplainLanguage: stored.ExplainLanguage}
	if s.ExplainLanguage == "" {
		s.ExplainLanguage = "en"
	}
	raw := bytes.TrimSpace(stored.Masks)
	if len(raw) > 0 && raw[0] == '{' {
		var legacy map[string][]string
		if e = json.Unmarshal(raw, &legacy); e != nil {
			return Settings{}, fmt.Errorf("invalid settings: %w", e)
		}
		roots := make([]string, 0, len(legacy))
		for root := range legacy {
			if !filepath.IsAbs(root) || filepath.Clean(root) != root {
				return Settings{}, fmt.Errorf("invalid root %q", root)
			}
			roots = append(roots, root)
		}
		sort.Strings(roots)
		seen := map[string]bool{}
		for _, root := range roots {
			for _, mask := range legacy[root] {
				if !seen[mask] {
					s.Masks = append(s.Masks, mask)
					seen[mask] = true
				}
			}
		}
	} else if len(raw) > 0 {
		if e = json.Unmarshal(raw, &s.Masks); e != nil {
			return Settings{}, fmt.Errorf("invalid settings: %w", e)
		}
	}
	if e = validate(s); e != nil {
		return Settings{}, e
	}
	if s.Masks == nil {
		s.Masks = []string{}
	}
	return s, nil
}
func Save(p string, s Settings) error {
	if s.ExplainLanguage == "" {
		s.ExplainLanguage = "en"
	}
	if e := validate(s); e != nil {
		return e
	}
	if s.Masks == nil {
		s.Masks = []string{}
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
