---
name: cut-release
description: Ship an ordinary claudius-maximus release out of main — check the milestone and draft, then close the milestone to publish. Use when the user wants to cut, ship, or release a new version, or asks what's ready to release.
---

# Cutting a release

Full process and reasoning: [RELEASING.md](../../../RELEASING.md). This skill
is the checklist for walking through it, not a replacement for it — read
RELEASING.md's "The short version" section if anything here is unclear.

This is for an **ordinary** release out of `main`. If the target is an older
line that has already been superseded by a newer minor, stop and use the
`backport-fix` skill instead — closing a milestone here would try to release
from `main`, not from the old line.

## Steps

1. **Find the milestone.** `gh api repos/{owner}/{repo}/milestones --jq '.[] | {title,open_issues,closed_issues,due_on}'`
   for open milestones. Confirm its title is a bare `vMAJOR.MINOR.PATCH` — if
   it is not, `milestone-release.yml` will refuse it outright when closed, so
   catch that now rather than after the fact.
2. **Check what is left open** on that milestone
   (`gh issue list --milestone <title>`, `gh pr list --search 'milestone:"<title>"'`
   — `gh pr list` has no `--milestone` flag; filtering goes through
   `--search`'s own qualifier). Report this plainly — do not assume "closed
   enough" on the user's behalf.
3. **Check the title itself is still right for what's attached.** Either
   read the latest `milestone-version-check.yml` run for this milestone, or
   re-run its logic live: does the milestone's title cover at least the
   bump its attached PRs' labels require, and not more than they require
   either? Surface a mismatch here explicitly rather than letting the
   scheduled check catch it later.
4. **Show the current draft.** `gh release view <title>` for its body (the
   draft's tag is the milestone's title itself). This is what will ship if
   nothing more is added, aside from the milestone's own description (see
   step 6), which the draft itself never shows.
5. **Stop here and present a plan** — the milestone title/version, what is
   still open (if anything), the title-vs-PRs check result, and the current
   draft's contents — and ask explicitly whether to proceed. Do not close
   the milestone, edit its description, or push anything before this
   confirmation. Closing the milestone is itself the trigger that publishes
   and builds the release; it is not a reversible "just checking" action.
6. **Actively ask** whether the user wants a release-notes intro/preamble,
   and about any other release-specific wording they'd want included —
   don't wait for them to bring it up. If yes, draft the wording and show it
   before writing it: this is release-facing prose someone else will read,
   not an internal decision, so it gets the same sign-off as step 5, not
   less. Write it into the **milestone's description**, not the draft:
   `gh api repos/{owner}/{repo}/milestones/{milestone_number} -X PATCH -f description="..."`.
   Unlike editing the draft, this can be redone as many times as needed with
   no race against further PR merges recomputing the draft body — nothing
   that recomputes the draft ever reads the milestone.
7. **Only once confirmed:** write the intro to the milestone's description if
   requested, then close the milestone (`gh api ... -X PATCH -f state=closed`
   or via the web UI) to publish. `milestone-release.yml` prepends whatever is
   in the description at that moment to the drafted notes automatically; there
   is nothing further to apply to the draft itself.
8. Report back once `milestone-release.yml` and then `release.yml` have run —
   link the published release and confirm both the intro and the artifacts
   are attached.
