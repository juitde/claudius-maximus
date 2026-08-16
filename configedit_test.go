package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestConfigSchemaMatchesStruct is the guard that lets the schema be trusted:
// hand-written metadata and the Config struct must describe the same set of
// properties, in both directions.
func TestConfigSchemaMatchesStruct(t *testing.T) {
	specs := configSchema()

	inSchema := map[string]bool{}
	for _, s := range specs {
		inSchema[s.Name] = true

		if s.Description == "" {
			t.Errorf("property %q has no description", s.Name)
		}
		if s.Type == "" {
			t.Errorf("property %q has no type", s.Name)
		}
		if _, ok := propertyMeta[s.Name]; !ok {
			t.Errorf("property %q exists on Config but has no metadata entry", s.Name)
		}
	}

	for name := range propertyMeta {
		if !inSchema[name] {
			t.Errorf("metadata entry %q does not correspond to any Config field", name)
		}
	}

	// Every exported field carrying a json tag must show up.
	tp := reflect.TypeOf(Config{})
	for i := range tp.NumField() {
		if name := jsonFieldName(tp.Field(i)); name != "" && !inSchema[name] {
			t.Errorf("Config field %q is missing from the schema", name)
		}
	}
}

func TestApplyConfigEditRejectsUnknownProperty(t *testing.T) {
	cfg := &Config{}
	_, err := applyConfigEdit(cfg, ConfigEdit{Operation: ConfigOpAdd, Property: "projekt_globs", Values: []string{"~/dev/*"}})
	if err == nil {
		t.Fatal("expected an error for an unknown property")
	}
	// The message has to point at the valid names, otherwise the user is stuck.
	for _, name := range configPropertyNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention valid property %q", err, name)
		}
	}
}

func TestApplyConfigEditSet(t *testing.T) {
	t.Run("replaces the value", func(t *testing.T) {
		cfg := &Config{ProjectGlobs: []string{"~/old/*"}}
		if _, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpSet, Property: "project_globs",
			Values: []string{"~/dev/*", "~/work/*"},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*", "~/work/*"}) {
			t.Errorf("got %v", cfg.ProjectGlobs)
		}
	})

	t.Run("deduplicates", func(t *testing.T) {
		cfg := &Config{}
		if _, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpSet, Property: "project_globs",
			Values: []string{"~/dev/*", "~/dev/*"},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*"}) {
			t.Errorf("got %v, want the duplicate collapsed", cfg.ProjectGlobs)
		}
	})

	t.Run("with no values yields an empty but present list", func(t *testing.T) {
		cfg := &Config{ProjectMarkers: []string{"go.mod"}}
		if _, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpSet, Property: "project_markers",
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ProjectMarkers == nil {
			t.Fatal("set with no values must produce an empty list, not nil")
		}
		if len(cfg.ProjectMarkers) != 0 {
			t.Errorf("got %v, want empty", cfg.ProjectMarkers)
		}
		// Empty means "no filtering", which is exactly not the default set.
		if equalStrings(cfg.markers(), defaultProjectMarkers) {
			t.Error("an explicitly empty list must not fall back to the defaults")
		}
	})
}

func TestApplyConfigEditUnset(t *testing.T) {
	cfg := &Config{ProjectMarkers: []string{"go.mod"}}
	if _, err := applyConfigEdit(cfg, ConfigEdit{
		Operation: ConfigOpUnset, Property: "project_markers",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectMarkers != nil {
		t.Errorf("got %v, want nil", cfg.ProjectMarkers)
	}
	if !equalStrings(cfg.markers(), defaultProjectMarkers) {
		t.Error("unset must restore the defaults")
	}

	t.Run("rejects values", func(t *testing.T) {
		_, err := applyConfigEdit(&Config{}, ConfigEdit{
			Operation: ConfigOpUnset, Property: "project_globs", Values: []string{"x"},
		})
		if err == nil {
			t.Error("expected an error when unset is given values")
		}
	})
}

func TestApplyConfigEditAdd(t *testing.T) {
	t.Run("appends", func(t *testing.T) {
		cfg := &Config{ProjectGlobs: []string{"~/dev/*"}}
		notes, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpAdd, Property: "project_globs", Values: []string{"~/work/*"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*", "~/work/*"}) {
			t.Errorf("got %v", cfg.ProjectGlobs)
		}
		if len(notes) != 0 {
			t.Errorf("unexpected notes: %v", notes)
		}
	})

	t.Run("skips duplicates with a note", func(t *testing.T) {
		cfg := &Config{ProjectGlobs: []string{"~/dev/*"}}
		notes, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpAdd, Property: "project_globs", Values: []string{"~/dev/*"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.ProjectGlobs) != 1 {
			t.Errorf("got %v, want no duplicate", cfg.ProjectGlobs)
		}
		if !hasWarningContaining(notes, "already present") {
			t.Errorf("expected a note about the duplicate, got %v", notes)
		}
	})

	t.Run("seeds from defaults when the property is unset", func(t *testing.T) {
		// Adding a marker must extend what is in force, not silently replace
		// the whole default set with one entry.
		cfg := &Config{}
		notes, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpAdd, Property: "project_markers", Values: []string{"Justfile"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.ProjectMarkers) != len(defaultProjectMarkers)+1 {
			t.Fatalf("got %d markers, want the defaults plus one", len(cfg.ProjectMarkers))
		}
		if !contains(cfg.ProjectMarkers, "go.mod") {
			t.Error("defaults were dropped instead of seeded")
		}
		if !contains(cfg.ProjectMarkers, "Justfile") {
			t.Error("the new marker is missing")
		}
		if !hasWarningContaining(notes, "was unset") {
			t.Errorf("the user must be told the defaults were materialised, got %v", notes)
		}
	})

	t.Run("does not seed a property without defaults", func(t *testing.T) {
		cfg := &Config{}
		notes, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpAdd, Property: "project_globs", Values: []string{"~/dev/*"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*"}) {
			t.Errorf("got %v", cfg.ProjectGlobs)
		}
		if len(notes) != 0 {
			t.Errorf("unexpected notes: %v", notes)
		}
	})

	t.Run("requires a value", func(t *testing.T) {
		if _, err := applyConfigEdit(&Config{}, ConfigEdit{
			Operation: ConfigOpAdd, Property: "project_globs",
		}); err == nil {
			t.Error("expected an error when add is given no values")
		}
	})
}

func TestApplyConfigEditRemove(t *testing.T) {
	t.Run("drops the value", func(t *testing.T) {
		cfg := &Config{ProjectGlobs: []string{"~/dev/*", "~/work/*"}}
		if _, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpRemove, Property: "project_globs", Values: []string{"~/dev/*"},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equalStrings(cfg.ProjectGlobs, []string{"~/work/*"}) {
			t.Errorf("got %v", cfg.ProjectGlobs)
		}
	})

	t.Run("removing an absent value fails without changing anything", func(t *testing.T) {
		cfg := &Config{ProjectGlobs: []string{"~/dev/*"}}
		_, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpRemove, Property: "project_globs",
			Values: []string{"~/dev/*", "~/typo/*"},
		})
		if err == nil {
			t.Fatal("expected an error for the absent value")
		}
		// Atomicity: the valid value must still be there.
		if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*"}) {
			t.Errorf("config was modified despite the error: %v", cfg.ProjectGlobs)
		}
	})

	t.Run("can remove a built-in default", func(t *testing.T) {
		cfg := &Config{}
		if _, err := applyConfigEdit(cfg, ConfigEdit{
			Operation: ConfigOpRemove, Property: "project_markers", Values: []string{"Makefile"},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if contains(cfg.ProjectMarkers, "Makefile") {
			t.Error("Makefile should have been removed")
		}
		if !contains(cfg.ProjectMarkers, "go.mod") {
			t.Error("the other defaults should have survived")
		}
	})
}

func TestApplyConfigEditValidatesValues(t *testing.T) {
	tests := []struct {
		name     string
		property string
		value    string
	}{
		{"blank glob", "project_globs", "   "},
		{"malformed glob", "project_globs", "~/dev/["},
		{"blank marker", "project_markers", ""},
		{"marker with a path separator", "project_markers", "src/go.mod"},
		{"malformed marker pattern", "project_markers", "[unterminated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			_, err := applyConfigEdit(cfg, ConfigEdit{
				Operation: ConfigOpAdd, Property: tt.property, Values: []string{tt.value},
			})
			if err == nil {
				t.Fatalf("expected %q to be rejected", tt.value)
			}
			// Rejected means untouched, not partially applied.
			if cfg.ProjectGlobs != nil || cfg.ProjectMarkers != nil {
				t.Errorf("config was modified by a rejected edit: %+v", cfg)
			}
		})
	}
}

// --- persistence ---

func TestSaveConfigPreservesTheThreeStates(t *testing.T) {
	tests := []struct {
		name          string
		markers       []string
		wantKey       bool
		wantEffective []string
	}{
		{"unset omits the key", nil, false, defaultProjectMarkers},
		{"empty list is written and kept", []string{}, true, []string{}},
		{"explicit list is written", []string{"go.mod"}, true, []string{"go.mod"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := saveConfig(path, &Config{
				ProjectGlobs:   []string{"~/dev/*"},
				ProjectMarkers: tt.markers,
			}); err != nil {
				t.Fatalf("save: %v", err)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var asMap map[string]json.RawMessage
			if err := json.Unmarshal(raw, &asMap); err != nil {
				t.Fatalf("parse written file: %v", err)
			}
			if _, present := asMap["project_markers"]; present != tt.wantKey {
				t.Errorf("project_markers present = %v, want %v (file: %s)", present, tt.wantKey, raw)
			}

			// The round trip is what actually matters.
			loaded, err := loadConfig(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !equalStrings(loaded.markers(), tt.wantEffective) {
				t.Errorf("effective markers = %v, want %v", loaded.markers(), tt.wantEffective)
			}
		})
	}
}

func TestLoadConfigRejectsUnknownProperty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	mustWrite(t, path, `{"projekt_globs":["~/dev/*"]}`)

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected an error for an unknown property")
	}
	if !strings.Contains(err.Error(), "project_globs") {
		t.Errorf("error %q should list the valid properties", err)
	}
}

// --- service level ---

func TestServiceEditConfigPersists(t *testing.T) {
	stateDir := t.TempDir()
	svc := newService(stateDir)

	if _, err := svc.EditConfig(ConfigEdit{
		Operation: ConfigOpAdd, Property: "project_globs", Values: []string{"~/dev/*"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A fresh Service must see the change.
	cfg, err := newService(stateDir).ShowConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalStrings(cfg.ProjectGlobs, []string{"~/dev/*"}) {
		t.Errorf("got %v", cfg.ProjectGlobs)
	}
}

func TestServiceEditConfigLeavesFileUntouchedOnError(t *testing.T) {
	stateDir := t.TempDir()
	path := filepath.Join(stateDir, "config.json")
	mustWrite(t, path, "{\n  \"project_globs\": [\n    \"~/dev/*\"\n  ]\n}\n")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if _, err := newService(stateDir).EditConfig(ConfigEdit{
		Operation: ConfigOpAdd, Property: "project_markers", Values: []string{"bad/marker"},
	}); err == nil {
		t.Fatal("expected the invalid value to be rejected")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file changed despite the error:\nbefore: %s\nafter:  %s", before, after)
	}
}
