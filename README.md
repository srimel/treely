# Treely

A terminal UI for managing git worktrees. Switch between worktrees and spin up a dev server in the right directory — all from one place.

## How it works

Running `treely` starts a TUI listing your git worktrees. Selecting one activates it: a background daemon starts your configured dev server in that worktree's directory. Switching worktrees stops the old server and starts a new one.

The daemon runs detached and persists between TUI sessions. Logs from the dev server go to `~/.treely/daemon.log`.

## Installation

```bash
go install github.com/srimel/treely/cmd/treely@latest
```

Or build from source:

```bash
git clone https://github.com/srimel/treely
cd treely
go build ./cmd/treely
```

## Usage

```bash
treely              # launch the TUI
treely -p /path     # override project path for this session
```

On first run, a setup wizard prompts for your project path and the startup command to run (e.g. `npm run dev`).

**Keys:**

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `enter` / `space` | Activate worktree |
| `q` / `ctrl+c` | Quit |

## Requirements

- Go 1.21+
- Git with worktree support

## Docs

- [Architecture](docs/architecture.md)
