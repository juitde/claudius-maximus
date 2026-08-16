package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// isEnvironmentAlive reports whether an environment's process is still there.
func isEnvironmentAlive(env Environment) bool {
	return processAlive(env.PID)
}

// stopEnvironment ends an environment's process.
func stopEnvironment(env Environment) error {
	return terminateProcess(env.PID)
}

// ProjectTarget names a project, by cached name or by path.
type ProjectTarget struct {
	Name string
	Path string
}

// resolve turns a target into a directory and the name to record for it.
//
// Exactly one of the two must be given. Validating here rather than in each
// caller is what makes the CLI and a later MCP tool produce the same error for
// the same mistake, rather than two similar ones.
func (t ProjectTarget) resolve(cache *ProjectCache) (path, name string, err error) {
	switch {
	case t.Name != "" && t.Path != "":
		return "", "", fmt.Errorf("give either a project name or a path, not both")
	case t.Name == "" && t.Path == "":
		return "", "", fmt.Errorf("a project name or a path is required")
	}

	if t.Name != "" {
		for _, p := range cache.Projects {
			if p.Name == t.Name {
				return p.Path, p.Name, nil
			}
		}
		return "", "", fmt.Errorf("no project named %q — run 'rescan', or check 'list-projects'", t.Name)
	}

	// A path is taken at face value. Requiring it to be in the cache would add
	// a second gate behind the marker filter, and the whole point of that
	// filter being curation is that naming a directory outright still works.
	abs, err := filepath.Abs(t.Path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("cannot use %s: %w", abs, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a directory", abs)
	}

	// Prefer the cached name when there is one, so a project started by path
	// still appears under the name everything else calls it.
	for _, p := range cache.Projects {
		if p.Path == abs {
			return abs, p.Name, nil
		}
	}
	return abs, sanitizeSegment(filepath.Base(abs)), nil
}

// StartArgs asks for an environment on a project.
type StartArgs struct {
	Target ProjectTarget
	// SpawnMode overrides the configured default for this one start.
	SpawnMode SpawnMode
}

// StartResult reports the environment now serving a project.
type StartResult struct {
	Environment Environment `json:"environment"`
	// AlreadyRunning distinguishes "started one" from "there was one". Not an
	// error: claude keeps one environment per directory and reconnects to it,
	// so asking twice is a reasonable thing to do and the honest answer is the
	// environment that exists.
	AlreadyRunning bool `json:"already_running"`
}

func (s *Service) StartEnvironment(args StartArgs) (*StartResult, error) {
	cache, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	projectPath, projectName, err := args.Target.resolve(cache)
	if err != nil {
		return nil, err
	}

	existing, err := s.registry.Get(projectPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if isEnvironmentAlive(*existing) {
			return &StartResult{Environment: *existing, AlreadyRunning: true}, nil
		}
		// The record outlived its process — a reboot, or a kill from outside.
		// Drop it and start cleanly rather than reporting a dead environment.
		if _, err := s.registry.Remove(projectPath); err != nil {
			return nil, err
		}
	}

	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	spawnMode := args.SpawnMode
	if spawnMode == "" {
		spawnMode = cfg.spawnMode()
	}
	if err := validateSpawnMode(string(spawnMode)); err != nil {
		return nil, fmt.Errorf("invalid spawn mode %q: %w", spawnMode, err)
	}

	logPath := logFileFor(s.stateDir, projectPath)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	outcome, err := spawnPlain(spawnSpec{
		ProjectPath: projectPath,
		ClaudeBin:   s.claudeBin,
		LogPath:     logPath,
		SpawnMode:   spawnMode,
	})
	if err != nil {
		return nil, err
	}

	env := Environment{
		ProjectPath:   projectPath,
		ProjectName:   projectName,
		EnvironmentID: outcome.EnvironmentID,
		URL:           outcome.URL,
		PID:           outcome.PID,
		StartedAt:     time.Now(),
		LogFile:       logPath,
		SpawnMode:     spawnMode,
	}
	if err := s.registry.Put(env); err != nil {
		return nil, err
	}
	return &StartResult{Environment: env}, nil
}

// ListEnvironments returns the environments still running, sorted by project
// name so repeated calls read the same way.
func (s *Service) ListEnvironments() ([]Environment, error) {
	environments, err := s.registry.ListAlive(isEnvironmentAlive)
	if err != nil {
		return nil, err
	}
	sort.Slice(environments, func(i, j int) bool {
		return environments[i].ProjectName < environments[j].ProjectName
	})
	return environments, nil
}

// StopResult reports what was stopped.
type StopResult struct {
	Environment Environment `json:"environment"`
}

func (s *Service) StopEnvironment(target ProjectTarget) (*StopResult, error) {
	cache, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	projectPath, _, err := target.resolve(cache)
	if err != nil {
		return nil, err
	}

	env, err := s.registry.Get(projectPath)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, fmt.Errorf("no environment running for %s", shortenPath(projectPath))
	}

	// An already-dead environment is reported rather than waved through. A
	// silent success would hide the interesting part: something ended it that
	// was not this tool.
	if !isEnvironmentAlive(*env) {
		if _, err := s.registry.Remove(projectPath); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the environment for %s had already stopped; its record has been cleared",
			shortenPath(projectPath))
	}

	if err := stopEnvironment(*env); err != nil {
		return nil, fmt.Errorf("stop environment: %w", err)
	}
	if _, err := s.registry.Remove(projectPath); err != nil {
		return nil, err
	}
	return &StopResult{Environment: *env}, nil
}
