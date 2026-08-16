#!/usr/bin/env python3
"""Run directly with python3 - no test framework dependency for a script this
small. Exits non-zero on the first failure."""

from render_milestone_notes import render


def check_eq(label, got, want):
    if got != want:
        raise SystemExit(f"FAIL {label}: got {got!r}, want {want!r}")
    print(f"ok   {label}")


def pr(number, title, labels):
    return {"number": number, "title": title, "labels": labels}


check_eq("empty PR list renders as no changes", render([]), "## Changes\n\nNo changes.")

check_eq(
    "all PRs excluded renders as no changes",
    render([pr(1, "Internal cleanup", ["no-release-notes"])]),
    "## Changes\n\nNo changes.",
)

check_eq(
    "a single featured PR",
    render([pr(6, "Discover projects and manage remote-control environments", ["feature"])]),
    "## Changes\n\n"
    "### New Features\n\n"
    "- Discover projects and manage remote-control environments (#6)",
)

check_eq(
    "sections render in CATEGORIES order regardless of input order",
    render(
        [
            pr(3, "Fix a race", ["bug"]),
            pr(1, "Add sleep detection", ["feature"]),
            pr(2, "Reject invalid globs", ["breaking"]),
        ]
    ),
    "## Changes\n\n"
    "### Breaking Changes\n\n"
    "- Reject invalid globs (#2)\n\n"
    "### New Features\n\n"
    "- Add sleep detection (#1)\n\n"
    "### Bugfixes\n\n"
    "- Fix a race (#3)",
)

check_eq(
    "unlabeled and excluded PRs mixed with real ones",
    render(
        [
            pr(1, "A feature", ["feature"]),
            pr(2, "Internal-only", ["no-release-notes"]),
            pr(3, "Unlabeled tweak", []),
        ]
    ),
    "## Changes\n\n"
    "### New Features\n\n"
    "- A feature (#1)\n\n"
    "### Miscellaneous\n\n"
    "- Unlabeled tweak (#3)",
)

check_eq(
    "a PR with two matching labels only appears once, under the higher-priority one",
    render([pr(1, "Both a bug and documentation", ["bug", "documentation"])]),
    "## Changes\n\n"
    "### Bugfixes\n\n"
    "- Both a bug and documentation (#1)",
)

print("all checks passed")
