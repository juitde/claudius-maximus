#!/usr/bin/env python3
"""Picks the milestone a merge-up PR should be attached to.

Separated from the workflow that calls it so the one piece of real logic here
- ordering milestones by SemVer and picking the lowest still-open one - can be
tested directly, the same way this project tests pure parsing logic elsewhere
(see sleep_darwin.go's parseSleepSetting) rather than only through a live
GitHub Actions run nothing here can exercise ahead of time.

Deliberately narrow: unlike laminas/automatic-releases, this never creates or
renames a branch, and never decides "the next line" by branch topology - it
only orders open milestone titles as SemVer and returns the smallest. main
stays main; a release branch is something a human creates by hand, per
RELEASING.md, only when a backport actually becomes necessary.
"""

import re
import sys

SEMVER = re.compile(
    r"^v(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)$"
)


def parse_semver(title):
    """Returns a (major, minor, patch) tuple, or None if title is not a bare
    vMAJOR.MINOR.PATCH milestone title. Pre-release/build metadata is
    deliberately not accepted - milestones name a release, not a variant of
    one."""
    m = SEMVER.match(title.strip())
    if not m:
        return None
    return (int(m["major"]), int(m["minor"]), int(m["patch"]))


def next_milestone(open_titles):
    """Returns the SemVer-lowest of the given open milestone titles, or None
    if none of them parse as vMAJOR.MINOR.PATCH. Titles that do not parse are
    skipped rather than raising - an unrelated milestone (a project board
    used for something else entirely) must not break this."""
    parsed = [(parse_semver(t), t) for t in open_titles]
    parsed = [(v, t) for v, t in parsed if v is not None]
    if not parsed:
        return None
    parsed.sort(key=lambda pair: pair[0])
    return parsed[0][1]


def main(argv):
    # One open milestone title per line on stdin, so the caller (a shell
    # step) never has to teach this script how to talk to GitHub's API - it
    # only does the one thing worth testing in isolation.
    titles = [line.rstrip("\n") for line in sys.stdin if line.strip()]
    result = next_milestone(titles)
    if result is not None:
        print(result)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
