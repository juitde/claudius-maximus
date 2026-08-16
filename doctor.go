package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CheckStatus string

const (
	StatusOK   CheckStatus = "ok"
	StatusWarn CheckStatus = "warn"
	StatusFail CheckStatus = "fail"
)

// CheckResult is one diagnostic. Only StatusFail makes doctor exit non-zero;
// a warning is information, not a blocker.
type CheckResult struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Detail  string      `json:"detail"`
	FixHint string      `json:"fix_hint,omitempty"`
}

// DoctorReport is the full diagnosis: the checks, plus what a rescan would do
// if it ran right now.
type DoctorReport struct {
	Version   string        `json:"version"`
	StateDir  string        `json:"state_dir"`
	Checks    []CheckResult `json:"checks"`
	Preflight *Preflight    `json:"preflight,omitempty"`
}

func (r *DoctorReport) failed() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// Doctor inspects the setup and runs a preflight scan.
//
// The preflight deliberately writes nothing. Its whole purpose is to answer
// "what would rescan do?" — a diagnosis that changed the thing being diagnosed
// would be useless for deciding whether to run the real thing.
func (s *Service) Doctor() *DoctorReport {
	report := &DoctorReport{Version: version, StateDir: s.stateDir}

	report.Checks = append(report.Checks,
		s.checkStateDir(),
		s.checkConfig(),
	)

	cfg, cfgErr := loadConfig(s.configPath)
	if cfgErr == nil {
		report.Checks = append(report.Checks,
			checkGlobs(cfg),
			checkMarkers(cfg),
		)
	}

	report.Checks = append(report.Checks, s.checkCache())

	// A broken config makes the scan meaningless, so skip it rather than
	// reporting a scan of nothing as if it were a real result.
	if cfgErr != nil {
		return report
	}
	preflight, err := s.Preflight()
	if err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name: "preflight", Status: StatusFail,
			Detail: "scan failed: " + err.Error(),
		})
		return report
	}
	report.Preflight = preflight
	report.Checks = append(report.Checks, checkFreshness(preflight))
	return report
}

func (s *Service) checkStateDir() CheckResult {
	probe := filepath.Join(s.stateDir, ".doctor-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return CheckResult{
			Name: "state directory", Status: StatusFail,
			Detail:  shortenPath(s.stateDir) + " is not writable: " + err.Error(),
			FixHint: "check permissions, or set " + envHome + " to a writable directory",
		}
	}
	os.Remove(probe)
	return CheckResult{Name: "state directory", Status: StatusOK, Detail: shortenPath(s.stateDir)}
}

func (s *Service) checkConfig() CheckResult {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return CheckResult{
			Name: "configuration", Status: StatusWarn,
			Detail:  "not created yet",
			FixHint: fmt.Sprintf("%s config add project_globs '~/dev/*'", appName),
		}
	}
	if _, err := loadConfig(s.configPath); err != nil {
		return CheckResult{
			Name: "configuration", Status: StatusFail,
			Detail:  err.Error(),
			FixHint: fmt.Sprintf("fix the file by hand, or see '%s config schema'", appName),
		}
	}
	return CheckResult{Name: "configuration", Status: StatusOK, Detail: shortenPath(s.configPath)}
}

func checkGlobs(cfg *Config) CheckResult {
	if len(cfg.ProjectGlobs) == 0 {
		return CheckResult{
			Name: "project globs", Status: StatusWarn,
			Detail:  "none configured, so no project can be found",
			FixHint: fmt.Sprintf("%s config add project_globs '~/dev/*'", appName),
		}
	}
	return CheckResult{
		Name: "project globs", Status: StatusOK,
		Detail: fmt.Sprintf("%s configured: %s",
			plural(len(cfg.ProjectGlobs), "pattern"), strings.Join(cfg.ProjectGlobs, ", ")),
	}
}

func checkMarkers(cfg *Config) CheckResult {
	_, warnings := validateMarkers(cfg.markers())
	if len(warnings) > 0 {
		return CheckResult{
			Name: "project markers", Status: StatusWarn,
			Detail:  strings.Join(warnings, "; "),
			FixHint: fmt.Sprintf("%s config remove project_markers <value>", appName),
		}
	}

	switch {
	case cfg.ProjectMarkers == nil:
		return CheckResult{
			Name: "project markers", Status: StatusOK,
			Detail: fmt.Sprintf("built-in defaults (%d entries)", len(defaultProjectMarkers)),
		}
	case len(cfg.ProjectMarkers) == 0:
		return CheckResult{
			Name: "project markers", Status: StatusOK,
			Detail: "filtering disabled — every matched directory counts as a project",
		}
	default:
		return CheckResult{
			Name: "project markers", Status: StatusOK,
			Detail: fmt.Sprintf("custom: %s", strings.Join(cfg.ProjectMarkers, ", ")),
		}
	}
}

func (s *Service) checkCache() CheckResult {
	cache, err := s.ListProjects()
	if err != nil {
		return CheckResult{
			Name: "project cache", Status: StatusFail,
			Detail:  err.Error(),
			FixHint: fmt.Sprintf("%s rescan", appName),
		}
	}
	if cache.ScannedAt.IsZero() {
		return CheckResult{
			Name: "project cache", Status: StatusWarn,
			Detail:  "no scan has run yet",
			FixHint: fmt.Sprintf("%s rescan", appName),
		}
	}
	return CheckResult{
		Name: "project cache", Status: StatusOK,
		Detail: fmt.Sprintf("%s, scanned %s",
			plural(len(cache.Projects), "project"), humanizeSince(cache.ScannedAt)),
	}
}

func checkFreshness(p *Preflight) CheckResult {
	if !p.Stale() {
		return CheckResult{Name: "cache freshness", Status: StatusOK, Detail: "up to date"}
	}

	var parts []string
	if n := len(p.Added); n > 0 {
		parts = append(parts, fmt.Sprintf("%d to add", n))
	}
	if n := len(p.Removed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d to remove", n))
	}
	if n := len(p.Renamed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d to rename", n))
	}
	return CheckResult{
		Name: "cache freshness", Status: StatusWarn,
		Detail:  "out of date — " + strings.Join(parts, ", "),
		FixHint: fmt.Sprintf("%s rescan", appName),
	}
}

// --- rendering ---

func printDoctorHuman(r *DoctorReport) {
	symbols := map[CheckStatus]string{StatusOK: "✓", StatusWarn: "!", StatusFail: "x"}

	fmt.Printf("%s %s\n\n", appName, r.Version)
	for _, c := range r.Checks {
		fmt.Printf("  %s %-18s %s\n", symbols[c.Status], c.Name, c.Detail)
		if c.FixHint != "" {
			fmt.Printf("    %-18s -> %s\n", "", c.FixHint)
		}
	}

	if r.Preflight == nil {
		return
	}
	p := r.Preflight

	fmt.Printf("\nPreflight scan (nothing written)\n\n")
	fmt.Printf("  %s would be cached\n", plural(len(p.Scan.Projects), "project"))

	printProjectGroup("would be added", p.Added)
	printProjectGroup("would be removed", p.Removed)

	if len(p.Renamed) > 0 {
		width := 0
		for _, r := range p.Renamed {
			width = max(width, len(r.OldName))
		}
		fmt.Printf("\n  would be renamed:\n")
		for _, r := range p.Renamed {
			fmt.Printf("    %-*s -> %-*s  %s\n", width, r.OldName, width, r.NewName, shortenPath(r.Path))
		}
	}

	if len(p.Scan.Rejected) > 0 {
		fmt.Printf("\n  matched a glob but skipped:\n")
		width := 0
		for _, d := range p.Scan.Rejected {
			width = max(width, len(shortenPath(d.Path)))
		}
		for _, d := range p.Scan.Rejected {
			fmt.Printf("    %-*s  %s\n", width, shortenPath(d.Path), d.Reason)
		}
	}

	if len(p.Scan.Warnings) > 0 {
		fmt.Printf("\n  warnings:\n")
		for _, w := range p.Scan.Warnings {
			fmt.Printf("    %s\n", w)
		}
	}
}

func printProjectGroup(label string, projects []Project) {
	if len(projects) == 0 {
		return
	}
	width := 0
	for _, p := range projects {
		width = max(width, len(p.Name))
	}
	fmt.Printf("\n  %s:\n", label)
	for _, p := range projects {
		fmt.Printf("    %-*s  %s\n", width, p.Name, shortenPath(p.Path))
	}
}

// --- formatting helpers ---

// shortenPath renders a path relative to the home directory, since a wall of
// identical prefixes hides the part that differs.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	default:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
