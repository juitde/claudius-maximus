package main

import (
	"path/filepath"
	"testing"
)

// rejectionFor finds a rejected entry by path suffix, so tests can name the
// fixture directory rather than the temp-dir prefix.
func rejectionFor(t *testing.T, scan *ScanResult, suffix string) RejectedDir {
	t.Helper()
	want := filepath.FromSlash(suffix)
	for _, r := range scan.Rejected {
		if filepath.Base(r.Path) == filepath.Base(want) && hasPathSuffix(r.Path, want) {
			return r
		}
	}
	t.Fatalf("no rejected entry ending in %q; got %v", suffix, rejectedPaths(scan))
	return RejectedDir{}
}

func hasPathSuffix(path, suffix string) bool {
	sep := string(filepath.Separator)
	return path == suffix || len(path) > len(suffix) &&
		path[len(path)-len(suffix):] == suffix &&
		path[len(path)-len(suffix)-1:len(path)-len(suffix)] == sep
}

func rejectedPaths(scan *ScanResult) []string {
	out := make([]string, len(scan.Rejected))
	for i, r := range scan.Rejected {
		out[i] = r.Path
	}
	return out
}

// rejectionTree builds the shape that motivated this classification:
//
//	root/clients                     container, holds projects below
//	root/clients/acme                container, holds one project
//	root/clients/acme/api            a project
//	root/clients/scratch             skipped, holds nothing
//	root/barren                      skipped, holds nothing
//	root/barren/a                    skipped, explained by root/barren
//	root/barren/a/b                  skipped, explained by root/barren
func rejectionTree(t *testing.T) *ScanResult {
	t.Helper()

	root := t.TempDir()
	makeProject(t, filepath.Join(root, "clients", "acme", "api"), "go.mod")
	mustMkdir(t, filepath.Join(root, "clients", "scratch"))
	mustMkdir(t, filepath.Join(root, "barren", "a", "b"))

	scan, err := resolveProjects(&Config{
		ProjectGlobs: []string{filepath.Join(root, globstar, "*")},
	})
	if err != nil {
		t.Fatalf("resolveProjects: %v", err)
	}
	return scan
}

func TestAnnotateRejectionsCountsContainedProjects(t *testing.T) {
	scan := rejectionTree(t)

	clients := rejectionFor(t, scan, filepath.Join("clients"))
	if clients.ContainsProjects != 1 {
		t.Errorf("clients contains %d projects, want 1", clients.ContainsProjects)
	}
	acme := rejectionFor(t, scan, filepath.Join("clients", "acme"))
	if acme.ContainsProjects != 1 {
		t.Errorf("clients/acme contains %d projects, want 1", acme.ContainsProjects)
	}
	barren := rejectionFor(t, scan, "barren")
	if barren.ContainsProjects != 0 {
		t.Errorf("barren contains %d projects, want 0", barren.ContainsProjects)
	}
}

func TestAnnotateRejectionsMarksCoveredDescendants(t *testing.T) {
	scan := rejectionTree(t)

	barren := rejectionFor(t, scan, "barren")
	a := rejectionFor(t, scan, filepath.Join("barren", "a"))
	b := rejectionFor(t, scan, filepath.Join("barren", "a", "b"))

	if barren.CoveredBy != "" {
		t.Errorf("the top of a barren chain must not be covered, got %q", barren.CoveredBy)
	}
	if a.CoveredBy != barren.Path {
		t.Errorf("barren/a covered by %q, want %q", a.CoveredBy, barren.Path)
	}
	// The nearest barren ancestor, not the outermost one.
	if b.CoveredBy != a.Path {
		t.Errorf("barren/a/b covered by %q, want %q", b.CoveredBy, a.Path)
	}
}

func TestAnnotateRejectionsDoesNotHideBehindAContainer(t *testing.T) {
	scan := rejectionTree(t)

	// clients/scratch sits under a container. The container explains nothing
	// about it, so it has to stay visible in its own right.
	scratch := rejectionFor(t, scan, filepath.Join("clients", "scratch"))
	if scratch.CoveredBy != "" {
		t.Errorf("a directory below a container must not be marked covered, got %q", scratch.CoveredBy)
	}
}

func TestSummarizeRejectionsSplitsAndCollapses(t *testing.T) {
	scan := rejectionTree(t)
	summary := summarizeRejections(scan.Rejected)

	// Containers: clients and clients/acme.
	if len(summary.Containers) != 2 {
		t.Errorf("got %d containers, want 2: %v", len(summary.Containers), summary.Containers)
	}

	// Skipped tops: barren and clients/scratch. The two levels below barren
	// collapse into its count.
	if len(summary.Skipped) != 2 {
		t.Fatalf("got %d skipped entries, want 2: %v", len(summary.Skipped), summary.Skipped)
	}

	byBase := map[string]skippedEntry{}
	for _, e := range summary.Skipped {
		byBase[filepath.Base(e.Dir.Path)] = e
	}
	if got := byBase["barren"].HiddenBelow; got != 2 {
		t.Errorf("barren hides %d directories, want 2", got)
	}
	if got := byBase["scratch"].HiddenBelow; got != 0 {
		t.Errorf("scratch hides %d directories, want 0", got)
	}
}

func TestSummarizeRejectionsOrdersContainersByProjectCount(t *testing.T) {
	summary := summarizeRejections([]RejectedDir{
		{Path: "/x/small", ContainsProjects: 1},
		{Path: "/x/big", ContainsProjects: 30},
		{Path: "/x/medium", ContainsProjects: 5},
	})

	want := []string{"/x/big", "/x/medium", "/x/small"}
	for i, w := range want {
		if summary.Containers[i].Path != w {
			t.Errorf("container %d = %q, want %q", i, summary.Containers[i].Path, w)
		}
	}
}

func TestSummarizeRejectionsEmpty(t *testing.T) {
	summary := summarizeRejections(nil)
	if len(summary.Skipped) != 0 || len(summary.Containers) != 0 {
		t.Errorf("expected an empty summary, got %+v", summary)
	}
}

func TestRejectionDetailSurvivesInJSON(t *testing.T) {
	// The collapsing is presentation only: every rejected directory must still
	// be present in the machine-readable output, with its reason.
	scan := rejectionTree(t)
	summary := summarizeRejections(scan.Rejected)

	shown := len(summary.Skipped) + len(summary.Containers)
	if shown >= len(scan.Rejected) {
		t.Fatalf("the summary (%d) should be smaller than the full list (%d), or this test proves nothing",
			shown, len(scan.Rejected))
	}
	for _, r := range scan.Rejected {
		if r.Reason == "" {
			t.Errorf("%q lost its reason", r.Path)
		}
	}
}

func TestIsAncestorDir(t *testing.T) {
	join := filepath.Join
	tests := []struct {
		ancestor, path string
		want           bool
	}{
		{join("/a", "b"), join("/a", "b", "c"), true},
		{join("/a", "b"), join("/a", "b", "c", "d"), true},
		{join("/a", "b"), join("/a", "b"), false},
		{join("/a", "b"), join("/a", "c"), false},
		// A sibling whose name merely starts with the ancestor's must not match.
		{join("/a", "b"), join("/a", "bb", "c"), false},
	}

	for _, tt := range tests {
		if got := isAncestorDir(tt.ancestor, tt.path); got != tt.want {
			t.Errorf("isAncestorDir(%q, %q) = %v, want %v", tt.ancestor, tt.path, got, tt.want)
		}
	}
}
