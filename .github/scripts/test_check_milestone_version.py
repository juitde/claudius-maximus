#!/usr/bin/env python3
"""Run directly with python3 - no test framework dependency for a script this
small. Exits non-zero on the first failure."""

from check_milestone_version import check, label_bump, required_bump, title_bump


def check_eq(label, got, want):
    if got != want:
        raise SystemExit(f"FAIL {label}: got {got!r}, want {want!r}")
    print(f"ok   {label}")


check_eq("breaking wins over everything else", label_bump(["breaking", "bug"]), "major")
check_eq("feature without breaking", label_bump(["feature"]), "minor")
check_eq("improvement is also a minor", label_bump(["improvement"]), "minor")
check_eq("bug alone is a patch", label_bump(["bug"]), "patch")
check_eq("documentation alone is a patch", label_bump(["documentation"]), "patch")
check_eq("unlabeled falls to patch (Miscellaneous)", label_bump([]), "patch")
check_eq("no-release-notes excludes regardless of other labels", label_bump(["no-release-notes", "breaking"]), None)

check_eq("highest bump wins across PRs", required_bump([["bug"], ["feature"], ["breaking"]]), "major")
check_eq("all-excluded PRs contribute nothing", required_bump([["no-release-notes"]]), None)
check_eq("empty PR list contributes nothing", required_bump([]), None)
check_eq("excluded PRs don't drag down a real one", required_bump([["no-release-notes"], ["feature"]]), "minor")

check_eq("major bump over base", title_bump((0, 1, 0), (1, 0, 0)), "major")
check_eq("minor bump over base", title_bump((0, 1, 0), (0, 2, 0)), "minor")
check_eq("patch bump over base", title_bump((0, 1, 0), (0, 1, 1)), "patch")
check_eq("major component unchanged but minor jumps twice is still minor", title_bump((0, 1, 0), (0, 3, 0)), "minor")

check_eq(
    "titled too low: a breaking PR in a minor-titled milestone",
    check("v0.1.0", "v0.2.0", [["breaking"]]),
    "Milestone v0.2.0 is titled for a minor release, "
    "but its attached PRs require at least a major bump "
    "(previous release: v0.1.0).",
)
check_eq(
    "titled too high: no PR needs more than patch",
    check("v0.1.0", "v0.2.0", [["bug"]]),
    "Milestone v0.2.0 is titled for a minor release, "
    "but its attached PRs only require a patch bump "
    "(previous release: v0.1.0); consider retitling it lower.",
)
check_eq(
    "exact match: minor label, minor title",
    check("v0.1.0", "v0.2.0", [["feature"]]),
    None,
)
check_eq(
    "nothing attached yet: too early to judge",
    check("v0.1.0", "v0.2.0", []),
    None,
)
check_eq(
    "everything attached is excluded: same as nothing attached",
    check("v0.1.0", "v0.2.0", [["no-release-notes"]]),
    None,
)
check_eq(
    "unparseable milestone title is not this script's job",
    check("v0.1.0", "Backlog", [["breaking"]]),
    None,
)
check_eq(
    "missing previous release defaults to v0.0.0",
    check("", "v1.0.0", [["breaking"]]),
    None,
)

print("all checks passed")
