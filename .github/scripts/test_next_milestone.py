#!/usr/bin/env python3
"""Run directly with python3 - no test framework dependency for a script this
small. Exits non-zero on the first failure."""

from next_milestone import parse_semver, next_milestone


def check(label, got, want):
    if got != want:
        raise SystemExit(f"FAIL {label}: got {got!r}, want {want!r}")
    print(f"ok   {label}")


check("parses a plain version", parse_semver("v0.1.0"), (0, 1, 0))
check("parses double digits", parse_semver("v1.12.3"), (1, 12, 3))
check("rejects missing v prefix", parse_semver("0.1.0"), None)
check("rejects a pre-release suffix", parse_semver("v0.1.0-rc1"), None)
check("rejects a non-version title", parse_semver("Backlog"), None)
check("rejects trailing garbage", parse_semver("v0.1.0 "), (0, 1, 0))  # stripped
check("rejects leading zeros", parse_semver("v0.01.0"), None)

check(
    "picks the lowest open version",
    next_milestone(["v0.3.0", "v0.1.1", "v0.2.0"]),
    "v0.1.1",
)
check(
    "a patch outranks a later minor",
    next_milestone(["v1.0.0", "v0.9.1"]),
    "v0.9.1",
)
check("skips titles that are not versions", next_milestone(["Backlog", "v0.2.0"]), "v0.2.0")
check("returns nothing when nothing parses", next_milestone(["Backlog", "Icebox"]), None)
check("returns nothing for an empty list", next_milestone([]), None)
check("a single candidate is returned as-is", next_milestone(["v2.0.0"]), "v2.0.0")

print("all checks passed")
