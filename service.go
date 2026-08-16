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
	// Rejected lists directories a glob matched that did not qualify.
	Rejected []RejectedDir `json:"rejected,omitempty"`
	// Warnings describes configured patterns that could not be used. Never
	// fatal, but always surfaced — see resolveProjects.
	Warnings []string `json:"warnings,omitempty"`
}

// Rescan expands the configured globs, assigns collision-free names and
// replaces the cache.
func (s *Service) Rescan() (*RescanResult, error) {
	scan, err := s.scan()
	if err != nil {
		return nil, err
	}

	cache := &ProjectCache{ScannedAt: time.Now(), Projects: scan.Projects}
	if err := saveProjectCache(s.cachePath, cache); err != nil {
		return nil, fmt.Errorf("write cache: %w", err)
	}

	return &RescanResult{Cache: *cache, Rejected: scan.Rejected, Warnings: scan.Warnings}, nil
}

// scan performs the discovery half of a rescan — everything except writing the
// cache. Both Rescan and the doctor's preflight go through it, so what the
// preflight reports is by construction what a rescan would do, rather than a
// second implementation that resembles it.
func (s *Service) scan() (*ScanResult, error) {
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	scan, err := resolveProjects(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve globs: %w", err)
	}

	// Naming has to happen here rather than inside resolveProjects: resolving
	// collisions requires seeing every discovered project at once.
	assignProjectNames(scan.Projects)
	sort.Slice(scan.Projects, func(i, j int) bool { return scan.Projects[i].Name < scan.Projects[j].Name })

	return scan, nil
}

// Preflight reports what a rescan would do, without touching the cache.
type Preflight struct {
	Scan ScanResult `json:"scan"`
	// Added, Removed and Renamed describe the cache's drift from reality.
	Added   []Project        `json:"added,omitempty"`
	Removed []Project        `json:"removed,omitempty"`
	Renamed []RenamedProject `json:"renamed,omitempty"`
	// CachedAt is the timestamp of the cache being compared against; zero if
	// no rescan has run yet.
	CachedAt time.Time `json:"cached_at"`
}

// RenamedProject is a project whose generated name would change, which happens
// when a newly discovered project collides with an existing one.
type RenamedProject struct {
	Path    string `json:"path"`
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

// Stale reports whether a rescan would change anything.
func (p *Preflight) Stale() bool {
	return len(p.Added) > 0 || len(p.Removed) > 0 || len(p.Renamed) > 0
}

func (s *Service) Preflight() (*Preflight, error) {
	scan, err := s.scan()
	if err != nil {
		return nil, err
	}
	cache, err := s.ListProjects()
	if err != nil {
		return nil, err
	}

	cached := make(map[string]Project, len(cache.Projects))
	for _, p := range cache.Projects {
		cached[p.Path] = p
	}

	out := &Preflight{Scan: *scan, CachedAt: cache.ScannedAt}
	found := make(map[string]bool, len(scan.Projects))
	for _, p := range scan.Projects {
		found[p.Path] = true
		switch previous, known := cached[p.Path]; {
		case !known:
			out.Added = append(out.Added, p)
		case previous.Name != p.Name:
			out.Renamed = append(out.Renamed, RenamedProject{p.Path, previous.Name, p.Name})
		}
	}
	for _, p := range cache.Projects {
		if !found[p.Path] {
			out.Removed = append(out.Removed, p)
		}
	}
	return out, nil
}
