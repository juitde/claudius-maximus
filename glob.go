package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// globstar is the segment that matches any number of intervening directories.
// Go's filepath.Glob does not implement it — there, ** behaves exactly like *
// and stops at the first separator — so patterns containing it are expanded
// here instead.
const globstar = "**"

// GlobExpansion is the outcome of expanding one pattern.
type GlobExpansion struct {
	// Matches are the paths the pattern resolved to, in the order found.
	Matches []string
	// Pruned counts, per directory name, how often descent was stopped. Kept
	// so that a project hidden behind an over-eager prune entry is discoverable
	// rather than simply absent.
	Pruned map[string]int
}

func (e *GlobExpansion) prune(name string) {
	if e.Pruned == nil {
		e.Pruned = map[string]int{}
	}
	e.Pruned[name]++
}

// expandGlobPattern resolves pattern, supporting one ** segment.
//
// Semantics follow the usual globstar convention: ** stands for zero or more
// directory levels, so "~/dev/**/*" matches every directory at any depth below
// ~/dev. A pattern ending in ** is read as "**/*".
//
// pruneDirs names directories that recursion must not descend into, and
// stopDescent decides the same thing per directory (used to stop at a project
// boundary). Both apply only to ** expansion: a pattern that spells out its
// levels explicitly has already said where to look, and silently skipping part
// of it would be surprising. stopDescent may be nil.
//
// A directory that stops descent still matches itself — the rule is "do not
// look inside", not "ignore".
func expandGlobPattern(pattern string, pruneDirs []string, stopDescent func(string) bool) (*GlobExpansion, error) {
	segments := strings.Split(filepath.ToSlash(pattern), "/")

	star := -1
	for i, s := range segments {
		if s != globstar {
			continue
		}
		if star >= 0 {
			return nil, fmt.Errorf("only one %s is supported per pattern", globstar)
		}
		star = i
	}

	if star < 0 {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		return &GlobExpansion{Matches: matches}, nil
	}

	prefix := strings.Join(segments[:star], "/")
	suffix := segments[star+1:]
	if len(suffix) == 0 {
		// A trailing ** means "everything below", i.e. the same as **/*.
		suffix = []string{"*"}
	}

	roots, err := filepath.Glob(rootPath(prefix))
	if err != nil {
		return nil, err
	}

	prune := make(map[string]bool, len(pruneDirs))
	for _, d := range pruneDirs {
		prune[d] = true
	}

	out := &GlobExpansion{}
	for _, root := range roots {
		if err := walkForMatches(root, suffix, prune, stopDescent, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// rootPath turns the segments before the ** back into a path to glob.
func rootPath(prefix string) string {
	switch prefix {
	case "":
		return "." // pattern began with **
	case "/":
		return string(filepath.Separator)
	default:
		return filepath.FromSlash(prefix)
	}
}

func walkForMatches(root string, suffix []string, prune map[string]bool, stopDescent func(string) bool, out *GlobExpansion) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory is not a reason to abandon the whole
			// scan; the rest of the tree is still worth reporting.
			if path == root {
				return nil
			}
			return fs.SkipDir
		}
		if path == root {
			return nil
		}

		// WalkDir reports a symlink as a non-directory. Resolve it so that a
		// symlinked project still counts, but never descend through it —
		// that is where walks find loops.
		if entry.Type()&fs.ModeSymlink != 0 {
			if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
				recordIfMatch(root, path, suffix, out)
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if prune[entry.Name()] {
			out.prune(entry.Name())
			return fs.SkipDir
		}

		recordIfMatch(root, path, suffix, out)

		// Stop at a project boundary. Without this, "~/dev/**/*" walks the
		// entire inside of every repository and reports each sub-package that
		// happens to carry a marker — which is not what "find my projects"
		// means, and costs orders of magnitude more time.
		if stopDescent != nil && stopDescent(path) {
			return fs.SkipDir
		}
		return nil
	})
}

func recordIfMatch(root, path string, suffix []string, out *GlobExpansion) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return
	}
	segments := strings.Split(filepath.ToSlash(rel), "/")
	if len(segments) < len(suffix) {
		return
	}
	// ** absorbs everything up to the trailing segments, so only those have to
	// match: "a/**/x/y" accepts any depth ending in x/y.
	if matchSegments(suffix, segments[len(segments)-len(suffix):]) {
		out.Matches = append(out.Matches, path)
	}
}

func matchSegments(patterns, segments []string) bool {
	for i, p := range patterns {
		ok, err := filepath.Match(p, segments[i])
		if err != nil || !ok {
			return false
		}
	}
	return true
}
