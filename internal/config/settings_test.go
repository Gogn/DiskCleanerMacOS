package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultsAndRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings", "config.json")
	s, e := Load(p)
	if e != nil || s.Mode != Trash || len(s.Masks) != 0 || s.ExplainLanguage != "en" {
		t.Fatal(s, e)
	}
	s.Mode = Permanent
	s.ExplainLanguage = "ru"
	s.Masks = []string{"cache", "*.tmp"}
	if e = Save(p, s); e != nil {
		t.Fatal(e)
	}
	got, e := Load(p)
	if e != nil || !reflect.DeepEqual(got, s) {
		t.Fatal(got, e)
	}
	i, _ := os.Stat(p)
	if i.Mode().Perm() != 0600 {
		t.Fatal(i.Mode())
	}
	for _, v := range []string{`{`, `{"mode":"bad"}`, `{"mode":"trash","masks":{"/one":["a/b"]}}`} {
		os.WriteFile(p, []byte(v), 0600)
		if _, e = Load(p); e == nil {
			t.Fatal("accepted", v)
		}
	}
}

func TestAnswerLanguageDefaultsAndLegacySettings(t *testing.T) {
	for _, raw := range []string{
		`{"mode":"trash","masks":[]}`,
		`{"mode":"trash","masks":{"/one":["cache"]}}`,
		`{"mode":"trash","masks":[],"explain_language":""}`,
	} {
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(p)
		if err != nil || s.ExplainLanguage != "en" {
			t.Fatal("older or empty answer language did not default to English", s, err)
		}
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := Save(p, Settings{Mode: Trash}); err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	if err != nil || s.ExplainLanguage != "en" {
		t.Fatal("saving manually constructed settings lost English default", s, err)
	}
}

func TestUnsupportedAnswerLanguageIsRejected(t *testing.T) {
	for _, language := range []string{"fr", "EN", "Russian"} {
		p := filepath.Join(t.TempDir(), "config.json")
		raw := `{"mode":"trash","masks":[],"explain_language":"` + language + `"}`
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatal("invalid persisted answer language accepted", language)
		}
		s := Default()
		s.ExplainLanguage = language
		if err := Save(p, s); err == nil {
			t.Fatal("invalid answer language saved", language)
		}
	}
}

func TestLegacyMasksMigrateWithoutWritingUntilSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"mode":"trash","masks":{"/z":["cache","*.tmp","build"],"/a":["node_modules","cache","cache"],"/empty":[],"/null":null}}`)
	if err := os.WriteFile(p, original, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	want := []string{"node_modules", "cache", "*.tmp", "build"}
	if err != nil || !reflect.DeepEqual(s.Masks, want) {
		t.Fatal("legacy lists were not merged in deterministic order", s, err)
	}
	unchanged, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("Load modified settings on disk", err)
	}
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Masks []string `json:"masks"`
	}
	if err := json.Unmarshal(b, &stored); err != nil || !reflect.DeepEqual(stored.Masks, want) {
		t.Fatal("Save did not write a global array", string(b), err)
	}
	loaded, err := Load(p)
	if err != nil || !reflect.DeepEqual(loaded, s) {
		t.Fatal("migrated settings did not round trip", loaded, err)
	}
	s.Masks = []string{}
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(p)
	if err != nil || loaded.Masks == nil || len(loaded.Masks) != 0 {
		t.Fatal("clearing global list restored legacy rules", loaded, err)
	}
}

func TestEmptyMasksRemainGlobalAndEmpty(t *testing.T) {
	for _, raw := range []string{``, `[]`, `null`, `{}`, `{"/one":[],"/two":null}`} {
		t.Run(raw, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			body := `{"mode":"trash"}`
			if raw != "" {
				body = `{"mode":"trash","masks":` + raw + `}`
			}
			if err := os.WriteFile(p, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := Load(p)
			if err != nil || s.Masks == nil || len(s.Masks) != 0 {
				t.Fatal("empty list changed", s, err)
			}
			// Saving nil must also produce the same empty array format.
			s.Masks = nil
			if err := Save(p, s); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(p)
			if err != nil || !bytes.Contains(b, []byte(`"masks": []`)) {
				t.Fatal("empty list was not saved as an array", string(b), err)
			}
			loaded, err := Load(p)
			if err != nil || loaded.Masks == nil || len(loaded.Masks) != 0 {
				t.Fatal("empty list changed after reload", loaded, err)
			}
		})
	}
}

func TestInvalidGlobalAndLegacyMasksRejected(t *testing.T) {
	for _, masks := range []string{
		`["a/b"]`, `["["]`, `[1]`, `[null]`, `"cache"`, `42`,
		`{"relative":["cache"]}`, `{"/one/../two":[]}`,
		`{"/one":["valid"],"/two":["a/b"]}`, `{"/one":"cache"}`, `{"/one":[null]}`, `{"/one":[1]}`,
	} {
		t.Run(masks, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(p, []byte(`{"mode":"trash","masks":`+masks+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil {
				t.Fatal("accepted invalid masks", masks)
			}
		})
	}
}
