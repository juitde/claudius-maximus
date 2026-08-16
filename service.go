package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// Service holds all of this tool's behaviour.
//
// Both entry points — the CLI and, later, the MCP server — call nothing but
// methods on this type. Neither of them makes decisions of its own: no
// validation, no error special-casing, no business rules. That is what makes
// the two interfaces behave identically rather than merely similarly; they are
// not two implementations kept in sync, they are one implementation called two
// ways.
type Service struct {
	stateDir   string
	configPath string
	cachePath  string
}

func newService(stateDir string) *Service {
	return &Service{
		stateDir:   stateDir,
		configPath: filepath.Join(stateDir, "config.json"),
		cachePath:  filepath.Join(stateDir, "projects.json"),
	}
}

// ListProjects returns the cached project list. It never scans the filesystem;
// see ProjectCache for why.
func (s *Service) ListProjects() (*ProjectCache, error) {
	return loadProjectCache(s.cachePath)
}

// --- configuration ---

// ShowConfig returns the configuration as stored.
func (s *Service) ShowConfig() (*Config, error) {
	return loadConfig(s.configPath)
}

// ConfigSchema lists the editable properties.
func (s *Service) ConfigSchema() []PropertySpec {
	return configSchema()
}

// ConfigEditResult reports the outcome of an edit.
type ConfigEditResult struct {
	Config Config `json:"config"`
	// Notes covers adjustments the caller did not explicitly ask for, such as
	// an unset property being seeded from its defaults or a duplicate value
	// being skipped.
	Notes []string `json:"notes,omitempty"`
}

// EditConfig applies one edit and persists the result.
//
// A single entry point for every operation rather than one method per verb:
// validation, the read-modify-write cycle and the persistence rules then exist
// exactly once, which is what keeps a later MCP tool from drifting away from
// the CLI.
func (s *Service) EditConfig(edit ConfigEdit) (*ConfigEditResult, error) {
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	notes, err := applyConfigEdit(cfg, edit)
	if err != nil {
		return nil, err
	}

	if err := saveConfig(s.configPath, cfg); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	return &ConfigEditResult{Config: *cfg, Notes: notes}, nil
}

// RescanResult reports what a rescan found.
type RescanResult struct {
	Cache ProjectCache `json:"cache"`
	// Warnings describes configured patterns that could not be used. Never
	// fatal, but always surfaced — see resolveProjects.
	Warnings []string `json:"warnings,omitempty"`
}

// Rescan expands the configured globs, assigns collision-free names and
// replaces the cache.
func (s *Service) Rescan() (*RescanResult, error) {
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	projects, warnings, err := resolveProjects(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve globs: %w", err)
	}

	// Naming has to happen here rather than inside resolveProjects: resolving
	// collisions requires seeing every discovered project at once.
	assignProjectNames(projects)
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })

	cache := &ProjectCache{ScannedAt: time.Now(), Projects: projects}
	if err := saveProjectCache(s.cachePath, cache); err != nil {
		return nil, fmt.Errorf("write cache: %w", err)
	}

	return &RescanResult{Cache: *cache, Warnings: warnings}, nil
}
