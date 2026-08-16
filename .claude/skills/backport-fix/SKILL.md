---
name: backport-fix
description: Backport a bugfix to an older claudius-maximus release line that a newer minor has already superseded, and cut a patch release for it. Use when the user reports a bug in an old version, or asks to backport, patch, or hotfix a released line.
---

# Backporting a fix

Full process and reasoning: [RELEASING.md](../../../RELEASING.md), section
"Backporting a fix to an older line". This skill is the checklist for walking
through it.

Use this only when the affected version is **not** the latest — if `main` and
the affected release are the same line, this is just an ordinary release; use
the `cut-release` skill instead.

## Steps

1. **Confirm the affected version and find (or note the absence of) a fix on
   `main`.** Is the bug still present there? If yes, fixing forward first is
   the default — see RELEASING.md for why (the merge-up PR this eventually
   creates then does nothing surprising). If the bug genuinely does not exist
   on `main` anymore, say so explicitly and skip to step 3.
2. **If fixing forward:** open a PR against `main` as usual — same process
   as any other change, same sign-off before opening it. Wait for it to be
   merged before continuing.
3. **Check whether `release/vMAJOR.MINOR` already exists**
   (`git ls-remote --heads origin release/vX.Y`). If not, it will need to be
   created from the affected version's tag.
4. **Stop here and present a plan**: the branch to create (or reuse), the
   exact commit to cherry-pick (its SHA and one-line summary), and the patch
   version this will become. Ask explicitly before touching git. Do not
   create the branch, cherry-pick, or open anything before this confirmation.
5. **Only once confirmed:** create the release branch if needed (from the
   affected tag), then cherry-pick onto a new branch off it and open a PR
   into `release/vMAJOR.MINOR` — not a direct push. A backport happens after
   an initial release has already shipped, so the same "no commit without a
   PR" rule already governing `main` applies here too. Report the result —
   including immediately if the cherry-pick conflicts; do not attempt to
   silently resolve a conflict in a fix that is about to ship as a patch
   release without showing what changed.
6. **Create the patch milestone** (`vMAJOR.MINOR.PATCH+1`) if it doesn't
   already exist, and attach the cherry-pick PR and the relevant issue to it.
   If the user wants a note on what this patch fixes beyond the bare PR
   title, write it into the milestone's own description (same mechanism and
   same sign-off as `cut-release`'s step 5) — `milestone-release.yml`
   prepends it to the published notes the same way for a backport as for an
   ordinary release.
7. **Once the cherry-pick PR is reviewed, merge it** (same sign-off as any
   other PR merge), then present the milestone for confirmation the same as
   the `cut-release` skill's step 4 — closing it is what publishes this
   release. **Only once confirmed:** close the milestone.
8. Once `milestone-release.yml` has run, a merge-up PR into `main` will exist.
   Report its URL and whether it shows any real changes or is empty as
   expected. Reviewing/merging that PR is a separate, later action — the
   backport release does not wait on it.
