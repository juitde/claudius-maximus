package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// Config is the user-editable configuration, stored as config.json inside the
// state directory.
type Config struct {
	// ProjectGlobs are shell-style glob patterns pointing at directories that
	// may contain projects, e.g. "~/dev/*" or "~/work/*/repos/*". A leading
	// "~/" is expanded to the user's home directory.
	ProjectGlobs []string `json:"project_globs"`

	// ProjectMarkers replaces the default set of files whose presence makes a
	// directory count as a project.
	//
	// Three states, distinguished by JSON's difference between an absent key
	// and an empty array:
	//
	//	key absent   -> nil        -> defaultProjectMarkers apply
	//	["go.mod"]   -> one entry  -> only that marker applies
	//	[]           -> empty, set -> filtering off, every matched directory counts
	//
	// The last one matters. Markers are curation, not correctness: this tool
	// runs claude in the directory, and claude runs anywhere. Someone whose
	// globs are already precise should be able to say so instead of hunting
	// for a marker that makes their project visible.
	//
	// Note the absent `omitempty`: it would erase exactly the distinction this
	// field depends on, because encoding/json treats an empty slice as empty
	// regardless of whether it is nil. saveConfig drops unset properties
	// itself, which can tell the two apart.
	ProjectMarkers []string `json:"project_markers"`
}

// markers returns the marker set this config actually uses.
func (c *Config) markers() []string {
	if c.ProjectMarkers == nil {
		return defaultProjectMarkers
	}
	return c.ProjectMarkers
}

// Project is a single discovered project directory.
//
// Name is deliberately not filled in here. Assigning names requires seeing
// every discovered project at once in order to resolve collisions, so it
// happens in a separate pass (see naming.go).
type Project struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// defaultProjectMarkers are the files and directories whose presence makes a
// directory look like a development project. A directory needs only one of
// them. Entries may be glob patterns, which is what ecosystems without a fixed
// root filename require (*.tf, *.sln).
//
// This list exists to keep noise out of a broad glob like "~/dev/*": stray
// directories do not just clutter the listing, they inflate everyone's names,
// because each extra entry is another chance to collide and every collision
// makes names longer. Override it via Config.ProjectMarkers.
//
// The aim is one canonical root file per ecosystem, not exhaustive coverage.
// Anything missing is a one-line config change away, whereas a marker that
// fires too eagerly costs every user a longer project name.
var defaultProjectMarkers = []string{
	// Version control, plus this tool's own convention
	".git", ".hg", ".svn", "CLAUDE.md",

	// Go, JavaScript, Rust, PHP
	"go.mod", "package.json", "deno.json", "Cargo.toml", "composer.json",

	// Python
	"pyproject.toml", "setup.py", "requirements.txt", "Pipfile",

	// JVM: Maven, Gradle, sbt, Clojure
	"pom.xml", "build.gradle", "build.gradle.kts",
	"settings.gradle", "settings.gradle.kts",
	"build.sbt", "deps.edn", "project.clj",

	// .NET
	"*.sln", "*.csproj", "*.fsproj",

	// Ruby, Elixir, Erlang
	"Gemfile", "*.gemspec", "mix.exs", "rebar.config",

	// C, C++, and other compiled ecosystems
	"CMakeLists.txt", "meson.build", "configure.ac", "Makefile",
	"Package.swift", "*.xcodeproj", "build.zig",
	"pubspec.yaml", "stack.yaml", "*.cabal",

	// Infrastructure as code
	"*.tf", ".terraform.lock.hcl", "terragrunt.hcl",
	"ansible.cfg", "Chart.yaml", "kustomization.yaml", "flake.nix",

	// Containers and local development environments
	"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yaml",
	".ddev", ".devcontainer",

	// Editor and IDE metadata. A directory someone has opened as a project in
	// an editor is a project, whatever else it does or does not contain —
	// which is the whole point for the ecosystems with no manifest file at
	// all, such as plain PHP or shell tooling.
	".idea",             // JetBrains: IntelliJ, PhpStorm, GoLand, PyCharm, Rider
	".vscode",           // VS Code and its forks
	".vs",               // Visual Studio
	".fleet",            // JetBrains Fleet
	".zed",              // Zed
	".project",          // Eclipse
	"*.sublime-project", // Sublime Text
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// A missing config is a normal first-run state, not an error. The
		// caller gets an empty config and discovers zero projects, which the
		// doctor command reports as a fixable warning.
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Unknown keys are rejected rather than ignored. A misspelled property
	// that silently does nothing is the same failure mode as a silently
	// skipped glob: the user sees no effect and gets no explanation.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w (valid properties: %s)",
			path, err, strings.Join(configPropertyNames(), ", "))
	}
	return &cfg, nil
}

// MarshalJSON writes the configuration with unset properties left out
// entirely.
//
// The struct alone cannot express "absent": encoding/json renders a nil slice
// as null, while `omitempty` collapses an explicitly empty list into the same
// absence. Either would destroy the three-state semantics of project_markers
// on the next read. Building a map sidesteps both, and keeps the stored file
// and the printed output identical — an unset property should not appear as
// null in one place and vanish in the other.
//
// Map keys marshal in sorted order, so the file is stable across writes.
func (c Config) MarshalJSON() ([]byte, error) {
	v := reflect.ValueOf(c)
	t := v.Type()

	out := map[string]any{}
	for i := range t.NumField() {
		name := jsonFieldName(t.Field(i))
		if name == "" {
			continue
		}
		field := v.Field(i)
		if field.Kind() == reflect.Slice && field.IsNil() {
			continue // unset — leave the key out entirely
		}
		out[name] = field.Interface()
	}
	return json.Marshal(out)
}

func saveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

// RejectedDir is something a glob matched that did not become a project.
//
// Reporting these matters because the alternative is a project that quietly
// fails to appear. The list is what makes "why is my project missing?"
// answerable without guessing.
type RejectedDir struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ScanResult is everything a scan of the configured globs learned.
type ScanResult struct {
	Projects []Project     `json:"projects"`
	Rejected []RejectedDir `json:"rejected,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
}

// resolveProjects expands every configured glob and keeps the directories that
// look like projects.
//
// Nothing is discarded in silence. Patterns that cannot be used produce
// warnings, and directories that matched but were filtered out are reported
// individually: a malformed pattern is not fatal, but swallowing it means a
// project never appears with nothing to explain why.
func resolveProjects(cfg *Config) (*ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}

	markers, warnings := validateMarkers(cfg.markers())
	result := &ScanResult{Warnings: warnings}

	seen := map[string]bool{}
	for _, pattern := range cfg.ProjectGlobs {
		expanded := expandHome(pattern, home)
		matches, globErr := filepath.Glob(expanded)
		if globErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("ignoring malformed pattern %q: %v", pattern, globErr))
			continue
		}
		if len(matches) == 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("pattern %q matched nothing", pattern))
			continue
		}

		matchedProject := false
		for _, match := range matches {
			abs, absErr := filepath.Abs(match)
			if absErr != nil || seen[abs] {
				continue
			}
			seen[abs] = true

			info, statErr := os.Stat(match)
			switch {
			case statErr != nil:
				result.Rejected = append(result.Rejected, RejectedDir{abs, "not readable: " + statErr.Error()})
			case !info.IsDir():
				result.Rejected = append(result.Rejected, RejectedDir{abs, "not a directory"})
			case !looksLikeProject(abs, markers):
				result.Rejected = append(result.Rejected, RejectedDir{abs, "no project marker"})
			default:
				matchedProject = true
				result.Projects = append(result.Projects, Project{Path: abs})
			}
		}
		if !matchedProject {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"pattern %q matched directories, but none contained a project marker (%s) — "+
					"set project_markers to [] in the config to accept every matched directory",
				pattern, strings.Join(markers, ", ")))
		}
	}
	return result, nil
}

// looksLikeProject reports whether dir carries at least one of the given
// markers. An empty marker set accepts every directory — the caller's globs
// are then the only filter, which is a legitimate way to configure this.
//
// Markers containing glob metacharacters are matched against the directory's
// entries; the rest are looked up directly. Literal markers are checked first
// because a single stat beats listing a directory that may hold thousands of
// files, and most matches come from a literal.
func looksLikeProject(dir string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}

	var patterns []string
	for _, marker := range markers {
		if isGlobPattern(marker) {
			patterns = append(patterns, marker)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	if len(patterns) == 0 {
		return false
	}

	// Matching against ReadDir entries rather than filepath.Glob on a joined
	// path: the directory name is data and may itself contain glob
	// metacharacters, which Glob would misread as part of the pattern.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		for _, pattern := range patterns {
			if ok, err := filepath.Match(pattern, entry.Name()); ok && err == nil {
				return true
			}
		}
	}
	return false
}

// isGlobPattern reports whether s carries any glob metacharacter, i.e. whether
// it needs matching rather than a direct lookup.
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, `*?[`)
}

// validateMarkers separates usable markers from malformed glob patterns.
//
// A bad pattern is reported rather than silently ignored, for the same reason
// a bad project glob is: it would otherwise make projects vanish with no
// explanation.
func validateMarkers(markers []string) (usable []string, warnings []string) {
	for _, marker := range markers {
		if isGlobPattern(marker) {
			if _, err := filepath.Match(marker, ""); err != nil {
				warnings = append(warnings, fmt.Sprintf("ignoring malformed project marker %q: %v", marker, err))
				continue
			}
		}
		usable = append(usable, marker)
	}
	return usable, warnings
}

// expandHome replaces a leading "~" or "~/" with the user's home directory.
// A "~name" prefix is left untouched: this tool has no business guessing at
// other users' home directories.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if len(path) >= 2 && path[0] == '~' && (path[1] == '/' || path[1] == filepath.Separator) {
		return filepath.Join(home, path[2:])
	}
	return path
}
