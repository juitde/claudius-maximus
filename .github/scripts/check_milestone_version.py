#!/usr/bin/env python3
"""Checks whether an open milestone's title covers at least the version bump
its attached PRs' labels actually require.

Mirrors release-drafter.yml's category-to-bump mapping (breaking -> major,
feature/improvement -> minor, everything else that isn't excluded -> patch,
no-release-notes -> excluded entirely, same as release-drafter.yml's
pre-exclude) so the two cannot drift apart silently - this script is the one
place that mapping is expressed for this purpose, and release-drafter.yml
should be read alongside it if either ever changes.

Checks both directions: a milestone titled too low for what is attached to it
(a breaking-labeled PR sitting in a milestone titled for a minor release), and
one titled too high for what remains attached (titled for a major release
after its one breaking-labeled PR was removed or reassigned elsewhere).
Deliberately only detects a mismatch and describes it; it never picks a
replacement title or renames anything. This project's stance throughout is
that a human decides the actual version number every time (see
release-drafter.yml's own note on $RESOLVED_VERSION) - either direction of
mismatch is exactly the kind of thing that stance means a human should look
at, not something worth automating away.
"""

import json
import re
import sys

SEMVER = re.compile(
    r"^v(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)$"
)

BUMP_RANK = {"patch": 0, "minor": 1, "major": 2}


def parse_semver(title):
    """Returns a (major, minor, patch) tuple, or None if title is not a bare
    vMAJOR.MINOR.PATCH version."""
    m = SEMVER.match(title.strip())
    if not m:
        return None
    return (int(m["major"]), int(m["minor"]), int(m["patch"]))


def label_bump(labels):
    """The bump one PR's labels require, or None if the PR is excluded from
    release notes (and therefore from version resolution) entirely."""
    if "no-release-notes" in labels:
        return None
    if "breaking" in labels:
        return "major"
    if "feature" in labels or "improvement" in labels:
        return "minor"
    return "patch"  # bug/documentation/unlabeled/anything else: Miscellaneous


def required_bump(pr_label_lists):
    """The highest bump any of the given PRs (each a list of label names)
    requires, or None if every PR is excluded or the list is empty."""
    bumps = [b for b in (label_bump(labels) for labels in pr_label_lists) if b is not None]
    if not bumps:
        return None
    return max(bumps, key=lambda b: BUMP_RANK[b])


def title_bump(base, title):
    """The bump `title` represents over `base` (both (major, minor, patch)
    tuples). Assumes title >= base, which holds by construction for an open
    milestone's planned version relative to the last published one."""
    if title[0] > base[0]:
        return "major"
    if title[1] > base[1]:
        return "minor"
    return "patch"


def check(base_title, milestone_title, pr_label_lists):
    """Returns a human-readable mismatch message, or None if the milestone's
    title already matches what its attached PRs require, or there is nothing
    yet to judge it against (the title doesn't parse, or nothing attached
    contributes a bump - an empty or fully-excluded milestone is a normal
    starting state, not a mismatch)."""
    title = parse_semver(milestone_title)
    if title is None:
        return None
    required = required_bump(pr_label_lists)
    if required is None:
        return None
    base = parse_semver(base_title) or (0, 0, 0)
    actual = title_bump(base, title)
    if BUMP_RANK[required] > BUMP_RANK[actual]:
        return (
            f"Milestone {milestone_title} is titled for a {actual} release, "
            f"but its attached PRs require at least a {required} bump "
            f"(previous release: {base_title})."
        )
    if BUMP_RANK[required] < BUMP_RANK[actual]:
        return (
            f"Milestone {milestone_title} is titled for a {actual} release, "
            f"but its attached PRs only require a {required} bump "
            f"(previous release: {base_title}); consider retitling it lower."
        )
    return None


def main(argv):
    if len(argv) != 3:
        print("usage: check_milestone_version.py <base-title> <milestone-title>", file=sys.stderr)
        return 2
    base_title, milestone_title = argv[1], argv[2]
    # One JSON array of label-name arrays on stdin, e.g. [["feature"], ["bug", "documentation"]] -
    # one entry per PR attached to the milestone, so the caller (a shell step)
    # never has to teach this script how to talk to GitHub's API.
    pr_label_lists = json.load(sys.stdin)
    message = check(base_title, milestone_title, pr_label_lists)
    if message:
        print(message)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
