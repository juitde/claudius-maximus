package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeChanges(t *testing.T) {
	tests := []struct {
		name                               string
		added, removed, renamed, unchanged int
		want                               string
	}{
		{"nothing at all", 0, 0, 0, 0, "no projects found"},
		{"nothing changed", 0, 0, 0, 42, "42 projects unmodified"},
		{"first run", 43, 0, 0, 0, "43 projects added"},
		{"one category plus unmodified", 2, 0, 0, 39, "2 projects added, 39 projects unmodified"},
		{"everything moved", 1, 27, 1, 15, "1 project added, 27 projects removed, 1 project renamed, 15 projects unmodified"},
		{"removal only", 0, 3, 0, 10, "3 projects removed, 10 projects unmodified"},
		{"rename only", 0, 0, 2, 40, "2 projects renamed, 40 projects unmodified"},
		{"singular everywhere", 1, 1, 1, 1, "1 project added, 1 project removed, 1 project renamed, 1 project unmodified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeChanges(tt.added, tt.removed, tt.renamed, tt.unchanged)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			// The point of the whole function: no category with a count of
			// zero is ever named. Checked per part, since "10 unchanged"
			// legitimately contains a zero.
			for _, part := range strings.Split(got, ", ") {
				if strings.HasPrefix(part, "0 ") {
					t.Errorf("%q names a category that did not happen", got)
				}
			}
		})
	}
}

func TestDescribeList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := describeList(nil); got != "(empty)" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("short list is spelled out", func(t *testing.T) {
		got := describeList([]string{"~/dev/*", "~/work/**/*"})
		if got != "~/dev/*, ~/work/**/*" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("long list collapses to a count", func(t *testing.T) {
		got := describeList(defaultProjectMarkers)
		if !strings.HasSuffix(got, "entries") {
			t.Errorf("got %q, want a count", got)
		}
		if len(got) > inlineListLimit {
			t.Errorf("the collapsed form is still too long: %q", got)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		if got := describeList([]string{"go.mod"}); got != "go.mod" {
			t.Errorf("got %q", got)
		}
	})
}

func TestParseOutputMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want outputMode
		ok   bool
	}{
		{"default", nil, outputSummary, true},
		{"json", []string{"--json"}, outputJSON, true},
		{"verbose", []string{"--verbose"}, outputVerbose, true},
		{"single dash", []string{"-json"}, outputJSON, true},
		{"unknown flag", []string{"--nope"}, outputSummary, false},
		// --json is already complete, so combining the two asks for
		// contradictory things rather than something sensible.
		{"both rejected", []string{"--json", "--verbose"}, outputSummary, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOutputMode("test", tt.args)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("mode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitOutputFlags(t *testing.T) {
	// Go's flag package stops at the first positional argument, which would
	// make flag position significant for no reason a user could guess.
	tests := []struct {
		name     string
		args     []string
		wantMode outputMode
		wantOps  []string
	}{
		{"no flags", []string{"project_globs", "~/dev/*"}, outputSummary, []string{"project_globs", "~/dev/*"}},
		{"flag first", []string{"--json", "project_globs", "~/dev/*"}, outputJSON, []string{"project_globs", "~/dev/*"}},
		{"flag last", []string{"project_globs", "~/dev/*", "--json"}, outputJSON, []string{"project_globs", "~/dev/*"}},
		{"flag in the middle", []string{"project_globs", "--verbose", "~/dev/*"}, outputVerbose, []string{"project_globs", "~/dev/*"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, operands, ok := splitOutputFlags("test", tt.args)
			if !ok {
				t.Fatal("unexpected failure")
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", mode, tt.wantMode)
			}
			if !equalStrings(operands, tt.wantOps) {
				t.Errorf("operands = %v, want %v", operands, tt.wantOps)
			}
		})
	}
}

func TestEffectiveProperty(t *testing.T) {
	t.Run("unset falls back to defaults and says so", func(t *testing.T) {
		values, isDefault := effectiveProperty(&Config{}, "project_markers")
		if !isDefault {
			t.Error("an unset property must be reported as coming from the defaults")
		}
		if !equalStrings(values, defaultProjectMarkers) {
			t.Errorf("got %v", values)
		}
	})

	t.Run("configured value is not a default", func(t *testing.T) {
		values, isDefault := effectiveProperty(&Config{ProjectMarkers: []string{"go.mod"}}, "project_markers")
		if isDefault {
			t.Error("a configured value must not be labelled a default")
		}
		if !equalStrings(values, []string{"go.mod"}) {
			t.Errorf("got %v", values)
		}
	})

	t.Run("explicitly empty is configured, not defaulted", func(t *testing.T) {
		values, isDefault := effectiveProperty(&Config{ProjectMarkers: []string{}}, "project_markers")
		if isDefault {
			t.Error("an explicitly empty list is a decision, not a default")
		}
		if len(values) != 0 {
			t.Errorf("got %v", values)
		}
	})

	t.Run("property without defaults", func(t *testing.T) {
		values, isDefault := effectiveProperty(&Config{}, "project_globs")
		if isDefault || len(values) != 0 {
			t.Errorf("got %v / %v", values, isDefault)
		}
	})
}

// --- the diff a rescan now reports ---

func TestRescanReportsWhatItChanged(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")
	makeProject(t, filepath.Join(root, "gone"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})

	first, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(first.Added) != 2 || first.Unchanged != 0 {
		t.Errorf("first run: added=%v unchanged=%d, want 2 added and 0 unchanged", first.Added, first.Unchanged)
	}
	if !first.Changed() {
		t.Error("a first run changes things")
	}

	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	makeProject(t, filepath.Join(root, "fresh"), "go.mod")

	second, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(second.Added) != 1 || second.Added[0].Name != "fresh" {
		t.Errorf("added = %v, want [fresh]", second.Added)
	}
	if len(second.Removed) != 1 || second.Removed[0].Name != "gone" {
		t.Errorf("removed = %v, want [gone]", second.Removed)
	}
	if second.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1 (api)", second.Unchanged)
	}

	third, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if third.Changed() {
		t.Errorf("a repeated rescan changes nothing, got %+v", third)
	}
	if third.Unchanged != 2 {
		t.Errorf("unchanged = %d, want 2", third.Unchanged)
	}
}

func TestRescanCountsRenameAsChangeNotUnchanged(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "dev", "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*", "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}

	// A colliding project forces the first one to be renamed.
	makeProject(t, filepath.Join(root, "work", "api"), "go.mod")

	result, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(result.Renamed) != 1 {
		t.Fatalf("renamed = %v, want one", result.Renamed)
	}
	// Two projects: one added, one renamed. Neither is unchanged.
	if result.Unchanged != 0 {
		t.Errorf("unchanged = %d, want 0 — a rename is a change", result.Unchanged)
	}
	if !result.Changed() {
		t.Error("a rename must count as a change")
	}
}

func TestRescanAndPreflightAgree(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "api"), "go.mod")

	svc, _ := newTestService(t, Config{ProjectGlobs: []string{filepath.Join(root, "*")}})
	if _, err := svc.Rescan(); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	makeProject(t, filepath.Join(root, "web"), "package.json")

	// What doctor promised has to be what rescan then does.
	preflight, err := svc.Preflight()
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	result, err := svc.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}

	if len(preflight.Added) != len(result.Added) || preflight.Unchanged != result.Unchanged {
		t.Errorf("preflight predicted added=%d unchanged=%d, rescan did added=%d unchanged=%d",
			len(preflight.Added), preflight.Unchanged, len(result.Added), result.Unchanged)
	}
}
