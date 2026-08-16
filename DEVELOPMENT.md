# Development notes

This document holds the reasoning that spans more than one change: the model the
code is built on, the constraints it was shaped by, and the things that were
tried and rejected.

It is deliberately not a changelog. Why any single change looks the way it does
belongs in that change's commit message, and that is where it is. What follows
is what no individual commit could tell you.

## The problem

Claude Code's Remote Control has to be started locally before it can be reached
from anywhere else. There is no way to bring up a *new* session from a phone
without first having been at the machine. Dispatch solves exactly that, but is
not available on Team plans.

This tool fills the gap: a running orchestrator session, or a shell, can start,
list and stop remote-control environments for any project on the machine. The
useful consequence is that one session on a laptop is enough to reach every
project on it from a phone.

## What running the real thing taught us

The original design was written without ever executing `claude remote-control`.
Running it once, against claude 2.1.233, invalidated three of its assumptions.
These findings are the most expensive knowledge in this repository — each one
costs a live session against a real account to re-derive — so they are recorded
here rather than left implicit in the code.

**The session URL carries its identifier as a query parameter, not a path
segment.**

```
https://claude.ai/code?environment=env_01LB1RY…
```

The original pattern expected `claude.ai/code/<id>` and therefore never matched.
Every start would have waited out its timeout and failed. Nothing else in the
tool could have worked.

**The same output contains a decoy URL.** Alongside the environment link,
claude prints `https://code.claude.com/docs/en/remote-control`. Any relaxed
"find a URL" match reports success while the process is still sitting at a
prompt, which is precisely what happened during the investigation before the
pattern was anchored.

**On a terminal, the first run blocks on an interactive prompt** asking whether
sessions should share the project directory or each get a git worktree. Without
a terminal it defaults silently. That combination is the worst possible one:
the prompt appears only where a terminal is attached but nothing is watching
it — a tmux or screen pane the caller happens to be running this from, for
instance. `--spawn` is therefore always passed explicitly.

Two smaller findings shaped details, discovered while running the manual test
inside a tmux pane during this investigation. The output is a redrawing TUI
that emits cursor-movement escapes even when redirected to a file, so logs
accumulate repeated frames and any excerpt shown to a user has to be stripped
of control sequences. And the URL line is 107 characters, so it wraps and the
identifier splits across lines at a terminal's default 80 columns — which is
why that manual test had to widen the pane to see the URL intact.

## The model: environments, not sessions

A `claude remote-control` process reports:

```
Capacity: 0/32 · New sessions will be created in the current directory
```

One process is an **environment** bound to a directory, inside which the mobile
app creates sessions. It is not itself a session. Modelling it as one — as the
original design did — gets the count wrong in both directions: one record for
something that holds many, and no record for the thing that actually exists.

**The project path is the identity.** Two independent runs in one directory
returned the identical `env_…`, and on shutdown claude prints "Environment
preserved. Restart `claude remote-control` to reconnect existing sessions." The
environment is derived from the directory and outlives the process. A
self-issued session ID would invent a distinction that does not exist.

Three behaviours follow directly, and none of them are choices:

- Starting a project that already has a live environment returns that
  environment rather than spawning a second process. It is reported with
  `already_running`, not treated as an error.
- The registry stores at most one record per path, so storing is an upsert.
  Appending would leave two records claiming one environment, the older holding
  a dead PID.
- A record whose process has died — after a reboot, or a kill from outside — is
  discarded and replaced rather than reported as running.

## One implementation, two front ends

`Service` holds all behaviour. The CLI and the MCP tools call nothing but its
methods and make no decisions of their own: no validation, no error
special-casing, no business rules.

This is what makes the two interfaces behave *identically* rather than merely
similarly. They are not two implementations kept in step by discipline; they are
one implementation reached two ways. When a rule changes, it changes once.

The exceptions are `install` and `uninstall`, which are CLI-only. An MCP tool
that registers its own server would have to be running, and therefore already
registered, in order to register itself.

`install --force` exists because `claude mcp add` has no overwrite of its
own — confirmed against the real command, not assumed — and errors outright on
a duplicate name and scope. `--force` removes any existing registration first,
ignoring the "nothing to remove" case, then adds fresh. That sidesteps needing
to detect *why* `add` failed from its error text, which would be the same kind
of undocumented-output guess this document argues against elsewhere; removing
unconditionally needs no such guess.

## Discovery: curation, not correctness

A directory qualifies as a project if it carries one of a set of marker files.
The set spans three kinds of evidence — a build manifest (`go.mod`,
`composer.json`), an environment definition (`.ddev`, `Dockerfile`), or the fact
that someone opened it in an editor (`.idea`, `.vscode`).

That third kind matters more than it looks. Whole ecosystems have no manifest —
plain PHP, shell tooling, a documentation tree — and editor metadata is the only
durable evidence that someone treats the directory as a project. It is also the
most reliable signal available, because it records an explicit human decision
rather than an inferred convention.

**But markers are curation, not a ruling.** This tool runs `claude` in a
directory, and `claude` runs anywhere; there is no technical requirement for a
repository. So the filter is fully overridable, and `--path` accepts any
directory whether discovery found it or not. A second gate behind the filter
would contradict the reason the filter is soft.

Filtering earns its place for a reason beyond tidiness: every extra directory in
the list is another chance for a name collision, and every collision makes names
longer for whoever collides. Noise costs everyone.

### Recursive patterns

Go's `filepath.Glob` has no globstar — `**` there behaves exactly like `*` and
stops at the first separator. A pattern like `~/dev/**/*` therefore matched two
levels and quietly returned too few projects, and because it matched *something*
no warning fired. `**` is implemented here rather than documented away.

Unbounded recursion needs two limits, and the first is the one that matters:

- **Recursion stops at a project.** Without it, `~/dev/**/*` descends into every
  repository and reports each sub-package that happens to carry a marker.
  Measured against a real tree: 338 results in 11 seconds, against 42 in 0.28
  seconds with the rule in place.
- **Dependency and build trees are skipped**, via a configurable list.

Both apply only to `**`. A pattern that spells out its levels has already said
where to look.

## Configuration

List properties have three states, using JSON's distinction between an absent
key and an empty array: absent means the defaults apply, a list replaces them,
and an empty list disables the feature. The last state is not decoration — for
`project_markers`, "no filtering" and "the default filtering" are genuinely
different things a user might want.

Keeping that distinction alive forced two decisions. `omitempty` cannot be used,
because `encoding/json` treats an empty slice as empty regardless of whether it
is nil. And `Config` marshals through a map, so an unset property is absent from
the file rather than written as `null` — which also keeps the stored file and
the printed output identical.

The schema is derived from the `Config` struct by reflection; only prose and
validation rules are written by hand. A test asserts that the two describe the
same properties in both directions, so adding a field without describing it
fails the build rather than silently producing an undocumented property.

## State on disk

```
config.json               the user's, edited by hand or through `config`
state/projects.json       derived from a scan, replaced by every rescan
state/environments.json   the record of running processes
state/logs/               output of those processes
```

The split says who owns what. Hiding the derived files behind a leading dot was
the alternative and is worse on both counts: it does not stop anyone editing
them, and it hides them from whoever later needs to clear one out.

The two kinds differ in what a hand edit costs. `projects.json` is rebuilt by
the next rescan. `environments.json` is the only record of which process belongs
to which project — delete an entry and a running claude becomes an orphan this
tool can neither find nor stop.

Both carry a `schema_version`. It was added before any release, because that
window does not reopen: whatever format the first release writes becomes one
that must be migrated from forever, and a migration that has to guess which
build wrote a file is a migration that breaks. `environments.json` is an object
wrapping a list rather than a bare array for the same reason — an array has
nowhere to put a version.

## Output

Three modes, meaning the same thing everywhere: a summary by default, `--verbose`
for full human-readable detail, `--json` for machines. Combining `--json` with
`--verbose` is rejected rather than silently ignored, since `--json` is already
complete.

`rescan` answers in one line, naming only the categories that happened — "2
projects added, 40 projects unmodified" rather than padding it with zeroes the
reader has to filter out. It deliberately does not list which projects changed,
even though that is the obvious next question: on a first run every project is
an addition, so answering it unprompted reproduces the wall of output the
summary exists to remove.

`doctor` previews what a rescan would do without performing one, and separates
the directories that hold other projects from those that qualified for nothing.
On a real tree that turned 23 lines into 3 that carry information.

## Rejected: tmux and screen support

An earlier iteration of this document deferred multiplexer support rather than
rejecting it, and the registry briefly carried a `Multiplexer` field toward
that end. Looking at what it would actually buy changed that.

Once `claude remote-control` is connected, a terminal attached to it shows
almost nothing — the session itself runs server-side, reached through the
URL/app, not in that pane. The one interaction a multiplexer session would
still offer is scanning the QR code, which needs a real terminal on the
process. But that case gains nothing from this tool either: it only arises
when sitting at the machine, where running `claude remote-control` directly
already works. The tool's whole point is reaching a project from somewhere
other than the machine it runs on, so the one case multiplexer support would
serve is exactly the case that does not need it.

Kept minimal instead: no `Multiplexer` field, no dispatch, one spawn path.

## Stopping an environment from inside itself

`stop_environment` never needed a way to resolve "myself" — a session already
knows its own directory and simply passes it, the same as stopping anything
else. The original design's self-identification (an env var, a PID fallback)
solved a question that does not arise here and was left out.

Something else does arise, though. If this server is registered at user scope,
a session running inside an environment can reach the same MCP server and ask
it to stop that very environment. The `claudius-maximus mcp` process handling
that call was spawned as a child of the environment's own `claude` process, and
inherits its process group — the same group `terminateProcess` signals as a
whole (see "One implementation, two front ends" — every stop goes through the
one code path, CLI or MCP, self-directed or not). Killing that group in-line
would signal the MCP server process too, with no guarantee its JSON-RPC
response reaches the client before it dies.

The fix is the delayed-kill helper the original design already had, minus the
self-identification half it no longer needs: `stopEnvironment` spawns a
detached copy of this binary with a hidden `__delayed-kill <pid> <delay>`
subcommand, which sleeps briefly and only then sends the signal. The response
gets a head start before anything dies. It applies to every stop, not only a
self-directed one, because there is no cheap, reliable way to tell in advance
whether this call happens to be one — and delaying a kill that did not need
delaying costs nothing anyone would notice.

## Deferred, and why

- **Cross-process file locking.** The mutex guards one process. Two processes
  writing the same state file concurrently are not protected. The window is
  small, the writes are rare, and the atomic replace means a loser overwrites
  rather than corrupts.
- **`self-update`.** Needs published release artifacts to exist first. Its
  migration half, however, constrains decisions now — which is why the state
  files are versioned already.

## How this is verified

Every commit builds, vets and tests on its own; the history is checked by
checking out each commit in turn, not by trusting that it does.

Tests run on Linux and macOS, because the parts most likely to break are the
ones that differ — process groups, signal delivery, temp paths behind symlinks
— and a single runner exercises none of that difference. Cross-compilation is a
separate job, because build-tag mistakes in the platform-specific process
handling only surface when the other platforms are actually built.

Two bugs found this way are worth recording, because neither would have survived
contact with a mock:

**Zombies read as alive.** The spawn released its child rather than reaping it,
which looks correct — the process must outlive its parent, and it does either
way. But an unreaped child that terminates becomes a zombie, and a zombie
answers signal 0, so every liveness check reported dead environments as running
for as long as the parent lived. In the MCP server, that is forever.

**A test that described the machine.** The doctor tests asserted that nothing
failed, and the checks added alongside the MCP server run the claude binary — so
they passed where Claude Code was installed and failed on a runner without it.
CI caught it on its first run, before any release. The fix was a stub, and the
lesson is that "passes on my machine" and "tests this code" are different
claims.
