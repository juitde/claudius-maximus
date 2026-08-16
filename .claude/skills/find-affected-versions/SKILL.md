---
name: find-affected-versions
description: Determine which released claudius-maximus versions a reported bug actually affects, without assuming the version it was reported against is the earliest one with the problem. Use when triaging a bug report against a specific version, before deciding what to backport.
---

# Finding which versions a bug affects

Purely investigative — this builds and runs historical versions locally, it
never touches git history, branches, milestones, or releases. No
confirmation checkpoint is needed before starting; the checkpoint is what you
do with the result afterward (typically feeding it into the `backport-fix`
skill, which does have one).

Do not assume the reported version is the earliest affected one, or that
affected-ness is monotonic (introduced once, present ever since) without
checking — a regression can be introduced, fixed, and reintroduced. Bisection
is the efficient default, but verify the result makes sense rather than
trusting it blindly.

## Steps

1. **Get a reliable repro.** If the report does not already have exact
   reproduction steps (a config, a command sequence, an expected vs. actual
   result), get or infer them before building anything — testing against
   version after version without a solid repro just produces noise.
2. **List every released tag**, oldest to newest:
   `git tag --sort=version:refname` (or `gh release list` for anything with a
   published GitHub Release). Include `main` at the end as "current."
3. **Build each candidate as you need it**, not all of them upfront:

   ```bash
   git worktree add /tmp/cmax-<tag> <tag>
   (cd /tmp/cmax-<tag> && go build -o /tmp/cmax-<tag>/cmax .)
   ```

   Worktrees rather than repeated checkouts, so you can keep several versions
   built side by side without disturbing the main working copy.
4. **Bisect**: run the repro against the middle untested version. If it
   reproduces, the bug was present at least that far back; check older. If it
   does not, check newer. Narrow until you find the exact version boundary.
5. **Check `main` explicitly**, even if bisection already implies an answer —
   confirming whether the bug is already fixed forward, or still live, is
   exactly the fact `backport-fix`'s step 1 needs.
6. **Sanity-check monotonicity** by spot-checking one or two versions outside
   the range bisection landed on, especially if the affected range looks
   surprising. If the result is inconsistent (e.g. an "unaffected" version
   sits between two "affected" ones), fall back to checking every version in
   the suspect range individually rather than trusting the bisection.
7. **Clean up the worktrees** (`git worktree remove /tmp/cmax-<tag>`) and
   report the result plainly: the affected range, whether `main` still has
   it, and — if more than one still-supported release line falls in that
   range — that a backport may be needed on each of them, not just the one
   originally reported.
