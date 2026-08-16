# Contributing

## Building and testing

```bash
make build     # ./claudius-maximus and a ./cmax symlink
make check     # go vet + go test
make crosscompile
```

CI runs `gofmt -l`, `go vet`, `go test -race` on Linux/macOS/Windows, and a
cross-compile of every supported target. Run those locally before opening a
PR — the pull request template has the checklist.

## Commit style

Commits are small, ordered, and each one builds and passes its own tests on
its own — you should be able to check out any single commit in the history
and get a working, tested state. If a change only makes sense once a later
commit lands, it usually means the two should be one commit, or the earlier
one is not yet complete.

Commit messages are explanatory prose, not [Conventional
Commits](https://www.conventionalcommits.org/). This is deliberate, not an
oversight: a commit message serves the person reading this history later,
explaining *why* a change looks the way it does, including alternatives that
were tried and rejected. That is a different audience and a different
question from what belongs in release notes, which describe *what changed*
for someone using the tool. Conventional Commits optimizes for the second
question at the expense of the first. Release notes here are expected to come
from PR titles/labels and milestones, not from parsing commit messages — see
[DEVELOPMENT.md](./DEVELOPMENT.md) for the reasoning in full.

Practical implications:

- Explain the *why* in the body: the constraint, the alternative that was
  rejected, the thing that broke and how it was found. Don't just restate
  what the diff already shows.
- No prefix convention (`feat:`, `fix:`, ...) is expected or enforced.
- A PR usually carries several commits rather than one squashed one, because
  the history is meant to be read, not just the diff.

## Language

Everything in this repository — code, comments, commit messages, docs — is
English, regardless of what language a discussion around it happens in.

## Where the reasoning lives

[DEVELOPMENT.md](./DEVELOPMENT.md) holds the reasoning that spans more than
one commit: the model the code is built on, constraints discovered by running
the real thing, and what was deferred and why. Read it before proposing a
structural change — it may already explain why the current shape was chosen.
