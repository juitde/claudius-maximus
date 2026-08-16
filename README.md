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
  "project_globs": ["~/dev/*", "~/work/*/*"]
}
```

Then scan them:

```bash
claudius-maximus rescan           # discover projects, assign names
claudius-maximus list-projects    # print the cached list
```

`rescan` keeps every project name as short as it can while staying unique —
`api` stays `api` unless a second `api` shows up, at which point both grow a
parent segment (`dev-api`, `client-a-api`). `list-projects` reads only the
cache and never scans, so every caller sees the same list until the next
`rescan`.

### Which directories count as projects

A matched directory needs at least one marker file. The defaults cover one
canonical root file per ecosystem:

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
| Containers | `Dockerfile`, `docker-compose.y[a]ml`, `compose.yaml` |

This keeps scratch directories out of a broad glob like `~/dev/*` — which
matters for more than tidiness, since every extra entry is another chance for a
name collision, and collisions make names longer for whoever collides.

Markers may be literal names or glob patterns. Override them with
`project_markers`:

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

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDIUS_HOME` | `~/.claudius-maximus` | State directory (config, caches, logs) |

## License

Copyright 2026 JUIT GmbH

Licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE).
