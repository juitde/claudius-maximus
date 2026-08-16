package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	const home = "/home/tester"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare tilde", "~", home},
		{"tilde slash", "~/dev", filepath.Join(home, "dev")},
		{"tilde slash glob", "~/dev/*", filepath.Join(home, "dev", "*")},
		{"absolute path untouched", "/opt/src/*", "/opt/src/*"},
		{"relative path untouched", "dev/*", "dev/*"},
		{"other user not expanded", "~alice/dev", "~alice/dev"},
		{"tilde inside path untouched", "/opt/~/dev", "/opt/~/dev"},
		{"empty stays empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHome(tt.in, home); got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// directoryMarkers are the default markers that appear as directories rather
// than files, so that fixtures mirror what a real project looks like.
var directoryMarkers = map[string]bool{
	".git":          true,
	".hg":           true,
	".svn":          true,
	".ddev":         true,
	".devcontainer": true,
	".idea":         true,
	".vscode":       true,
	".vs":           true,
	".fleet":        true,
	".zed":          true,
}

func TestLooksLikeProject(t *testing.T) {
	t.Run("every default marker is recognised", func(t *testing.T) {
		for _, marker := range defaultProjectMarkers {
			dir := t.TempDir()
			// Some markers are directories in the wild and some are files.
			// Both must be accepted, so create each in its natural form.
			if directoryMarkers[marker] {
				mustMkdir(t, filepath.Join(dir, marker))
			} else {
				mustWrite(t, filepath.Join(dir, marker), "")
			}
			if !looksLikeProject(dir, defaultProjectMarkers) {
				t.Errorf("marker %q not recognised", marker)
			}
		}
	})

	t.Run("empty directory is not a project", func(t *testing.T) {
		if looksLikeProject(t.TempDir(), defaultProjectMarkers) {
			t.Error("empty directory reported as project")
		}
	})

	t.Run("unrelated files are not markers", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "README.md"), "")
		mustWrite(t, filepath.Join(dir, "notes.txt"), "")
		if looksLikeProject(dir, defaultProjectMarkers) {
			t.Error("directory without markers reported as project")
		}
	})

	t.Run("empty marker set accepts anything", func(t *testing.T) {
		if !looksLikeProject(t.TempDir(), nil) {
			t.Error("empty marker set should accept every directory")
		}
		if !looksLikeProject(t.TempDir(), []string{}) {
			t.Error("empty marker set should accept every directory")
		}
	})

	t.Run("glob markers match by pattern", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "network.tf"), "")

		if !looksLikeProject(dir, []string{"*.tf"}) {
			t.Error("glob marker did not match")
		}
		if looksLikeProject(dir, []string{"*.sln"}) {
			t.Error("non-matching glob marker was accepted")
		}
	})

	t.Run("terraform and dotnet projects are found by default", func(t *testing.T) {
		for _, file := range []string{"main.tf", "App.csproj", "Solution.sln", "pom.xml", "build.gradle.kts"} {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, file), "")
			if !looksLikeProject(dir, defaultProjectMarkers) {
				t.Errorf("%s not recognised by the default markers", file)
			}
		}
	})

	t.Run("editor metadata alone marks a project", func(t *testing.T) {
		// The case this exists for: a directory with no manifest of any kind,
		// which someone has nonetheless opened as a project.
		for _, marker := range []string{".idea", ".vscode", ".vs", ".fleet", ".zed"} {
			dir := t.TempDir()
			mustMkdir(t, filepath.Join(dir, marker))
			if !looksLikeProject(dir, defaultProjectMarkers) {
				t.Errorf("a directory containing %s should count as a project", marker)
			}
		}
	})

	t.Run("a ddev environment alone marks a project", func(t *testing.T) {
		// DDEV projects need no other marker: the .ddev directory holds the
		// whole environment definition, and the project around it may well be
		// plain PHP with no composer.json.
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, ".ddev"))
		mustWrite(t, filepath.Join(dir, ".ddev", "config.yaml"), "name: demo\n")

		if !looksLikeProject(dir, defaultProjectMarkers) {
			t.Error("a directory containing .ddev should count as a project")
		}
	})

	t.Run("directory name containing glob metacharacters is not misread", func(t *testing.T) {
		// The directory name is data. If it were spliced into a glob pattern
		// the brackets below would be parsed as a character class.
		parent := t.TempDir()
		dir := filepath.Join(parent, "release-[v1]")
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "main.tf"), "")

		if !looksLikeProject(dir, []string{"*.tf"}) {
			t.Error("glob marker failed inside a directory with metacharacters in its name")
		}
	})

	t.Run("custom marker set is honoured exclusively", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "go.mod"), "")

		if !looksLikeProject(dir, []string{"go.mod"}) {
			t.Error("custom marker not recognised")
		}
		// A default marker that is not in the custom set must not count.
		if looksLikeProject(dir, []string{"Cargo.toml"}) {
			t.Error("marker outside the configured set was accepted")
		}
	})
}

func TestDefaultProjectMarkersAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, marker := range defaultProjectMarkers {
		if marker == "" {
			t.Error("empty marker in the default set")
		}
		if seen[marker] {
			t.Errorf("duplicate marker %q in the default set", marker)
		}
		seen[marker] = true

		if isGlobPattern(marker) {
			if _, err := filepath.Match(marker, ""); err != nil {
				t.Errorf("default marker %q is not a valid pattern: %v", marker, err)
			}
		}
	}
}

func TestValidateMarkers(t *testing.T) {
	t.Run("well-formed markers pass through unchanged", func(t *testing.T) {
		in := []string{"go.mod", "*.tf", ".git"}
		got, warnings := validateMarkers(in)
		if !equalStrings(got, in) {
			t.Errorf("got %v, want %v", got, in)
		}
		if len(warnings) != 0 {
			t.Errorf("unexpected warnings: %v", warnings)
		}
	})

	t.Run("malformed pattern is dropped and reported", func(t *testing.T) {
		got, warnings := validateMarkers([]string{"[unterminated", "go.mod"})
		if !equalStrings(got, []string{"go.mod"}) {
			t.Errorf("got %v, want the usable marker only", got)
		}
		if !hasWarningContaining(warnings, "malformed project marker") {
			t.Errorf("expected a warning, got %v", warnings)
		}
	})

	t.Run("literal markers are never treated as patterns", func(t *testing.T) {
		// A literal filename cannot be malformed, however odd it looks.
		got, warnings := validateMarkers([]string{"weird{name}.txt"})
		if len(got) != 1 || len(warnings) != 0 {
			t.Errorf("got %v / %v, want the marker kept without warnings", got, warnings)
		}
	})
}

func TestIsGlobPattern(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"go.mod", false},
		{".git", false},
		{"docker-compose.yml", false},
		{"*.tf", true},
		{"file?.txt", true},
		{"[abc].go", true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isGlobPattern(tt.in); got != tt.want {
				t.Errorf("isGlobPattern(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestConfigMarkers(t *testing.T) {
	// The three states hinge on JSON distinguishing an absent key from an
	// empty array, so exercise them through actual unmarshalling rather than
	// by constructing the struct by hand.
	tests := []struct {
		name string
		json string
		want []string
	}{
		{"absent key falls back to defaults", `{}`, defaultProjectMarkers},
		{"null falls back to defaults", `{"project_markers":null}`, defaultProjectMarkers},
		{"empty array disables filtering", `{"project_markers":[]}`, []string{}},
		{"explicit list replaces defaults", `{"project_markers":["go.mod"]}`, []string{"go.mod"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			mustWrite(t, path, tt.json)

			cfg, err := loadConfig(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := cfg.markers()
			if !equalStrings(got, tt.want) {
				t.Errorf("markers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing file yields empty config", func(t *testing.T) {
		cfg, err := loadConfig(filepath.Join(t.TempDir(), "absent.json"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.ProjectGlobs) != 0 {
			t.Errorf("expected no globs, got %v", cfg.ProjectGlobs)
		}
	})

	t.Run("valid file is parsed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		mustWrite(t, path, `{"project_globs":["~/dev/*","/opt/src/*"]}`)

		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"~/dev/*", "/opt/src/*"}
		if len(cfg.ProjectGlobs) != len(want) {
			t.Fatalf("got %v, want %v", cfg.ProjectGlobs, want)
		}
		for i := range want {
			if cfg.ProjectGlobs[i] != want[i] {
				t.Errorf("glob %d = %q, want %q", i, cfg.ProjectGlobs[i], want[i])
			}
		}
	})

	t.Run("malformed JSON reports the path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		mustWrite(t, path, `{"project_globs": [`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatal("expected an error for malformed JSON")
		}
		// The path matters: the user needs to know which file to fix.
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not mention %q", err, path)
		}
	})
}

func TestResolveProjects(t *testing.T) {
	// Layout:
	//   root/dev/api        -> project (go.mod)
	//   root/dev/web        -> project (package.json)
	//   root/dev/notes      -> no marker
	//   root/dev/loose.txt  -> not a directory
	//   root/other/tool     -> project (.git)
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "dev", "api"), "go.mod")
	makeProject(t, filepath.Join(root, "dev", "web"), "package.json")
	mustMkdir(t, filepath.Join(root, "dev", "notes"))
	mustWrite(t, filepath.Join(root, "dev", "loose.txt"), "")
	makeProject(t, filepath.Join(root, "other", "tool"), ".git")

	t.Run("finds projects and skips non-projects", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{filepath.Join(root, "dev", "*")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertPaths(t, scan.Projects,
			filepath.Join(root, "dev", "api"),
			filepath.Join(root, "dev", "web"),
		)
	})

	t.Run("deduplicates across overlapping patterns", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{
				filepath.Join(root, "dev", "*"),
				filepath.Join(root, "*", "*"),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertPaths(t, scan.Projects,
			filepath.Join(root, "dev", "api"),
			filepath.Join(root, "dev", "web"),
			filepath.Join(root, "other", "tool"),
		)
	})

	t.Run("malformed pattern warns without aborting the rest", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{
				filepath.Join(root, "dev", "["), // unterminated character class
				filepath.Join(root, "dev", "*"),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The good pattern must still have been processed.
		assertPaths(t, scan.Projects,
			filepath.Join(root, "dev", "api"),
			filepath.Join(root, "dev", "web"),
		)
		if !hasWarningContaining(scan.Warnings, "malformed pattern") {
			t.Errorf("expected a malformed-pattern warning, got %v", scan.Warnings)
		}
	})

	t.Run("pattern matching nothing warns", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{filepath.Join(root, "nonexistent", "*")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !hasWarningContaining(scan.Warnings, "matched nothing") {
			t.Errorf("expected a matched-nothing warning, got %v", scan.Warnings)
		}
	})

	t.Run("directories without markers warn", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs: []string{filepath.Join(root, "dev", "notes")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !hasWarningContaining(scan.Warnings, "project marker") {
			t.Errorf("expected a missing-marker warning, got %v", scan.Warnings)
		}
	})

	t.Run("empty marker set accepts marker-less directories", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs:   []string{filepath.Join(root, "dev", "*")},
			ProjectMarkers: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "notes" has no marker but is still a directory, so it now counts.
		// "loose.txt" is a file and must stay excluded regardless.
		assertPaths(t, scan.Projects,
			filepath.Join(root, "dev", "api"),
			filepath.Join(root, "dev", "web"),
			filepath.Join(root, "dev", "notes"),
		)
		if len(scan.Warnings) != 0 {
			t.Errorf("expected no warnings, got %v", scan.Warnings)
		}
	})

	t.Run("custom marker set narrows the result", func(t *testing.T) {
		scan, err := resolveProjects(&Config{
			ProjectGlobs:   []string{filepath.Join(root, "dev", "*")},
			ProjectMarkers: []string{"go.mod"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only "api" has go.mod; "web" has package.json, which is no longer
		// in the marker set.
		assertPaths(t, scan.Projects, filepath.Join(root, "dev", "api"))
	})

	t.Run("no globs configured yields nothing", func(t *testing.T) {
		scan, err := resolveProjects(&Config{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(scan.Projects) != 0 || len(scan.Warnings) != 0 {
			t.Errorf("expected empty result, got %v / %v", scan.Projects, scan.Warnings)
		}
	})
}

// --- helpers ---

func makeProject(t *testing.T, dir, marker string) {
	t.Helper()
	mustMkdir(t, dir)
	if marker == ".git" {
		mustMkdir(t, filepath.Join(dir, marker))
		return
	}
	mustWrite(t, filepath.Join(dir, marker), "")
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	// Create the parent so fixtures can name a path under state/ without
	// each test having to lay out the directory first.
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// assertPaths compares discovered project paths against an expected set,
// ignoring order. Paths are resolved through EvalSymlinks because t.TempDir()
// hands out paths below /var on macOS, which is a symlink to /private/var,
// while resolveProjects reports what filepath.Abs produced.
func assertPaths(t *testing.T, projects []Project, want ...string) {
	t.Helper()

	got := map[string]bool{}
	for _, p := range projects {
		got[resolve(t, p.Path)] = true
	}
	if len(got) != len(want) {
		t.Fatalf("got %d projects (%v), want %d (%v)", len(got), keys(got), len(want), want)
	}
	for _, w := range want {
		if !got[resolve(t, w)] {
			t.Errorf("missing expected project %q (got %v)", w, keys(got))
		}
	}
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasWarningContaining(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
