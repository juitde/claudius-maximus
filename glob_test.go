package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// globTree builds a fixture:
//
//	root/a/api                 (depth 2)
//	root/a/nested/deep/api     (depth 4)
//	root/b/api                 (depth 2)
//	root/b/node_modules/pkg    (inside a pruned directory)
//	root/b/.git/objects        (inside a pruned directory)
func globTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{
		"a/api",
		"a/nested/deep/api",
		"b/api",
		"b/node_modules/pkg",
		"b/.git/objects",
	} {
		mustMkdir(t, filepath.Join(root, filepath.FromSlash(dir)))
	}
	return root
}

// relMatches turns absolute matches into slash-separated paths relative to
// root, sorted, so expectations read like the fixture above.
func relMatches(t *testing.T, root string, e *GlobExpansion) []string {
	t.Helper()
	out := make([]string, 0, len(e.Matches))
	for _, m := range e.Matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

func TestExpandGlobPatternWithoutGlobstar(t *testing.T) {
	root := globTree(t)

	got, err := expandGlobPattern(filepath.Join(root, "*", "api"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a/api", "b/api"}
	if !equalStrings(relMatches(t, root, got), want) {
		t.Errorf("got %v, want %v", relMatches(t, root, got), want)
	}
	if len(got.Pruned) != 0 {
		t.Errorf("pruning must not apply without %s, got %v", globstar, got.Pruned)
	}
}

func TestExpandGlobPatternGlobstarCrossesLevels(t *testing.T) {
	root := globTree(t)

	// This is the case Go's filepath.Glob cannot express: ** has to reach
	// both the shallow and the deep "api".
	got, err := expandGlobPattern(filepath.Join(root, globstar, "api"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a/api", "a/nested/deep/api", "b/api"}
	if !equalStrings(relMatches(t, root, got), want) {
		t.Errorf("got %v, want %v", relMatches(t, root, got), want)
	}
}

func TestExpandGlobPatternGlobstarStar(t *testing.T) {
	root := globTree(t)

	got, err := expandGlobPattern(filepath.Join(root, globstar, "*"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Every directory at any depth, minus what pruning cut off.
	want := []string{"a", "a/api", "a/nested", "a/nested/deep", "a/nested/deep/api", "b", "b/api"}
	if !equalStrings(relMatches(t, root, got), want) {
		t.Errorf("got %v, want %v", relMatches(t, root, got), want)
	}
}

func TestExpandGlobPatternPrunes(t *testing.T) {
	root := globTree(t)

	got, err := expandGlobPattern(filepath.Join(root, globstar, "*"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range relMatches(t, root, got) {
		if m == "b/node_modules" || m == "b/node_modules/pkg" || m == "b/.git" || m == "b/.git/objects" {
			t.Errorf("%q should have been pruned", m)
		}
	}
	if got.Pruned["node_modules"] != 1 {
		t.Errorf("node_modules pruned %d times, want 1", got.Pruned["node_modules"])
	}
	if got.Pruned[".git"] != 1 {
		t.Errorf(".git pruned %d times, want 1", got.Pruned[".git"])
	}
}

func TestExpandGlobPatternPruningCanBeDisabled(t *testing.T) {
	root := globTree(t)

	got, err := expandGlobPattern(filepath.Join(root, globstar, "*"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(relMatches(t, root, got), "b/node_modules/pkg") {
		t.Error("with pruning disabled, everything below node_modules must be reachable")
	}
	if len(got.Pruned) != 0 {
		t.Errorf("nothing should have been pruned, got %v", got.Pruned)
	}
}

func TestExpandGlobPatternTrailingGlobstar(t *testing.T) {
	root := globTree(t)

	trailing, err := expandGlobPattern(filepath.Join(root, globstar), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	explicit, err := expandGlobPattern(filepath.Join(root, globstar, "*"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalStrings(relMatches(t, root, trailing), relMatches(t, root, explicit)) {
		t.Errorf("a trailing %s should mean %s/*: got %v vs %v",
			globstar, globstar, relMatches(t, root, trailing), relMatches(t, root, explicit))
	}
}

func TestExpandGlobPatternRejectsMultipleGlobstars(t *testing.T) {
	root := globTree(t)

	_, err := expandGlobPattern(filepath.Join(root, globstar, "x", globstar, "*"), nil, nil)
	if err == nil {
		t.Fatal("expected a second ** to be rejected rather than silently mishandled")
	}
}

func TestExpandGlobPatternMultiSegmentSuffix(t *testing.T) {
	root := globTree(t)

	got, err := expandGlobPattern(filepath.Join(root, globstar, "deep", "api"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a/nested/deep/api"}
	if !equalStrings(relMatches(t, root, got), want) {
		t.Errorf("got %v, want %v", relMatches(t, root, got), want)
	}
}

func TestExpandGlobPatternDoesNotFollowSymlinkedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}

	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "real", "inner"))
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	got, err := expandGlobPattern(filepath.Join(root, globstar, "*"), defaultPruneDirectories, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches := relMatches(t, root, got)
	// The link itself is a directory and counts...
	if !contains(matches, "link") {
		t.Errorf("a symlinked directory should still match, got %v", matches)
	}
	// ...but walking through it must not happen, or a cycle would hang the scan.
	if contains(matches, "link/inner") {
		t.Errorf("recursion must not descend through a symlink, got %v", matches)
	}
}

func TestExpandGlobPatternNoMatches(t *testing.T) {
	root := t.TempDir()

	got, err := expandGlobPattern(filepath.Join(root, globstar, "nothing-here"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Matches) != 0 {
		t.Errorf("got %v, want no matches", got.Matches)
	}
}

func TestExpandGlobPatternNonexistentRoot(t *testing.T) {
	got, err := expandGlobPattern(filepath.Join(t.TempDir(), "absent", globstar, "*"), nil, nil)
	if err != nil {
		t.Fatalf("a missing root is not an error: %v", err)
	}
	if len(got.Matches) != 0 {
		t.Errorf("got %v, want no matches", got.Matches)
	}
}

// --- integration with the scan ---

func TestResolveProjectsWithGlobstar(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "client", "team", "api"), "go.mod")
	makeProject(t, filepath.Join(root, "solo"), "go.mod")

	scan, err := resolveProjects(&Config{
		ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ** reaches both depths, which a plain glob cannot do.
	assertPaths(t, scan.Projects,
		filepath.Join(root, "solo"),
		filepath.Join(root, "client", "team", "api"),
	)
}

func TestResolveProjectsStopsAtProjectBoundaries(t *testing.T) {
	root := t.TempDir()
	// A repository with an inner package that also carries a marker. Only the
	// repository itself is a project; the sub-package is part of it.
	makeProject(t, filepath.Join(root, "repo"), "go.mod")
	makeProject(t, filepath.Join(root, "repo", "services", "billing"), "go.mod")

	scan, err := resolveProjects(&Config{
		ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPaths(t, scan.Projects, filepath.Join(root, "repo"))
}

func TestNamesInBothMarkerAndPruneListsBehaveCorrectly(t *testing.T) {
	// .idea and .vscode appear in both default lists. The combination has to
	// mean "this directory is a project, and do not search inside it" — not
	// one cancelling the other.
	root := t.TempDir()
	ideDir := filepath.Join(root, "legacy-php")
	mustMkdir(t, filepath.Join(ideDir, ".idea"))
	// Something inside .idea that would otherwise look like a project.
	mustMkdir(t, filepath.Join(ideDir, ".idea", "modules"))
	mustWrite(t, filepath.Join(ideDir, ".idea", "modules", "go.mod"), "")

	scan, err := resolveProjects(&Config{
		ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The project is found via the marker...
	assertPaths(t, scan.Projects, ideDir)
	// ...and nothing inside .idea was reached. Descent stops at the project
	// boundary first, so the prune entry never even has to fire here.
	for _, p := range scan.Projects {
		if strings.Contains(p.Path, ".idea") {
			t.Errorf("%q is inside .idea and should not have been reached", p.Path)
		}
	}
}

func TestResolveProjectsPrunesOutsideProjects(t *testing.T) {
	root := t.TempDir()
	// node_modules sitting in a plain directory, not inside a project, so the
	// prune list is what has to stop the walk.
	makeProject(t, filepath.Join(root, "scratch", "node_modules", "dep"), "package.json")
	makeProject(t, filepath.Join(root, "keep"), "go.mod")

	scan, err := resolveProjects(&Config{
		ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPaths(t, scan.Projects, filepath.Join(root, "keep"))
	if scan.Pruned["node_modules"] == 0 {
		t.Errorf("node_modules should have been pruned and reported, got %v", scan.Pruned)
	}
}

func TestResolveProjectsPruneListIsConfigurable(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "keep"), "go.mod")
	makeProject(t, filepath.Join(root, "build", "artifact"), "go.mod")

	t.Run("default prunes build", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertPaths(t, scan.Projects, filepath.Join(root, "keep"))
	})

	t.Run("removing build from the list reaches it", func(t *testing.T) {
		markers := defaultProjectMarkers
		var pruneWithoutBuild []string
		for _, d := range defaultPruneDirectories {
			if d != "build" {
				pruneWithoutBuild = append(pruneWithoutBuild, d)
			}
		}

		scan, err := resolveProjects(&Config{
			ProjectGlobs:     []string{filepath.Join(root, globstar, "*")},
			ProjectMarkers:   markers,
			PruneDirectories: pruneWithoutBuild,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertPaths(t, scan.Projects,
			filepath.Join(root, "keep"),
			filepath.Join(root, "build", "artifact"),
		)
	})
}

func TestConfigPruneDirectories(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{"absent uses defaults", `{}`, defaultPruneDirectories},
		{"empty disables pruning", `{"prune_directories":[]}`, []string{}},
		{"explicit list replaces defaults", `{"prune_directories":["node_modules"]}`, []string{"node_modules"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			mustWrite(t, path, tt.json)

			cfg, err := loadConfig(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := cfg.pruneDirectories(); !equalStrings(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateDirectoryName(t *testing.T) {
	valid := []string{"node_modules", ".git", "build"}
	for _, v := range valid {
		if err := validateDirectoryName(v); err != nil {
			t.Errorf("%q should be valid: %v", v, err)
		}
	}

	invalid := []string{"", "   ", "src/vendor", `src\vendor`, "*.tmp", "node_modules?"}
	for _, v := range invalid {
		if err := validateDirectoryName(v); err == nil {
			t.Errorf("%q should have been rejected", v)
		}
	}
}
