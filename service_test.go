package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestService builds a Service over a throwaway state directory containing
// the given config, and returns it alongside that directory.
func newTestService(t *testing.T, cfg Config) (*Service, string) {
	t.Helper()

	stateDir := t.TempDir()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	mustWrite(t, configFile(stateDir), string(data))

	return newService(stateDir), stateDir
}

func TestServiceListProjectsReadsOnlyTheCache(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})

	// Before any rescan the cache is empty, even though the project exists on
	// disk. That is the contract: list never scans.
	cache, err := svc.ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.Projects) != 0 {
		t.Fatalf("expected empty cache before rescan, got %v", cache.Projects)
	}

	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	cache, err = svc.ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.Projects) != 1 || cache.Projects[0].Name != "api" {
		t.Fatalf("expected the scanned project in the cache, got %v", cache.Projects)
	}
}

func TestServiceListProjectsDoesNotSeeLaterChanges(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// A project appearing after the rescan must stay invisible until the next
	// one, otherwise the cache would not be a stable shared view.
	makeProject(t, filepath.Join(root, "web"), "package.json")

	cache, err := svc.ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.Projects) != 1 {
		t.Fatalf("cache should still hold 1 project, got %v", cache.Projects)
	}

	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("second rescan: %v", err)
	}
	cache, _ = svc.ListProjects()
	if len(cache.Projects) != 2 {
		t.Fatalf("after rescan expected 2 projects, got %v", cache.Projects)
	}
}

func TestServiceRescanAssignsNamesAndSorts(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "dev", "api"), "go.mod")
	makeProject(t, filepath.Join(root, "work", "api"), "go.mod")
	makeProject(t, filepath.Join(root, "dev", "frontend"), "package.json")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*", "*")}})

	result, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// Sorted by name, with the two colliding "api" projects lifted a level and
	// "frontend" left short.
	want := []string{"dev-api", "frontend", "work-api"}
	if len(result.Cache.Projects) != len(want) {
		t.Fatalf("got %d projects (%v), want %d", len(result.Cache.Projects), result.Cache.Projects, len(want))
	}
	for i, w := range want {
		if got := result.Cache.Projects[i].Name; got != w {
			t.Errorf("project %d: got name %q, want %q", i, got, w)
		}
	}
}

func TestServiceRescanSurfacesWarnings(t *testing.T) {
	svc, _ := newTestService(t, Config{
		ProjectGlobs: []string{filepath.Join(t.TempDir(), "nothing-here", "*")},
	})

	result, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if !hasWarningContaining(result.Warnings, "matched nothing") {
		t.Errorf("expected the unusable pattern to be reported, got %v", result.Warnings)
	}
}

func TestServiceRescanPersistsCacheToDisk(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, stateDir := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})

	before := time.Now().Add(-time.Second)
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// A fresh Service over the same directory must see the same data — the
	// cache is what the CLI and the MCP server share.
	cache, err := newService(stateDir).ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.Projects) != 1 {
		t.Fatalf("expected 1 project from disk, got %v", cache.Projects)
	}
	if cache.ScannedAt.Before(before) {
		t.Errorf("ScannedAt %v was not updated", cache.ScannedAt)
	}
}

func TestServiceRescanFailsOnMalformedConfig(t *testing.T) {
	stateDir := t.TempDir()
	mustWrite(t, configFile(stateDir), `{"project_globs":`)

	if _, err := newService(stateDir).Rescan(); err == nil {
		t.Fatal("expected an error for a malformed config")
	}
}

func TestServiceRescanWithoutConfig(t *testing.T) {
	// A missing config is a normal first run: no projects, no error.
	result, err := newService(t.TempDir()).Rescan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Cache.Projects) != 0 {
		t.Errorf("expected no projects, got %v", result.Cache.Projects)
	}
}

func TestProjectCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")

	original := &ProjectCache{
		ScannedAt: time.Now().Truncate(time.Second),
		Projects: []Project{
			{Name: "api", Path: "/home/u/dev/api"},
			{Name: "web", Path: "/home/u/dev/web"},
		},
	}
	if err := saveProjectCache(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadProjectCache(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.ScannedAt.Equal(original.ScannedAt) {
		t.Errorf("ScannedAt: got %v, want %v", loaded.ScannedAt, original.ScannedAt)
	}
	if len(loaded.Projects) != len(original.Projects) {
		t.Fatalf("got %d projects, want %d", len(loaded.Projects), len(original.Projects))
	}
	for i := range original.Projects {
		if loaded.Projects[i] != original.Projects[i] {
			t.Errorf("project %d: got %+v, want %+v", i, loaded.Projects[i], original.Projects[i])
		}
	}
}

func TestSaveProjectCacheLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	if err := saveProjectCache(path, &ProjectCache{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "projects.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected only projects.json, found %v", names)
	}
}

func TestResolveStateDir(t *testing.T) {
	t.Run("environment override wins", func(t *testing.T) {
		want := t.TempDir()
		t.Setenv(envHome, want)

		got, err := resolveStateDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("defaults below the home directory", func(t *testing.T) {
		t.Setenv(envHome, "")

		got, err := resolveStateDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory available")
		}
		if want := filepath.Join(home, "."+appName); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
