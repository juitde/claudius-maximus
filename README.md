# claudius-maximus

Start, stop and list Claude Code remote-control sessions on your machine —
usable both as an MCP server (from a running orchestrator session) and directly
from the command line.

The point: Claude Code's Remote Control has to be started locally first, so there
is no way to bring up a *new* session from your phone. Dispatch would solve that
but is not available on Team plans. This tool fills that gap — a running
orchestrator session (or a shell) can spawn, track and tear down remote-control
sessions for any project on the machine.

> **Status:** working, unreleased. Environments can be started, listed and
> stopped from the command line and over MCP. Published binaries are not there
> yet. Attaching from a local terminal (tmux/screen) is deliberately not
> planned — see DEVELOPMENT.md.
>
> [DEVELOPMENT.md](./DEVELOPMENT.md) explains the model the code is built on and
> what running `claude remote-control` for real taught us about it.
> [CONTRIBUTING.md](./CONTRIBUTING.md) has the build/test commands and commit
> style for anyone sending a PR.

## Build

```bash
make build     # produces ./claudius-maximus and a ./cmax symlink
make check     # go vet + go test
```

## Usage

Point the tool at the directories that hold your projects:

```bash
# ~/.claudius-maximus/config.json
{
  "project_globs": ["~/dev/*", "~/work/**/*"]
}
```

Then scan them:

```bash
claudius-maximus rescan           # discover projects, assign names
claudius-maximus list-projects    # print the cached list
claudius-maximus doctor           # check the setup, preview the next rescan
```

`rescan` answers in a single line, naming only the categories that happened:

```
$ claudius-maximus rescan
2 projects added, 1 project removed, 40 projects unmodified

$ claudius-maximus rescan
43 projects unmodified
```

Every reporting command takes `--verbose` for the full detail and `--json` for
the complete machine-readable result. The default is the summary, because that
is what a person asking "did that work?" needs.

### Checking the setup

`doctor` reports on the configuration and previews what a `rescan` would do —
without writing anything:

```
  ✓ project globs      1 pattern configured: ~/dev/*
  ✓ project markers    built-in defaults (58 entries)
  ✓ project cache      29 projects, scanned 5 minutes ago
  ! cache freshness    out of date — 2 to add, 1 to rename

Preflight scan (nothing written)

  30 projects would be cached

  would be added:
    infra    ~/dev/infra

  matched a glob but skipped:
    ~/dev/notes       no project marker
    ~/dev/old-stuff   no project marker (+4 below)
    ~/dev/.DS_Store   not a directory

  3 directories only hold other projects: ~/dev, ~/work, ~/work/acme
```

The skipped list is what to read when a project is missing. Two kinds of noise
are kept out of it so the remainder is worth reading:

- A directory holding discovered projects is not itself a project, and being
  skipped is correct for it. Those are counted on one line instead of listed.
- Once a directory is skipped, everything below it is skipped too. Only the top
  of such a chain is shown, with the rest as `+N below`.

`--json` is unabridged — every skipped directory appears with its reason, plus
`contains_projects` and `covered_by` so a script can regroup differently. Only
a hard failure exits non-zero; warnings describe things you may well have
chosen on purpose.

### Recursive patterns

`**` matches any number of directory levels, so one pattern covers a tree of
uneven depth:

```jsonc
{
  "project_globs": ["~/Documents/Development/**/*"]
}
```

Two rules keep that from walking your whole disk:

**Recursion stops at a project.** Once a directory qualifies, its contents are
not searched. A repository containing sub-packages with their own `go.mod` or
`composer.json` is reported once, as itself — which is what "find my projects"
means. Sub-packages are reachable by naming their level explicitly
(`~/dev/*/services/*`) if you really want them separately.

**Dependency and build trees are skipped.** `node_modules`, `vendor`, `target`,
`build`, `.git` and friends are never descended into. The list is
`prune_directories` and is editable like any other property:

```bash
claudius-maximus config remove prune_directories build   # if you have a project called build
claudius-maximus config set prune_directories            # disable pruning entirely
```

Pruning applies only to `**`. A pattern that spells out its levels has already
said where to look. `doctor` reports which prune entries actually fired, so an
unexpectedly missing project is traceable.

One `**` per pattern; a second one is rejected rather than quietly
misinterpreted. Symlinked directories match but are not descended through.

`rescan` keeps every project name as short as it can while staying unique —
`api` stays `api` unless a second `api` shows up, at which point both grow a
parent segment (`dev-api`, `client-a-api`). `list-projects` reads only the
cache and never scans, so every caller sees the same list until the next
`rescan`.

### Which directories count as projects

A matched directory needs at least one marker. The defaults span three kinds
of evidence — a build manifest, an environment definition, or the fact that
someone opened the directory in an editor:

| | |
|---|---|
| Version control | `.git`, `.hg`, `.svn`, `CLAUDE.md` |
| Go, JS, Rust, PHP | `go.mod`, `package.json`, `deno.json`, `Cargo.toml`, `composer.json` |
| Python | `pyproject.toml`, `setup.py`, `requirements.txt`, `Pipfile` |
| JVM | `pom.xml`, `build.gradle[.kts]`, `settings.gradle[.kts]`, `build.sbt`, `deps.edn`, `project.clj` |
| .NET | `*.sln`, `*.csproj`, `*.fsproj` |
| Ruby, Elixir, Erlang | `Gemfile`, `*.gemspec`, `mix.exs`, `rebar.config` |
| C/C++ and friends | `CMakeLists.txt`, `meson.build`, `configure.ac`, `Makefile`, `Package.swift`, `*.xcodeproj`, `build.zig`, `pubspec.yaml`, `stack.yaml`, `*.cabal` |
| Infrastructure | `*.tf`, `.terraform.lock.hcl`, `terragrunt.hcl`, `ansible.cfg`, `Chart.yaml`, `kustomization.yaml`, `flake.nix` |
| Containers, local envs | `Dockerfile`, `docker-compose.y[a]ml`, `compose.yaml`, `.ddev`, `.devcontainer` |
| Editors and IDEs | `.idea`, `.vscode`, `.vs`, `.fleet`, `.zed`, `.project`, `*.sublime-project` |

That last row does more work than it looks like. Whole ecosystems have no
manifest at all — plain PHP, shell tooling, a documentation tree — and there
the editor directory is the only durable evidence that someone treats this as a
project. It is also the most reliable signal there is, because it records an
explicit human decision rather than an inferred convention.

Some names appear both here and in `prune_directories`, which is not a
contradiction: as a marker `.idea` means "the directory containing it is a
project", as a prune entry it means "do not walk around inside it". Together
they are exactly right.

Filtering keeps scratch directories out of a broad glob like `~/dev/*` — which
matters for more than tidiness, since every extra entry is another chance for a
name collision, and collisions make names longer for whoever collides.

Markers may be literal names or glob patterns. Edit them with `config`:

```bash
claudius-maximus config schema                      # what can be set, and what it means
claudius-maximus config add project_markers Justfile
claudius-maximus config remove project_markers Makefile
claudius-maximus config set project_markers         # no values: turn filtering off
claudius-maximus config unset project_markers       # back to the built-in defaults
```

Values are checked against the schema before anything is written — an unknown
property, a malformed glob or a marker containing a path separator is rejected
with the config left untouched.

Note the difference between the last two. `set` with no values writes an empty
list, meaning "accept every matched directory". `unset` removes the property so
the built-in defaults apply again. And because `add` and `remove` operate on
what is currently in force, adding one marker to an unset property writes out
the full default set first — the command says so when it happens.

Or edit `config.json` by hand:

```jsonc
{
  "project_globs": ["~/dev/*"],
  "project_markers": ["go.mod", "*.tf"]   // only these count, defaults do not merge in
}
```

```jsonc
{
  "project_globs": ["~/work/acme/*"],
  "project_markers": []   // no filtering — every matched directory counts
}
```

The empty list is worth knowing about: markers are a convenience filter, not a
requirement. This tool runs `claude` in the directory, and `claude` runs
anywhere. If your globs are already precise, say so rather than adding a marker
file just to become visible.

## Running environments

```bash
claudius-maximus start --project api    # or --path ~/some/directory
claudius-maximus list
claudius-maximus stop  --project api
```

```
$ claudius-maximus start --project api
Started api
  https://claude.ai/code?environment=env_01LB1RYiukoQKoCU4JLEVnWA
```

Open that link on your phone, or in the Claude mobile app, and create sessions
inside it.

One process serves one directory — claude calls it an environment and hosts up
to 32 sessions in it. It also keeps the same environment per directory across
restarts, so starting twice reconnects rather than duplicating:

```
$ claudius-maximus start --project api
Already running: api
  https://claude.ai/code?environment=env_01LB1RYiukoQKoCU4JLEVnWA
```

`--path` takes any directory, whether or not discovery found it. Marker
filtering curates the project list; it does not decide where claude may run.

`--spawn same-dir|worktree` overrides `spawn_mode` for a single start. See
`config schema` for what the two mean, and why `same-dir` is the default.

## As an MCP server

```bash
claudius-maximus install      # registers with Claude Code
claudius-maximus doctor       # confirms the registration
```

From an orchestrator session, five tools are then available:

| Tool | Purpose |
|---|---|
| `list_projects` | what can be started |
| `rescan_projects` | refresh that list |
| `start_environment` | start one, returns the URL to open |
| `list_environments` | what is running, with URLs |
| `stop_environment` | stop one |

`install` registers `claudius-maximus mcp`, which is the protocol server.
Running the binary bare prints usage instead — being explicit means a human who
types the command name gets help rather than a process silently waiting on
stdin, and a host configured without the subcommand fails immediately rather
than mixing usage text into the protocol stream.

`uninstall` removes the registration, and refuses while environments are
running unless given `--force`: unregistering leaves them running but no longer
reachable, which should not happen silently.

Deliberately not exposed as MCP tools: `install` and `uninstall`. Registering
this server from inside itself would require it to already be registered and
running.

`install --force` replaces an existing registration under the same name and
scope instead of failing — useful after this binary has moved (a Homebrew
upgrade, a rebuild elsewhere). `claude mcp add` has no overwrite of its own; it
refuses a duplicate name outright, so `--force` removes the old entry first and
ignores the "nothing to remove" case, then adds fresh.

## Staying reachable

Two separate ways this machine can stop being reachable from a phone: it falls
asleep, or nothing is running that could spawn a new environment for you.
Neither is solved by installing something on your behalf — both are one-line
recipes below, adapted to your platform.

### Preventing sleep

A sleeping machine suspends every running environment along with it. For
Remote Control specifically, sleep is not a pause — the process loses its
network connection to the phone.

**macOS** — `doctor` checks this directly (see below), because `pmset -g`
resolves the whole question, including any active override, into one line.
Prevent it for a while with:

```bash
caffeinate -s &
```

or set sleep to *Never* in System Settings → Battery for something permanent.

**Linux** (systemd-based distros) — prevent it for a while with:

```bash
systemd-inhibit --what=sleep --why="claudius-maximus environments" sleep infinity &
```

There is no `doctor` check here on purpose. Detecting whether something is
*already* inhibiting sleep would mean parsing `systemd-inhibit --list`, whose
exact output format has no citable documentation to build against — the kind
of guess that has already cost this project a rewrite once (see
DEVELOPMENT.md's URL-pattern story). And the *configured* idle timeout has no
portable answer at all: GNOME and KDE each manage it themselves and bypass
`logind`'s own `IdleActionSec`, confirmed by their own bug trackers. Check your
desktop environment's power settings if this machine sleeps unexpectedly.

**Windows** — Settings → System → Power & battery → Screen and sleep, set
sleep to *Never*. There is no built-in command-line equivalent of `caffeinate`;
`powercfg /requests` reports active power requests but does not create one, and
its exact output for "nothing is active" is not documented anywhere citable
either. A small dedicated utility (search for "Caffeine" or "Insomnia" style
tools) is the practical option if you need this from a script.

### Keeping an orchestrator environment available

`start` is idempotent — reconnecting to an existing environment rather than
duplicating it costs nothing (see "Running environments" above) — so the way
to always have *something* available to spawn new project environments from is
just: run `start` against one real project, regularly. No new concept, no
fabricated "meta" directory; pick any project you already have and designate it
your standing jumping-off point.

**macOS**, a launchd user agent — save as
`~/Library/LaunchAgents/com.example.claudius-maximus.orchestrator.plist` and
load with `launchctl load <path>`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.example.claudius-maximus.orchestrator</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/claudius-maximus</string>
    <string>start</string>
    <string>--project</string>
    <string>orchestrator</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>StartInterval</key>
  <integer>300</integer>
</dict>
</plist>
```

**Linux**, a systemd user timer — save the two files below under
`~/.config/systemd/user/`, then `systemctl --user enable --now
claudius-maximus-orchestrator.timer`. Also run `loginctl enable-linger $USER`
once, or the timer stops the moment you log out — which, for a machine you
reach by *not* being logged in, defeats the point.

`claudius-maximus-orchestrator.service`:

```ini
[Unit]
Description=Ensure the claudius-maximus orchestrator environment is running

[Service]
Type=oneshot
ExecStart=/usr/local/bin/claudius-maximus start --project orchestrator
```

`claudius-maximus-orchestrator.timer`:

```ini
[Unit]
Description=Periodically ensure the claudius-maximus orchestrator environment is running

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min

[Install]
WantedBy=timers.target
```

**Windows**, two Scheduled Tasks (one schedule type per `schtasks /create`
call, so it takes two — one for login, one for periodic re-checks):

```bat
schtasks /create /tn "ClaudiusMaximusOrchestrator" /sc onlogon /rl highest ^
  /tr "C:\path\to\claudius-maximus.exe start --project orchestrator"
schtasks /create /tn "ClaudiusMaximusOrchestratorRecheck" /sc minute /mo 5 ^
  /tr "C:\path\to\claudius-maximus.exe start --project orchestrator"
```

Every recipe above just calls `start` on a schedule; adjust the project name
and binary path for your setup.


## State directory

```
~/.claudius-maximus/
  config.json               yours — edit by hand or through `config`
  state/projects.json       derived from a scan, replaced by every rescan
  state/environments.json   the record of running processes
  state/logs/               output of those processes
```

Files under `state/` carry a `schema_version`. A file written by a newer build
is refused with a message saying to update, rather than being misread.

Everything under `state/` belongs to the tool. `projects.json` can be deleted
at any time and a `rescan` rebuilds it; `environments.json` cannot, since it is
the only record of which process belongs to which project.

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDIUS_HOME` | `~/.claudius-maximus` | State directory |
| `CLAUDIUS_URL_PATTERN` | built-in | Regular expression extracting the environment URL from claude's output; needs one capture group for the ID |

## License

Copyright 2026 JUIT GmbH

Licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE).
