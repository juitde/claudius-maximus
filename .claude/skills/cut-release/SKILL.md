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
   (`gh issue list --milestone <title>`, `gh pr list --milestone <title>`).
   Report this plainly — do not assume "closed enough" on the user's behalf.
3. **Show the current draft.** `gh release list --json isDraft,tagName,name`
   to find it, then `gh release view <tag>` for its body. This is what will
   ship if nothing more is added, aside from the milestone's own description
   (see step 5), which the draft itself never shows.
4. **Stop here and present a plan** — the milestone title/version, what is
   still open (if anything), and the current draft's contents — and ask
   explicitly whether to proceed. Do not close the milestone, edit its
   description, or push anything before this confirmation. Closing the
   milestone is itself the trigger that publishes and builds the release; it
   is not a reversible "just checking" action.
5. **If the user wants an intro paragraph added**, draft the wording and show
   it before writing it — this is release-facing prose someone else will
   read, not an internal decision, so it gets the same sign-off as step 4,
   not less. Write it into the **milestone's description**, not the draft:
   `gh api repos/{owner}/{repo}/milestones/{milestone_number} -X PATCH -f description="..."`.
   Unlike editing the draft, this can be redone as many times as needed with
   no race against Release Drafter recomputing the draft body from further PR
   merges — Release Drafter never reads milestones at all.
6. **Only once confirmed:** write the intro to the milestone's description if
   requested, then close the milestone (`gh api ... -X PATCH -f state=closed`
   or via the web UI) to publish. `milestone-release.yml` prepends whatever is
   in the description at that moment to the drafted notes automatically; there
   is nothing further to apply to the draft itself.
7. Report back once `milestone-release.yml` and then `release.yml` have run —
   link the published release and confirm both the intro and the artifacts
   are attached.
