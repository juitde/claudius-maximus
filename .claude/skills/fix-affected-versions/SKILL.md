---
name: fix-affected-versions
description: Take a bug report all the way through every still-supported release line it affects - find the range, fix forward on main, then a patch milestone and cherry-pick PR per affected line, merged and released. Use when a bug report needs a full multi-line resolution, not just a single backport onto one already-known line.
---

# Fixing a bug across every affected release line

A meta-skill: it sequences `find-affected-versions`, then repeats the
`backport-fix` flow once per still-supported affected line, plus the
milestone bookkeeping each of those assumes already exists when used alone.
Read RELEASING.md and the three individual skills for what each step relies
on - this page only adds the orchestration across lines and the confirmation
gates that come from doing several at once instead of one.

Use this when a bug report's blast radius across releases isn't known yet.
If the affected line is already known and it's a single line, `backport-fix`
alone is simpler and is what this skill calls internally anyway.

## Steps

1. **Analyze the bug report**: repro steps, the version it was reported
   against. If the report doesn't have a solid repro, get or infer one before
   going further - everything downstream depends on it.
2. **Run `find-affected-versions` for real**: bisect across released tags for
   the affected range, and check whether `main` still has it. Do not assume
   the reported version is the earliest affected one, or that the affected
   range is contiguous - report the actual range(s) found, plural if the bug
   was introduced, fixed, and reintroduced.
3. **If `main` still has the bug**: fix forward first - open a PR against
   `main` as usual, same sign-off as any other change. Wait for it to merge
   before continuing; every cherry-pick below comes from this exact commit.
   If `main` genuinely no longer has it (already fixed differently, or the
   affected code is gone), note that explicitly and skip to step 4 - each
   per-line PR in step 7 then targets its release branch directly, same as
   `backport-fix`'s "bug no longer exists on main" case.
4. **Work out which release lines actually need a patch**: any `vX.Y` line
   inside the affected range that is not the line `main` is currently on,
   and that was actually released (has a tag) - a superseded line that was
   never tagged has no shipped artifact to patch, so it needs nothing.
5. **For each such line**, work out: does `release/vX.Y` already exist? What
   is its current highest patch tag, so the new milestone is
   `vX.Y.(PATCH+1)`?
6. **Stop and present the full plan** before creating anything: every
   affected line, which release branches already exist vs. need creating,
   the milestone number for each, and the exact commit being cherry-picked.
   One confirmation for the whole multi-line plan, not one per line - but
   call out explicitly anything that looks like it needs a second look (an
   unusually old line still being patched, a line inside the range that's
   being skipped and why) so that judgment call is visible before anything
   happens.
7. **Only once confirmed**, for each affected line: create the release
   branch if it doesn't exist yet (from that line's existing release tag),
   cherry-pick the fix onto a new branch off it, and open a PR into
   `release/vX.Y` - never a direct push, per `backport-fix`. Create the
   line's patch milestone if it doesn't already exist, and attach the PR
   (and the original issue) to it.
8. **Report every opened PR together**, grouped by line. Merging is a
   separate action per PR - only merge one after it's been reviewed and
   explicitly confirmed, same as any other PR here.
9. **Once a line's cherry-pick PR is merged**, its milestone is ready to
   close. Present that as its own confirmation (closing publishes the
   release, exactly as in `backport-fix`) - do not close any milestone
   automatically just because its PR merged, and do not batch this
   confirmation across lines the way step 6 batched the initial plan, since
   lines will realistically finish review at different times.
10. **After each closed milestone**, `milestone-release.yml` opens that
    line's merge-up PR into `main`. Once every affected line has shipped,
    collect and report all of them together, noting which are empty (fix
    already present via the forward-fix) vs. carry real changes.

This skill only ever creates PRs and milestones on its own; it never merges a
PR or closes a milestone without a confirmation covering that specific
action, even though step 6 groups the earlier "should this happen at all"
decision into a single gate.
