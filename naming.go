package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// assignProjectNames fills in Project.Name for every project, guaranteeing
// that no two projects end up with the same name.
//
// The rule:
//   - start with the directory name alone ("api")
//   - whenever several projects want the same name, each of them takes one
//     more path segment from the left ("dev-api", "client-a-api")
//   - repeat until nothing collides
//
// Only the projects that actually collide are lifted to a deeper level.
// A project whose name is unique stays short, which is the whole point: the
// common case should be typeable.
//
//	~/dev/api           -> dev-api        (collides with the other "api")
//	~/work/client-a/api -> client-a-api
//	~/dev/frontend      -> frontend       (never collides, stays short)
//
// Termination is guaranteed because absolute paths are unique, so lifting far
// enough always separates two projects. The one case that cannot be separated
// by path segments — two projects whose sanitised segments are identical all
// the way up — is caught by a numeric suffix at the end.
func assignProjectNames(projects []Project) {
	n := len(projects)
	if n == 0 {
		return
	}

	// segments[i] holds project i's path components, deepest first, so that
	// "level" and "number of segments consumed" are the same number.
	segments := make([][]string, n)
	levels := make([]int, n)
	for i, p := range projects {
		segments[i] = reversedSegments(p.Path)
		levels[i] = 1
	}

	nameAt := func(i int) string {
		level := min(levels[i], len(segments[i]))
		parts := make([]string, level)
		for k := range level {
			// Reverse back to natural order: deepest segment last.
			parts[k] = sanitizeSegment(segments[i][level-1-k])
		}
		return strings.Join(parts, "-")
	}

	// Lift colliding groups one level at a time. Each pass may create new
	// collisions (a lifted name can land on a name that was already taken),
	// so this repeats until a pass changes nothing.
	for {
		groups := groupByName(n, nameAt)

		changed := false
		for _, indices := range groups {
			if len(indices) <= 1 {
				continue
			}
			for _, i := range indices {
				if levels[i] < len(segments[i]) {
					levels[i]++
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	// Anything still sharing a name has exhausted its path segments. Keep the
	// first by path order unsuffixed and number the rest, so the outcome is
	// deterministic rather than dependent on map iteration order.
	for name, indices := range groupByName(n, nameAt) {
		if len(indices) == 1 {
			projects[indices[0]].Name = name
			continue
		}
		sort.Slice(indices, func(a, b int) bool {
			return projects[indices[a]].Path < projects[indices[b]].Path
		})
		for k, i := range indices {
			if k == 0 {
				projects[i].Name = name
			} else {
				projects[i].Name = fmt.Sprintf("%s-%d", name, k+1)
			}
		}
	}
}

func groupByName(n int, nameAt func(int) string) map[string][]int {
	groups := make(map[string][]int, n)
	for i := range n {
		name := nameAt(i)
		groups[name] = append(groups[name], i)
	}
	return groups
}

// reversedSegments splits a path into its components, deepest directory first.
func reversedSegments(path string) []string {
	cleaned := filepath.ToSlash(filepath.Clean(path))

	var segments []string
	for _, s := range strings.Split(cleaned, "/") {
		if s != "" && s != "." {
			segments = append(segments, s)
		}
	}
	slicesReverse(segments)
	return segments
}

func slicesReverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// sanitizeSegment reduces a path segment to a lowercase identifier safe to
// pass as a --project argument: letters, digits and single dashes.
//
// Collapsing distinct segments onto the same string (say "My.App" and
// "my-app") is harmless — the caller resolves whatever collisions result.
func sanitizeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	lastWasDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastWasDash = false
		default:
			// Any run of unsupported characters becomes a single dash.
			if !lastWasDash && b.Len() > 0 {
				b.WriteByte('-')
				lastWasDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		// A segment made entirely of punctuation still has to contribute a
		// word. Returning "" instead would join into names like "-api".
		return "project"
	}
	return out
}
