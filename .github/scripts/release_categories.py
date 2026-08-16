#!/usr/bin/env python3
"""Canonical mapping from a PR's labels to a release-notes category and the
SemVer bump it contributes. The one place this project expresses that
mapping - both check_milestone_version.py (does a milestone's title match
what its PRs require?) and render_milestone_notes.py (what do the notes
actually say?) import from here, so the two cannot silently drift apart the
way a config file and a hand-maintained mirror of it eventually would.

CATEGORIES is walked top to bottom; a PR is assigned to the first category
whose label it carries, not every category that matches - simpler to read
than letting a PR appear in more than one section for the same reason.
"no-release-notes" is checked before anything else and excludes a PR from
both rendering and version resolution entirely; it is not a category itself.
"""

EXCLUDE_LABEL = "no-release-notes"

# (label, title, bump) - the last entry (label=None) is the catch-all for
# anything that matched nothing above it: unlabeled PRs, or ones labeled
# something this mapping doesn't otherwise care about (dependencies, chore).
CATEGORIES = [
    ("breaking", "Breaking Changes", "major"),
    ("feature", "New Features", "minor"),
    ("improvement", "Improvements", "minor"),
    ("bug", "Bugfixes", "patch"),
    ("documentation", "Documentation", "patch"),
    (None, "Miscellaneous", "patch"),
]


def category_for(labels):
    """Returns (title, bump) for the first matching category, or None if
    `labels` contains the exclude label."""
    if EXCLUDE_LABEL in labels:
        return None
    for label, title, bump in CATEGORIES:
        if label is None or label in labels:
            return (title, bump)
    return None  # unreachable: the last entry's label is None and always matches


def label_bump(labels):
    """The bump a single PR's labels require, or None if excluded."""
    result = category_for(labels)
    return result[1] if result else None
