package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEnvironment(path, name string) Environment {
	return Environment{
		ProjectPath:   path,
		ProjectName:   name,
		EnvironmentID: "env_" + name,
		URL:           "https://claude.ai/code?environment=env_" + name,
		PID:           4242,
		StartedAt:     time.Now().Truncate(time.Second),
		LogFile:       filepath.Join(path, ".log"),
		SpawnMode:     SpawnSameDir,
	}
}

func alwaysAlive(Environment) bool { return true }

func TestRegistryEmptyBeforeAnythingIsStored(t *testing.T) {
	reg := newRegistry(t.TempDir())

	got, err := reg.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}

	env, err := reg.Get("/nowhere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != nil {
		t.Errorf("got %+v, want nil", env)
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	want := testEnvironment("/home/u/dev/api", "api")

	if err := newRegistry(stateDir).Put(want); err != nil {
		t.Fatalf("put: %v", err)
	}

	// A fresh Registry over the same directory has to see it — the file is
	// what the CLI and the MCP server share.
	got, err := newRegistry(stateDir).Get(want.ProjectPath)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("stored environment not found")
	}
	if got.EnvironmentID != want.EnvironmentID || got.URL != want.URL {
		t.Errorf("identity fields differ: got %+v", got)
	}
	if got.PID != want.PID || got.LogFile != want.LogFile {
		t.Errorf("process fields differ: got %+v", got)
	}
	if got.SpawnMode != want.SpawnMode {
		t.Errorf("spawn mode = %q, want %q", got.SpawnMode, want.SpawnMode)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
}

func TestRegistryPutReplacesRatherThanAppends(t *testing.T) {
	// The behaviour that follows from claude's model: a second start against
	// the same directory reconnects to the same environment. Appending would
	// leave two records claiming one environment, the stale one holding a
	// dead PID.
	reg := newRegistry(t.TempDir())
	const path = "/home/u/dev/api"

	first := testEnvironment(path, "api")
	first.PID = 100
	if err := reg.Put(first); err != nil {
		t.Fatalf("put: %v", err)
	}

	second := testEnvironment(path, "api")
	second.PID = 200
	if err := reg.Put(second); err != nil {
		t.Fatalf("put: %v", err)
	}

	all, err := reg.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(all), all)
	}
	if all[0].PID != 200 {
		t.Errorf("the newer record should have won, got %+v", all[0])
	}
}

func TestRegistryKeepsDistinctPathsApart(t *testing.T) {
	reg := newRegistry(t.TempDir())
	for _, p := range []struct{ path, name string }{
		{"/home/u/dev/api", "dev-api"},
		{"/home/u/work/api", "work-api"},
	} {
		if err := reg.Put(testEnvironment(p.path, p.name)); err != nil {
			t.Fatalf("put: %v", err)
		}
	}

	all, err := reg.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("got %d records, want 2 — same base name, different paths", len(all))
	}
}

func TestRegistryRemove(t *testing.T) {
	reg := newRegistry(t.TempDir())
	const path = "/home/u/dev/api"
	if err := reg.Put(testEnvironment(path, "api")); err != nil {
		t.Fatalf("put: %v", err)
	}

	removed, err := reg.Remove(path)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed {
		t.Error("removing an existing record should report true")
	}

	// Removing again reports false rather than pretending to succeed, so a
	// caller can tell "stopped it" from "there was nothing to stop".
	removed, err = reg.Remove(path)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed {
		t.Error("removing an absent record should report false")
	}
}

func TestRegistryListAlive(t *testing.T) {
	reg := newRegistry(t.TempDir())
	live := testEnvironment("/home/u/dev/live", "live")
	dead := testEnvironment("/home/u/dev/dead", "dead")
	for _, env := range []Environment{live, dead} {
		if err := reg.Put(env); err != nil {
			t.Fatalf("put: %v", err)
		}
	}

	onlyLive := func(env Environment) bool { return env.ProjectName == "live" }

	got, err := reg.ListAlive(onlyLive)
	if err != nil {
		t.Fatalf("list alive: %v", err)
	}
	if len(got) != 1 || got[0].ProjectName != "live" {
		t.Fatalf("got %+v, want just the live one", got)
	}

	// The dead record is dropped from the file, not merely filtered out of
	// this one answer.
	all, err := reg.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("dead record should have been pruned from disk, file holds %+v", all)
	}
}

func TestRegistryListAliveLeavesFileAloneWhenNothingDied(t *testing.T) {
	stateDir := t.TempDir()
	reg := newRegistry(stateDir)
	if err := reg.Put(testEnvironment("/home/u/dev/api", "api")); err != nil {
		t.Fatalf("put: %v", err)
	}

	path := stateFile(stateDir, "environments.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if _, err := reg.ListAlive(alwaysAlive); err != nil {
		t.Fatalf("list alive: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Error("a read that changed nothing should not rewrite the file")
	}
}

func TestRegistryRenameByPath(t *testing.T) {
	reg := newRegistry(t.TempDir())
	if err := reg.Put(testEnvironment("/home/u/dev/api", "api")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := reg.Put(testEnvironment("/home/u/dev/web", "web")); err != nil {
		t.Fatalf("put: %v", err)
	}

	events, err := reg.RenameByPath(map[string]string{
		"/home/u/dev/api":     "dev-api", // changed
		"/home/u/dev/web":     "web",     // unchanged
		"/home/u/dev/unknown": "ghost",   // no environment here
	})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if events[0].OldName != "api" || events[0].NewName != "dev-api" {
		t.Errorf("got %+v", events[0])
	}

	stored, err := reg.Get("/home/u/dev/api")
	if err != nil || stored == nil {
		t.Fatalf("get: %v / %v", stored, err)
	}
	if stored.ProjectName != "dev-api" {
		t.Errorf("stored name = %q, want dev-api", stored.ProjectName)
	}

	// Renaming is bookkeeping only; nothing about the running process moves.
	if stored.EnvironmentID != "env_api" || stored.PID != 4242 {
		t.Errorf("rename touched process state: %+v", stored)
	}
}

func TestRegistryRenameByPathNoChanges(t *testing.T) {
	reg := newRegistry(t.TempDir())
	if err := reg.Put(testEnvironment("/home/u/dev/api", "api")); err != nil {
		t.Fatalf("put: %v", err)
	}

	events, err := reg.RenameByPath(map[string]string{"/home/u/dev/api": "api"})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %+v, want no events", events)
	}
}

func TestRescanRelabelsRunningEnvironments(t *testing.T) {
	root := t.TempDir()
	apiPath := filepath.Join(root, "dev", "api")
	makeProject(t, apiPath, "go.mod")

	svc, stateDir := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*", "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// Pretend an environment is running for it, labelled as the cache has it.
	env := testEnvironment(apiPath, "api")
	if err := newRegistry(stateDir).Put(env); err != nil {
		t.Fatalf("put: %v", err)
	}

	// A colliding project forces "api" to become "dev-api".
	makeProject(t, filepath.Join(root, "work", "api"), "go.mod")

	result, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(result.RenamedEnvironments) != 1 {
		t.Fatalf("got %+v, want one relabelled environment", result.RenamedEnvironments)
	}
	if got := result.RenamedEnvironments[0]; got.OldName != "api" || got.NewName != "dev-api" {
		t.Errorf("got %+v, want api -> dev-api", got)
	}

	// The stored record must now carry the name the user is shown, or it
	// becomes unfindable by that name.
	stored, err := newRegistry(stateDir).Get(apiPath)
	if err != nil || stored == nil {
		t.Fatalf("get: %v / %v", stored, err)
	}
	if stored.ProjectName != "dev-api" {
		t.Errorf("stored name = %q, want dev-api", stored.ProjectName)
	}
	if stored.EnvironmentID != env.EnvironmentID || stored.PID != env.PID {
		t.Errorf("relabelling disturbed process state: %+v", stored)
	}
}

func TestRegistryReportsMalformedFile(t *testing.T) {
	stateDir := t.TempDir()
	path := stateFile(stateDir, "environments.json")
	mustWrite(t, path, `{"environments":[{"project_path":`)

	_, err := newRegistry(stateDir).List()
	if err == nil {
		t.Fatal("expected an error for a malformed file")
	}
	// The path matters: the user needs to know what to fix or delete.
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not mention %q", err, path)
	}
}
