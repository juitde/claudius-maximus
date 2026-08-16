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

```bash
claudius-maximus version
```

More commands land as the build progresses.
