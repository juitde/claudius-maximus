package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func findCheck(t *testing.T, report *DoctorReport, name string) CheckResult {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in report", name)
	return CheckResult{}
}

func TestPreflightLeavesTheCacheUntouched(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, stateDir := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	cachePath := stateFile(stateDir, "projects.json")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}

	// A project appears; the preflight must see it but must not record it.
	makeProject(t, filepath.Join(root, "web"), "package.json")

	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if len(preflight.Added) != 1 || preflight.Added[0].Name != "web" {
		t.Errorf("expected 'web' to be reported as added, got %v", preflight.Added)
	}

	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(before) != string(after) {
		t.Error("preflight modified the cache; it must be read-only")
	}

	// And the cache really is still the old view.
	cache, _ := svc.ListProjects()
	if len(cache.Projects) != 1 {
		t.Errorf("cache should still hold 1 project, got %v", cache.Projects)
	}
}

func TestPreflightReportsDrift(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")
	makeProject(t, filepath.Join(root, "gone"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	makeProject(t, filepath.Join(root, "fresh"), "go.mod")

	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !preflight.Stale() {
		t.Error("expected the cache to be reported as stale")
	}
	if len(preflight.Added) != 1 || preflight.Added[0].Name != "fresh" {
		t.Errorf("added = %v, want [fresh]", preflight.Added)
	}
	if len(preflight.Removed) != 1 || preflight.Removed[0].Name != "gone" {
		t.Errorf("removed = %v, want [gone]", preflight.Removed)
	}
}

func TestPreflightReportsRenames(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "dev", "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*", "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// A second "api" forces both to grow a parent segment, so the cached name
	// of the first one no longer matches what a rescan would produce.
	makeProject(t, filepath.Join(root, "work", "api"), "go.mod")

	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if len(preflight.Renamed) != 1 {
		t.Fatalf("renamed = %v, want exactly one", preflight.Renamed)
	}
	got := preflight.Renamed[0]
	if got.OldName != "api" || got.NewName != "dev-api" {
		t.Errorf("got %s -> %s, want api -> dev-api", got.OldName, got.NewName)
	}
	if !preflight.Stale() {
		t.Error("a pending rename must count as stale")
	}
}

func TestPreflightIsNotStaleWhenUpToDate(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if preflight.Stale() {
		t.Errorf("expected no drift, got added=%v removed=%v renamed=%v",
			preflight.Added, preflight.Removed, preflight.Renamed)
	}
}

func TestScanReportsRejectedDirectories(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")
	mustMkdir(t, filepath.Join(root, "notes"))
	mustWrite(t, filepath.Join(root, "loose.txt"), "")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}

	reasons := map[string]string{}
	for _, r := range preflight.Scan.Rejected {
		reasons[filepath.Base(r.Path)] = r.Reason
	}
	if reasons["notes"] != "no project marker" {
		t.Errorf("notes: got %q, want 'no project marker'", reasons["notes"])
	}
	if reasons["loose.txt"] != "not a directory" {
		t.Errorf("loose.txt: got %q, want 'not a directory'", reasons["loose.txt"])
	}
	if _, rejected := reasons["api"]; rejected {
		t.Error("a real project must not be listed as rejected")
	}
}

func TestDoctorChecks(t *testing.T) {
	t.Run("healthy setup", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, filepath.Join(root, "api"), "go.mod")

		svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
		if _, err := svc.Rescan(); err != nil {
			t.Fatalf("rescan: %v", err)
		}

		report := svc.Doctor()
		if report.failed() {
			t.Errorf("expected no failures, got %+v", report.Checks)
		}
		if got := findCheck(t, report, "cache freshness"); got.Status != StatusOK {
			t.Errorf("freshness = %v (%s), want ok", got.Status, got.Detail)
		}
		if report.Preflight == nil {
			t.Error("a healthy report must include the preflight")
		}
	})

	t.Run("no configuration yet", func(t *testing.T) {
		report := newService(t.TempDir()).Doctor()

		if report.failed() {
			t.Errorf("a missing config is a warning, not a failure: %+v", report.Checks)
		}
		if got := findCheck(t, report, "configuration"); got.Status != StatusWarn {
			t.Errorf("configuration = %v, want warn", got.Status)
		}
		if got := findCheck(t, report, "project cache"); got.Status != StatusWarn {
			t.Errorf("project cache = %v, want warn", got.Status)
		}
	})

	t.Run("malformed configuration fails and skips the scan", func(t *testing.T) {
		stateDir := t.TempDir()
		mustWrite(t, configFile(stateDir), `{"project_globs":`)

		report := newService(stateDir).Doctor()
		if !report.failed() {
			t.Error("a malformed config must be a failure")
		}
		// Scanning with a config that could not be read would report an empty
		// world as though it were real.
		if report.Preflight != nil {
			t.Error("the preflight must be skipped when the config is unreadable")
		}
	})

	t.Run("stale cache is a warning", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, filepath.Join(root, "api"), "go.mod")

		svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
		if _, err := svc.Rescan(); err != nil {
			t.Fatalf("rescan: %v", err)
		}
		makeProject(t, filepath.Join(root, "web"), "package.json")

		report := svc.Doctor()
		got := findCheck(t, report, "cache freshness")
		if got.Status != StatusWarn {
			t.Errorf("freshness = %v, want warn", got.Status)
		}
		if report.failed() {
			t.Error("a stale cache must not be a hard failure")
		}
	})

	t.Run("marker configuration is described", func(t *testing.T) {
		tests := []struct {
			name    string
			markers []string
			want    string
		}{
			{"defaults", nil, "built-in defaults"},
			{"disabled", []string{}, "filtering disabled"},
			{"custom", []string{"go.mod"}, "custom"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, _ := newTestService(t, Config{
					ProjectGlobs:   []string{filepath.Join(t.TempDir(), "*")},
					ProjectMarkers: tt.markers,
				})
				got := findCheck(t, svc.Doctor(), "project markers")
				if got.Status != StatusOK {
					t.Errorf("status = %v, want ok", got.Status)
				}
				if !strings.Contains(got.Detail, tt.want) {
					t.Errorf("detail %q should mention %q", got.Detail, tt.want)
				}
			})
		}
	})
}

func TestShortenPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available")
	}

	tests := []struct {
		in   string
		want string
	}{
		{home, "~"},
		{filepath.Join(home, "dev", "api"), filepath.Join("~", "dev", "api")},
		{"/opt/src/api", "/opt/src/api"},
		// A sibling directory whose name merely starts with the home path must
		// not be rewritten.
		{home + "-backup/api", home + "-backup/api"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := shortenPath(tt.in); got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHumanizeSince(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"seconds", 10 * time.Second, "just now"},
		{"one minute", 90 * time.Second, "1 minute ago"},
		{"minutes", 5 * time.Minute, "5 minutes ago"},
		{"one hour", 90 * time.Minute, "1 hour ago"},
		{"hours", 5 * time.Hour, "5 hours ago"},
		{"one day", 30 * time.Hour, "1 day ago"},
		{"days", 72 * time.Hour, "3 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizeSince(time.Now().Add(-tt.ago)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 projects"},
		{1, "1 project"},
		{2, "2 projects"},
	}
	for _, tt := range tests {
		if got := plural(tt.n, "project"); got != tt.want {
			t.Errorf("plural(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want string // "<", ">" or "="
	}{
		{"2.1.51", "2.1.51", "="},
		{"2.1.50", "2.1.51", "<"},
		{"2.1.233", "2.1.51", ">"}, // not a string comparison: 233 > 51
		{"1.9.99", "2.0.0", "<"},
		{"3.0.0", "2.9.9", ">"},
		{"2.1", "2.1.0", "<"}, // fewer components sort first
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			sign := "="
			if got < 0 {
				sign = "<"
			} else if got > 0 {
				sign = ">"
			}
			if sign != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d (%s), want %s", tt.a, tt.b, got, sign, tt.want)
			}
		})
	}
}

func TestCheckAuthBlockers(t *testing.T) {
	t.Run("nothing set", func(t *testing.T) {
		for _, name := range authBlockers {
			t.Setenv(name, "")
		}
		if got := checkAuthBlockers(); got.Status != StatusOK {
			t.Errorf("status = %v, want ok", got.Status)
		}
	})

	t.Run("a blocker warns rather than fails", func(t *testing.T) {
		// A warning, not a failure: the claim is documented rather than
		// verified here, and refusing to run on it would be worse than saying
		// so and letting the attempt speak for itself.
		for _, name := range authBlockers {
			t.Setenv(name, "")
		}
		t.Setenv(authBlockers[0], "token-value")

		got := checkAuthBlockers()
		if got.Status != StatusWarn {
			t.Errorf("status = %v, want warn", got.Status)
		}
		if !strings.Contains(got.Detail, authBlockers[0]) {
			t.Errorf("detail should name the variable: %q", got.Detail)
		}
	})
}

func TestCheckClaudeBinaryMissing(t *testing.T) {
	svc := newService(t.TempDir())
	svc.claudeBin = filepath.Join(t.TempDir(), "definitely-not-here")

	got := svc.checkClaudeBinary()
	if got.Status != StatusFail {
		t.Errorf("status = %v, want fail — nothing works without it", got.Status)
	}
	if got.FixHint == "" {
		t.Error("a failure should say what to do about it")
	}
}
