# claudius-maximus

Start, stop and list Claude Code remote-control sessions on your machine —
usable both as an MCP server (from a running orchestrator session) and directly
from the command line.

The point: Claude Code's Remote Control has to be started locally first, so there
is no way to bring up a *new* session from your phone. Dispatch would solve that
but is not available on Team plans. This tool fills that gap — a running
orchestrator session (or a shell) can spawn, track and tear down remote-control
sessions for any project on the machine.

> **Status:** under construction. See [DEVELOPMENT.md](./DEVELOPMENT.md) for the
> design decisions and the current build order.

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

Session management commands land as the build progresses.

## State directory

```
~/.claudius-maximus/
  config.json               yours — edit by hand or through `config`
  state/projects.json       derived from a scan, replaced by every rescan
  state/environments.json   the record of running processes
  state/logs/               output of those processes
```

Everything under `state/` belongs to the tool. `projects.json` can be deleted
at any time and a `rescan` rebuilds it; `environments.json` cannot, since it is
the only record of which multiplexer session belongs to which process.

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDIUS_HOME` | `~/.claudius-maximus` | State directory |
| `CLAUDIUS_URL_PATTERN` | built-in | Regular expression extracting the environment URL from claude's output; needs one capture group for the ID |

## License

Copyright 2026 JUIT GmbH

Licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE).
