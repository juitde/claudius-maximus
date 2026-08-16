#!/usr/bin/env python3
"""Renders the categorized release notes body for a milestone, from the
list of PRs actually attached to it - not "everything merged since the last
tag" the way release-drafter computed it. release-drafter had no concept of
milestones, so a milestone-scoped computation was never something it could
do; rendering directly from a milestone's own attached PRs also sidesteps
the "no previous published release" bootstrapping problem release-drafter
hit on this project's actual first release, since there is no git-history
comparison involved to have missed a baseline for.

Uses the same category-to-bump mapping as check_milestone_version.py (see
release_categories.py) so the rendered notes and the version-consistency
check can never silently disagree about what a label means.
"""

import json
import sys

from release_categories import CATEGORIES, EXCLUDE_LABEL


def categorize(prs):
    """Groups PRs (each {"number": int, "title": str, "labels": [str]}) by
    category title, in CATEGORIES' order. Each PR appears in exactly one
    category: the first one (in CATEGORIES' order) whose label it carries.
    Excluded PRs are dropped entirely."""
    grouped = {title: [] for _, title, _ in CATEGORIES}
    for pr in prs:
        labels = pr["labels"]
        if EXCLUDE_LABEL in labels:
            continue
        for label, title, _ in CATEGORIES:
            if label is None or label in labels:
                grouped[title].append(pr)
                break
    return grouped


def render(prs):
    """Returns the full '## Changes' markdown body for the given PRs, or a
    plain 'No changes.' notice if every PR was excluded or the list is
    empty."""
    grouped = categorize(prs)
    sections = []
    for _, title, _ in CATEGORIES:
        entries = grouped[title]
        if not entries:
            continue
        lines = [f"- {pr['title']} (#{pr['number']})" for pr in entries]
        sections.append(f"### {title}\n\n" + "\n".join(lines))
    if not sections:
        return "## Changes\n\nNo changes."
    return "## Changes\n\n" + "\n\n".join(sections)


def main(argv):
    # A JSON array of {"number", "title", "labels"} on stdin - one entry per
    # PR attached to the milestone, so the caller (a shell step) never has
    # to teach this script how to talk to GitHub's API.
    prs = json.load(sys.stdin)
    print(render(prs))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
