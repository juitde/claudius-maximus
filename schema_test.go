package main

import (
	"os"
	"strings"
	"testing"
)

func TestCheckSchemaVersion(t *testing.T) {
	t.Run("current version passes", func(t *testing.T) {
		if err := checkSchemaVersion("/x/state.json", stateSchemaVersion, "do something"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("a newer format says the binary is behind", func(t *testing.T) {
		err := checkSchemaVersion("/x/state.json", stateSchemaVersion+1, "do something")
		if err == nil {
			t.Fatal("expected an error")
		}
		// The remedy here is updating this tool, not touching the file.
		if !strings.Contains(err.Error(), "update "+appName) {
			t.Errorf("got %q", err)
		}
	})

	t.Run("an older format carries the caller's advice", func(t *testing.T) {
		err := checkSchemaVersion("/x/state.json", stateSchemaVersion-1, "run the thing")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "run the thing") {
			t.Errorf("the caller's remedy should reach the user: %q", err)
		}
	})

	t.Run("the path is always named", func(t *testing.T) {
		for _, found := range []int{stateSchemaVersion - 1, stateSchemaVersion + 1} {
			err := checkSchemaVersion("/x/state.json", found, "advice")
			if err == nil || !strings.Contains(err.Error(), "/x/state.json") {
				t.Errorf("version %d: error should name the file, got %v", found, err)
			}
		}
	})
}

func TestStateFilesCarryTheSchemaVersion(t *testing.T) {
	stateDir := t.TempDir()

	t.Run("project cache", func(t *testing.T) {
		path := stateFile(stateDir, "projects.json")
		if err := saveProjectCache(path, &ProjectCache{Projects: []Project{{Name: "api", Path: "/x"}}}); err != nil {
			t.Fatalf("save: %v", err)
		}

		loaded, err := loadProjectCache(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if loaded.SchemaVersion != stateSchemaVersion {
			t.Errorf("schema version = %d, want %d", loaded.SchemaVersion, stateSchemaVersion)
		}
	})

	t.Run("environment registry", func(t *testing.T) {
		reg := newRegistry(stateDir)
		if err := reg.Put(testEnvironment("/x/api", "api")); err != nil {
			t.Fatalf("put: %v", err)
		}

		raw := mustRead(t, stateFile(stateDir, "environments.json"))
		// The wrapping object is the point: a bare array has nowhere to put a
		// version, which is why the shape was chosen before anything shipped.
		if !strings.Contains(raw, `"schema_version"`) {
			t.Errorf("no schema version in the file:\n%s", raw)
		}
		if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
			t.Errorf("expected an object, got:\n%s", raw)
		}
	})
}

func TestStateFilesRejectAFutureVersion(t *testing.T) {
	stateDir := t.TempDir()

	t.Run("project cache", func(t *testing.T) {
		path := stateFile(stateDir, "projects.json")
		mustWrite(t, path, `{"schema_version":99,"projects":[]}`)

		if _, err := loadProjectCache(path); err == nil {
			t.Fatal("expected an error")
		} else if !strings.Contains(err.Error(), "rescan") && !strings.Contains(err.Error(), "update") {
			t.Errorf("error should say what to do: %v", err)
		}
	})

	t.Run("environment registry", func(t *testing.T) {
		mustWrite(t, stateFile(stateDir, "environments.json"), `{"schema_version":99,"environments":[]}`)

		_, err := newRegistry(stateDir).List()
		if err == nil {
			t.Fatal("expected an error")
		}
		// Discarding this file orphans running processes, so the advice must
		// not be "just delete it" without the stop step.
		if !strings.Contains(err.Error(), "update") {
			t.Errorf("got %v", err)
		}
	})
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
