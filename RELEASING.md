# Releasing

This document is the release process itself: what to do, in order. The
reasoning behind why it looks this way — what was considered and rejected,
what tradeoffs it makes — lives in [DEVELOPMENT.md](./DEVELOPMENT.md).

Versions are [SemVer](https://semver.org/), tagged `vMAJOR.MINOR.PATCH`. The
project is currently in its `0.x` phase: nothing is guaranteed stable yet, and
that is deliberate — see the first milestone.

**Only one milestone targeting `main` is ever open at a time** — the next
release, major or minor depending on what's actually merged since the last
one shipped. Patch/bugfix milestones for older, already-superseded lines are
the exception: several of those can be open in parallel, one per affected
`release/vX.Y` branch, entirely independent of each other and of whatever is
queued for `main`. `milestone-release.yml` enforces the `main` rule: it
refuses to publish if the current draft contains a PR milestoned for a
different release than the one being closed, rather than silently shipping
someone else's not-yet-ready work under the wrong version.

## The short version

1. Plan a release as a GitHub **milestone** named exactly `vMAJOR.MINOR.PATCH`
   (no `v` missing, no suffix). Attach the PRs meant for it.
2. Merge those PRs to `main` as usual. [Release
   Drafter](https://github.com/release-drafter/release-drafter) keeps one
   draft release up to date automatically as they land — open the repo's
   Releases page any time to see it grow.
3. When ready to ship: if you want a release-specific intro, write it into
   the **milestone's own description** (edit the milestone, not the draft
   release). It can be rewritten as many times as you like, whenever you
   like; unlike editing the draft directly, nothing here overwrites it, since
   Release Drafter never touches milestones.
4. Close the milestone. This publishes the release and triggers the build.
   Watch the Actions tab; artifacts appear on the release once it finishes.
   Whatever is in the milestone's description at the moment it closes gets
   prepended to the drafted notes automatically.

That is the whole process for an ordinary release out of `main`. Everything
below is about the case that needs more care: a fix for a version that is no
longer the latest one.

## Backporting a fix to an older line

Scenario: `v0.1.0` shipped, then `v0.2.0` and `v0.3.0` shipped after it, and a
bug is found that affects `v0.1.0`. `main` has moved on; a fix landing there
next ships as part of whatever comes after `v0.3.x`, not as a fix `v0.1.0`
users can get without upgrading past two more minor versions.

**Fix forward first, then backport the exact same commit.** In that order,
always, unless the bug genuinely no longer exists on `main`:

1. Open a normal PR against `main` with the fix, milestoned for whatever
   comes next. Merge it as usual.
2. Create the release branch for the affected line, if it does not exist yet:

   ```bash
   git fetch origin v0.1.0
   git switch -c release/v0.1 v0.1.0
   git push -u origin release/v0.1
   ```

3. Cherry-pick the exact commit merged in step 1 onto a branch off it, and
   open a PR into `release/v0.1` rather than pushing straight to it:

   ```bash
   git switch -c backport/v0.1.1 release/v0.1
   git cherry-pick <the fix commit's SHA>
   git push -u origin backport/v0.1.1
   gh pr create --base release/v0.1 --head backport/v0.1.1
   ```

   A backport is, by definition, something happening after an initial release
   has already shipped, so the same "no commit without a PR" rule that governs
   `main` applies here too — merge it like any other PR once it's reviewed.

4. Create a milestone `v0.1.1` (if one doesn't already exist for it), attach
   the PR from step 3 and whatever issue tracks the bug to it:

   ```bash
   gh pr edit backport/v0.1.1 --milestone v0.1.1
   ```

   (`--milestone` can't be passed at `gh pr create` time in step 3 — the
   milestone doesn't exist yet then.) Merge the PR once it's reviewed, then
   close the milestone.

Closing that milestone is what makes this a backport rather than an ordinary
release: `milestone-release.yml` checks whether a `release/vMAJOR.MINOR`
branch matching the milestone already exists, and if it does, tags and
releases **from that branch**, not from `main`.

Because the fix was already merged to `main` in step 1, the automatic
"merge-up" pull request this creates (`release/v0.1` → `main`) will usually
show no changes — the content is already there. That is expected, not a bug in
the automation; the merge-up PR exists as a safety net for the rarer case
where something on the release branch genuinely was not, or could not be,
applied to `main` the same way.

**If the bug no longer exists on `main`** (already fixed differently, or the
affected code is gone), skip step 1 and open the fix as a PR against the
release branch instead of against `main`. The merge-up PR is then the real
mechanism for deciding whether anything from it is still relevant going
forward — resolve it like any other PR, including closing it unmerged if
genuinely nothing applies.

**Which milestone does the merge-up PR land in**, if `main` has more than one
open milestone (say `v0.4.0` and `v1.0.0` both exist)? Whichever is
SemVer-lowest and still open — deterministic, not a guess dressed up as one.
Reassign it by hand if your actual planning wants it elsewhere; the automation
picks a reasonable default, not a final answer.

## What the automation does and does not do

- `release-drafter.yml` runs on every push to `main` and every PR event,
  keeping one draft current. It never publishes anything on its own.
- `milestone-release.yml` runs when a milestone closes. It validates the
  title, decides `main` vs. an existing `release/vX.Y` branch, promotes the
  matching draft (or creates the release directly if none exists — the normal
  case for a backport, which Release Drafter does not track), prepends the
  milestone's own description to the published notes if one was written, and
  opens the merge-up PR when releasing from a release branch.
- `release.yml` runs when a release is published (by either of the above, or
  by hand) and attaches the build artifacts via GoReleaser. It does not decide
  what gets released — only builds what already has a release object.
- `milestone-version-check.yml` runs daily and on PR label/milestone changes.
  It compares the version bump the milestone targeting `main` is titled for
  against what its attached PRs' labels actually require (mirroring
  release-drafter.yml's category mapping) and fails loudly on a mismatch in
  either direction — titled too low for a breaking-labeled PR that landed in
  it, or still titled for a major release after that label was removed or
  reassigned. It never retitles the milestone; a human decides the actual
  number, same as everywhere else in this process.
- **Nothing here ever creates, renames, or switches which branch is the
  repository's default.** `main` is permanent. This is a deliberate difference
  from tools like `laminas/automatic-releases`, which rotate the default
  branch forward on every release — see DEVELOPMENT.md for why that was
  rejected here.
- Release branches (`release/vX.Y`) are created by a human, only when a
  backport actually becomes necessary — never proactively, and never by CI.

## Retracting a release

Version numbers are never reused, and version tags are never moved or
deleted — a repository ruleset enforces this at the git level, for everyone,
with no bypass. If a shipped release turns out to be bad enough to pull back:

- **Do not delete the tag or the release.** Someone may already have it
  pinned (`go install ...@vX.Y.Z`, a Dockerfile, a lockfile); deleting the tag
  out from under them turns a bad release into a broken build with no
  explanation.
- **Edit the release notes** to say plainly that it's retracted and why, and
  point at the version that fixes it. Marking it a GitHub "pre-release" is
  reasonable extra signal (keeps it off the default "Latest" label) but is not
  a substitute for saying so in the notes themselves.
- **Ship the fix as the next version** — a normal release or backport,
  whichever applies. There is no such thing as re-releasing `vX.Y.Z`; the
  number is burned the moment the tag exists, good or bad.

## Repository settings this process depends on

Merges to `main` and to any `release/vX.Y` branch must go through a pull
request — a repository ruleset blocks direct pushes, force-pushes, and branch
deletion on both, enforced for everyone including admins. (This is why the
repository is public: GitHub does not offer rulesets or classic branch
protection for private repositories on the free plan.)

Merges to `main` must use **"Create a merge commit"** — squash and rebase
merging are disabled for this repository. Squashing a PR into one commit would
erase exactly the property this project's commit history depends on (each
commit builds and tests on its own — see CONTRIBUTING.md), and a real merge
commit is also what lets a PR's origin be found later from `main`'s history if
that's ever needed.
