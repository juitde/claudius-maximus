package main

import (
	"fmt"
	"sort"
	"strings"
)

// outputMode is how a command should report its result.
//
// Default is a summary a person can read at a glance. The other two are
// deliberate opt-ins: JSON for machines, verbose for when the summary is not
// enough to explain something.
type outputMode int

const (
	outputSummary outputMode = iota
	outputVerbose
	outputJSON
)

// inlineListLimit is how wide a joined value may be before it is reported as a
// count instead. Long enough for a handful of globs, short enough that a
// 58-entry marker list does not bury the line that matters.
const inlineListLimit = 100

// describeList renders a list either in full or as a count, whichever stays
// readable.
func describeList(values []string) string {
	if len(values) == 0 {
		return "(empty)"
	}
	joined := strings.Join(values, ", ")
	if len(joined) <= inlineListLimit {
		return joined
	}
	return plural(len(values), "entry", "entries")
}

// describeChanges renders a rescan's effect as one self-contained line,
// naming only the categories that actually happened.
//
// The zeroes are exactly the part a reader would have to filter out
// themselves, and the unmodified count carries the total, so no separate
// "n projects" prefix is needed to make the line complete.
func describeChanges(added, removed, renamed, unmodified int) string {
	counts := []struct {
		n    int
		verb string
	}{
		{added, "added"},
		{removed, "removed"},
		{renamed, "renamed"},
		{unmodified, "unmodified"},
	}

	var parts []string
	for _, c := range counts {
		if c.n > 0 {
			parts = append(parts, plural(c.n, "project")+" "+c.verb)
		}
	}
	if len(parts) == 0 {
		return "no projects found"
	}
	return strings.Join(parts, ", ")
}

// columnWidth measures the widest rendered key so that columns line up.
func columnWidth[T any](items []T, render func(T) string) int {
	width := 0
	for _, item := range items {
		width = max(width, len(render(item)))
	}
	return width
}

func printRenames(renames []RenamedProject) {
	if len(renames) == 0 {
		return
	}
	width := columnWidth(renames, func(r RenamedProject) string { return r.OldName })

	fmt.Printf("\n  renamed:\n")
	for _, r := range renames {
		fmt.Printf("    %-*s -> %-*s  %s\n", width, r.OldName, width, r.NewName, shortenPath(r.Path))
	}
}

func printEnvironmentRenames(events []RenameEvent) {
	if len(events) == 0 {
		return
	}
	width := columnWidth(events, func(e RenameEvent) string { return e.OldName })

	fmt.Printf("\n  running environments relabelled:\n")
	for _, e := range events {
		fmt.Printf("    %-*s -> %-*s  %s\n", width, e.OldName, width, e.NewName, shortenPath(e.ProjectPath))
	}
}

func printPruned(pruned map[string]int) {
	if len(pruned) == 0 {
		return
	}

	names := make([]string, 0, len(pruned))
	for name := range pruned {
		names = append(names, name)
	}
	// Most-pruned first: that entry is doing the real work, and it is the one
	// to look at if a project is unexpectedly missing.
	sort.Slice(names, func(i, j int) bool {
		if pruned[names[i]] != pruned[names[j]] {
			return pruned[names[i]] > pruned[names[j]]
		}
		return names[i] < names[j]
	})

	total := 0
	parts := make([]string, len(names))
	for i, name := range names {
		total += pruned[name]
		parts[i] = fmt.Sprintf("%s (%d)", name, pruned[name])
	}
	fmt.Printf("\n  not descended into (%s):\n    %s\n",
		plural(total, "directory", "directories"), strings.Join(parts, ", "))
}

func printProjectList(projects []Project, indent string) {
	width := columnWidth(projects, func(p Project) string { return p.Name })
	for _, p := range projects {
		fmt.Printf("%s%-*s  %s\n", indent, width, p.Name, shortenPath(p.Path))
	}
}
