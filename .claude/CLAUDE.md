# claudius-maximus

Starts, stops and lists Claude Code remote-control environments — so a new
session can be started on a project from a phone, without having been at the
machine first.

AI is used here as a development tool in support of the people building this
project, not as an unsupervised author. Every change is reviewed and owned by
a human before it ships, and anything this project publishes (release notes,
docs, commit messages) is expected to read as human-curated rather than as AI
output. Concretely: no em-dashes in prose meant for outside readers (release
notes, README, announcements). Use periods, commas or colons instead.

- **[DEVELOPMENT.md](../DEVELOPMENT.md)** — the model this is built on, what
  running `claude remote-control` for real revealed, and what was deferred and
  why. Read this before proposing a structural change.
- **[CONTRIBUTING.md](../CONTRIBUTING.md)** — build/test commands and the
  commit style (small, ordered, each one green on its own; explanatory
  messages, not Conventional Commits).
- **[RELEASING.md](../RELEASING.md)** — the release process: SemVer
  milestones, how the milestone-draft and milestone-close automation fit
  together, and how to backport a fix to an older line. Use the
  `cut-release`, `backport-fix`, `find-affected-versions` and
  `fix-affected-versions` skills to walk through it rather than re-deriving
  the steps from memory each time; the last one is the meta-skill that drives
  a bug report through all of the others when its full blast radius across
  release lines isn't known yet.

Releasing and backporting always present a plan and wait for explicit
confirmation before closing a milestone, cherry-picking, or pushing a release
branch — see the skills themselves for exactly where that checkpoint sits.

Do not duplicate any of the above files' content here; keep this page short
and let it point rather than repeat.
