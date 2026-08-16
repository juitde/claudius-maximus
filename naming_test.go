package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestAssignProjectNames(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  map[string]string // path -> expected name
	}{
		{
			name:  "no projects",
			paths: nil,
			want:  map[string]string{},
		},
		{
			name:  "single project keeps the bare directory name",
			paths: []string{"/home/u/dev/api"},
			want:  map[string]string{"/home/u/dev/api": "api"},
		},
		{
			name:  "distinct names are left short",
			paths: []string{"/home/u/dev/api", "/home/u/dev/frontend"},
			want: map[string]string{
				"/home/u/dev/api":      "api",
				"/home/u/dev/frontend": "frontend",
			},
		},
		{
			// The worked example from DEVELOPMENT.md.
			name: "collisions lift, non-collisions stay",
			paths: []string{
				"/home/u/dev/api",
				"/home/u/work/client-a/api",
				"/home/u/work/client-b/api",
				"/home/u/dev/frontend",
			},
			want: map[string]string{
				"/home/u/dev/api":           "dev-api",
				"/home/u/work/client-a/api": "client-a-api",
				"/home/u/work/client-b/api": "client-b-api",
				"/home/u/dev/frontend":      "frontend",
			},
		},
		{
			name: "lifts repeatedly until unique",
			paths: []string{
				"/home/u/x/dev/api",
				"/home/u/y/dev/api",
			},
			want: map[string]string{
				"/home/u/x/dev/api": "x-dev-api",
				"/home/u/y/dev/api": "y-dev-api",
			},
		},
		{
			// Lifting "api" to "a-api" lands on a name that already existed,
			// so a second pass has to resolve the newly created collision.
			name: "collision created by lifting is resolved",
			paths: []string{
				"/a/api",
				"/b/api",
				"/x/a-api",
			},
			want: map[string]string{
				"/a/api":   "a-api",
				"/b/api":   "b-api",
				"/x/a-api": "x-a-api",
			},
		},
		{
			name:  "segments are lowercased",
			paths: []string{"/home/u/dev/MyAPI"},
			want:  map[string]string{"/home/u/dev/MyAPI": "myapi"},
		},
		{
			name: "spaces, underscores and dots become dashes",
			paths: []string{
				"/home/u/dev/my project",
				"/home/u/dev/my_service",
				"/home/u/dev/my.tool",
			},
			want: map[string]string{
				"/home/u/dev/my project": "my-project",
				"/home/u/dev/my_service": "my-service",
				"/home/u/dev/my.tool":    "my-tool",
			},
		},
		{
			name:  "trailing slash is irrelevant",
			paths: []string{"/home/u/dev/api/"},
			want:  map[string]string{"/home/u/dev/api/": "api"},
		},
		{
			name:  "project directly under root",
			paths: []string{"/api"},
			want:  map[string]string{"/api": "api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projects := projectsFor(tt.paths)
			assignProjectNames(projects)

			for _, p := range projects {
				want, ok := tt.want[p.Path]
				if !ok {
					t.Fatalf("unexpected project %q in result", p.Path)
				}
				if p.Name != want {
					t.Errorf("%s: got name %q, want %q", p.Path, p.Name, want)
				}
			}
			assertNamesUnique(t, projects)
		})
	}
}

// TestAssignProjectNamesAlwaysUnique is the property that actually matters:
// whatever the input, two projects must never share a name, and every project
// must get one.
func TestAssignProjectNamesAlwaysUnique(t *testing.T) {
	cases := [][]string{
		{"/a/api", "/b/api", "/c/api", "/d/api"},
		{"/a/b/c", "/a/b/c"},                 // literal duplicates
		{"/dev/api", "/dev/API", "/dev/Api"}, // differ only by case
		{"/dev/a-b", "/dev/a_b", "/dev/a b"}, // collapse to the same segment
		{"/dev/...", "/dev/___"},             // punctuation-only segments
		{"/x", "/y", "/z"},                   // single-segment paths
		{"/a/b/api", "/c/b/api", "/d/b/api"}, // shared middle segment
		{"/a/api", "/x/a-api", "/y/x-a-api"}, // cascading lifts
	}

	for i, paths := range cases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			projects := projectsFor(paths)
			assignProjectNames(projects)

			for _, p := range projects {
				if p.Name == "" {
					t.Errorf("project %q got an empty name", p.Path)
				}
			}
			assertNamesUnique(t, projects)
		})
	}
}

func TestAssignProjectNamesIsDeterministic(t *testing.T) {
	// Names must not depend on map iteration order, so the same input has to
	// produce the same output every time. Identical paths force the numeric
	// suffix fallback, which is the part most at risk.
	paths := []string{"/a/b/c", "/a/b/c", "/a/b/c", "/dev/api", "/other/api"}

	first := namesFor(paths)
	for range 20 {
		if got := namesFor(paths); !equalStrings(got, first) {
			t.Fatalf("non-deterministic result:\n first: %v\n   got: %v", first, got)
		}
	}
}

func TestSanitizeSegment(t *testing.T) {
	tests := []struct{ in, want string }{
		{"api", "api"},
		{"API", "api"},
		{"my project", "my-project"},
		{"my_service", "my-service"},
		{"my.tool", "my-tool"},
		{"a---b", "a-b"},     // runs collapse
		{"-lead-", "lead"},   // edges trimmed
		{"...", "project"},   // nothing usable left
		{"", "project"},      // empty in, usable name out
		{"v2.1", "v2-1"},     // digits survive
		{"Ünïcödé", "n-c-d"}, // non-ASCII letters drop out
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := sanitizeSegment(tt.in); got != tt.want {
				t.Errorf("sanitizeSegment(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReversedSegments(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"/home/u/dev/api", []string{"api", "dev", "u", "home"}},
		{"/api", []string{"api"}},
		{"/home/u/dev/api/", []string{"api", "dev", "u", "home"}},
		{"/home//u///api", []string{"api", "u", "home"}},
		{"/", nil},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := reversedSegments(tt.in)
			if !equalStrings(got, tt.want) {
				t.Errorf("reversedSegments(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// --- helpers ---

func projectsFor(paths []string) []Project {
	projects := make([]Project, len(paths))
	for i, p := range paths {
		projects[i] = Project{Path: p}
	}
	return projects
}

func namesFor(paths []string) []string {
	projects := projectsFor(paths)
	assignProjectNames(projects)

	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names
}

func assertNamesUnique(t *testing.T, projects []Project) {
	t.Helper()

	seen := map[string][]string{}
	for _, p := range projects {
		seen[p.Name] = append(seen[p.Name], p.Path)
	}
	for name, paths := range seen {
		if len(paths) > 1 {
			sort.Strings(paths)
			t.Errorf("name %q assigned to %d projects: %s", name, len(paths), strings.Join(paths, ", "))
		}
	}
}
