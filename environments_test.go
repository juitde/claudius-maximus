package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitForExit gives a signalled process a moment to actually go away, since
// termination is asynchronous.
func waitForExit(pid int) bool {
	for range 40 {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// environmentFixture builds a service over one discoverable project, with the
// stub standing in for claude.
func environmentFixture(t *testing.T, environmentID string) (svc *Service, projectPath string) {
	t.Helper()

	root := t.TempDir()
	projectPath = filepath.Join(root, "api")
	makeProject(t, projectPath, "go.mod")

	stubEnv(t, map[string]string{
		"CLAUDESTUB_BANNER": "https://claude.ai/code?environment=" + environmentID + "\n",
	})
	t.Setenv(envClaudeBin, claudeStub(t))

	svc, _ = newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	svc.selfBinary = builtSelf(t)
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	return svc, projectPath
}

// stopAll cleans up whatever a test left running.
func stopAll(t *testing.T, svc *Service) {
	t.Helper()
	environments, err := svc.registry.List()
	if err != nil {
		return
	}
	for _, env := range environments {
		terminateProcess(env.PID)
	}
}

func TestStartEnvironment(t *testing.T) {
	svc, projectPath := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	result, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.AlreadyRunning {
		t.Error("a first start is not an existing environment")
	}

	env := result.Environment
	if env.EnvironmentID != "env_alpha" {
		t.Errorf("environment id = %q", env.EnvironmentID)
	}
	if env.ProjectPath != projectPath || env.ProjectName != "api" {
		t.Errorf("got %+v", env)
	}
	if env.SpawnMode != defaultSpawnMode {
		t.Errorf("spawn mode = %q, want the configured default %q", env.SpawnMode, defaultSpawnMode)
	}
	if !processAlive(env.PID) {
		t.Error("the process should be running")
	}

	// It has to be findable afterwards, from a fresh view of the state.
	stored, err := svc.registry.Get(projectPath)
	if err != nil || stored == nil {
		t.Fatalf("get: %v / %v", stored, err)
	}
	if stored.URL != env.URL {
		t.Errorf("stored URL = %q, want %q", stored.URL, env.URL)
	}
}

func TestStartEnvironmentTwiceReconnects(t *testing.T) {
	// claude keeps one environment per directory, so a second start is a
	// reasonable request with an honest answer: the one that already exists.
	svc, _ := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	first, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}

	second, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}})
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if !second.AlreadyRunning {
		t.Error("the second start should report the existing environment")
	}
	if second.Environment.PID != first.Environment.PID {
		t.Errorf("a second process was spawned: %d then %d", first.Environment.PID, second.Environment.PID)
	}

	all, err := svc.registry.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("got %d records, want 1: %+v", len(all), all)
	}
}

func TestStartEnvironmentReplacesADeadRecord(t *testing.T) {
	svc, projectPath := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	// A record left behind by a reboot or an outside kill.
	stale := testEnvironment(projectPath, "api")
	stale.PID = 999999
	if err := svc.registry.Put(stale); err != nil {
		t.Fatalf("put: %v", err)
	}

	result, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.AlreadyRunning {
		t.Error("a dead record must not be reported as a running environment")
	}
	if result.Environment.PID == stale.PID {
		t.Error("the stale pid survived")
	}
}

func TestStartEnvironmentSpawnModeOverride(t *testing.T) {
	svc, _ := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	result, err := svc.StartEnvironment(StartArgs{
		Target:    ProjectTarget{Name: "api"},
		SpawnMode: SpawnWorktree,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.Environment.SpawnMode != SpawnWorktree {
		t.Errorf("spawn mode = %q, want the override", result.Environment.SpawnMode)
	}
}

func TestStartEnvironmentRejectsAnInvalidSpawnMode(t *testing.T) {
	svc, _ := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	_, err := svc.StartEnvironment(StartArgs{
		Target:    ProjectTarget{Name: "api"},
		SpawnMode: SpawnMode("sideways"),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	// Caught before spawning, not after.
	all, _ := svc.registry.List()
	if len(all) != 0 {
		t.Errorf("nothing should have been started: %+v", all)
	}
}

func TestStartEnvironmentByPath(t *testing.T) {
	svc, projectPath := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	result, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Path: projectPath}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Started by path, but the cached name still applies — otherwise the same
	// environment would answer to two different names.
	if result.Environment.ProjectName != "api" {
		t.Errorf("project name = %q, want the cached name", result.Environment.ProjectName)
	}
}

func TestStartEnvironmentAcceptsAPathOutsideTheCache(t *testing.T) {
	// Markers are curation, not correctness, so naming a directory outright
	// has to keep working even when discovery would not have found it.
	svc, _ := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	outside := t.TempDir() // no marker, never scanned

	result, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Path: outside}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.Environment.ProjectPath == "" {
		t.Error("expected the directory to be used as given")
	}
}

func TestStopEnvironment(t *testing.T) {
	svc, projectPath := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	started, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	result, err := svc.StopEnvironment(ProjectTarget{Name: "api"})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if result.Environment.PID != started.Environment.PID {
		t.Errorf("stopped the wrong environment: %+v", result.Environment)
	}

	stored, err := svc.registry.Get(projectPath)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored != nil {
		t.Errorf("the record should be gone, found %+v", stored)
	}
	if !waitForExit(started.Environment.PID) {
		t.Error("the process is still running")
	}
}

func TestStopEnvironmentWithNoneRunning(t *testing.T) {
	svc, _ := environmentFixture(t, "env_alpha")

	_, err := svc.StopEnvironment(ProjectTarget{Name: "api"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no environment running") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestStopEnvironmentAlreadyDead(t *testing.T) {
	// Reported rather than waved through: a silent success would hide that
	// something other than this tool ended it.
	svc, projectPath := environmentFixture(t, "env_alpha")

	stale := testEnvironment(projectPath, "api")
	stale.PID = 999999
	if err := svc.registry.Put(stale); err != nil {
		t.Fatalf("put: %v", err)
	}

	_, err := svc.StopEnvironment(ProjectTarget{Name: "api"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "already stopped") {
		t.Errorf("unhelpful error: %v", err)
	}
	// The stale record is cleared anyway, so the next start works.
	stored, _ := svc.registry.Get(projectPath)
	if stored != nil {
		t.Errorf("the stale record should have been cleared, found %+v", stored)
	}
}

func TestListEnvironments(t *testing.T) {
	svc, projectPath := environmentFixture(t, "env_alpha")
	defer stopAll(t, svc)

	empty, err := svc.ListEnvironments()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("got %+v, want nothing", empty)
	}

	if _, err := svc.StartEnvironment(StartArgs{Target: ProjectTarget{Name: "api"}}); err != nil {
		t.Fatalf("start: %v", err)
	}

	// A dead record from elsewhere must not appear.
	ghost := testEnvironment(filepath.Join(projectPath, "..", "ghost"), "ghost")
	ghost.PID = 999999
	if err := svc.registry.Put(ghost); err != nil {
		t.Fatalf("put: %v", err)
	}

	running, err := svc.ListEnvironments()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(running) != 1 || running[0].ProjectName != "api" {
		t.Errorf("got %+v, want only the live one", running)
	}
}

func TestProjectTargetResolve(t *testing.T) {
	dir := t.TempDir()
	cache := &ProjectCache{Projects: []Project{{Name: "api", Path: dir}}}

	t.Run("both given", func(t *testing.T) {
		if _, _, err := (ProjectTarget{Name: "api", Path: dir}).resolve(cache); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("neither given", func(t *testing.T) {
		if _, _, err := (ProjectTarget{}).resolve(cache); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("unknown name points at the way out", func(t *testing.T) {
		_, _, err := (ProjectTarget{Name: "nope"}).resolve(cache)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "rescan") {
			t.Errorf("the error should say what to do: %v", err)
		}
	})

	t.Run("a file is not a project directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "notadir")
		mustWrite(t, file, "")
		if _, _, err := (ProjectTarget{Path: file}).resolve(cache); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("a missing directory is reported", func(t *testing.T) {
		if _, _, err := (ProjectTarget{Path: filepath.Join(t.TempDir(), "absent")}).resolve(cache); err == nil {
			t.Error("expected an error")
		}
	})
}
