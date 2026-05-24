# Treely

A terminal UI for managing dev server processes between worktrees of a single project - all from one place.

## How it works

Running `treely` starts a TUI listing your git worktrees. Selecting one activates it: a background daemon starts your configured dev server in the worktree directory. Switching worktrees stops the old server and starts a new one.

The daemon runs detached and persists between TUI sessions. Logs from the dev server go to `~/.treely/daemon.log`.

## Installation

```bash
go install github.com/srimel/treely/cmd/treely@latest
```

> **Note:** The binary is placed in `~/go/bin`. If `treely` isn't found after installing, add `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.) and restart your terminal.

Or download a pre-built binary from the [Releases page](https://github.com/srimel/treely/releases), extract the archive for your platform, and move the binary to a directory on your `PATH`:

```bash
tar -xzf treely_*_linux_amd64.tar.gz
mv treely /usr/local/bin/
```

Or build from source:

```bash
git clone https://github.com/srimel/treely
cd treely
go build ./cmd/treely
```

## Uninstalling

If you installed via `go install`, the binary lives in `$(go env GOPATH)/bin` (usually `~/go/bin`).

1. Stop the daemon and dev server. From inside the TUI press `K` (shift+k), or run:

   ```bash
   pkill -f 'treely --daemon'
   ```

2. Remove the binary:

   ```bash
   rm "$(go env GOPATH)/bin/treely"
   ```

3. Remove Treely's data directory (config, state, logs, socket):

   ```bash
   rm -rf ~/.treely
   ```

## Usage

```bash
treely                              # launch the TUI with saved defaults
treely .                            # use the repo in the current directory
treely /path/to/project             # override project path for this session
treely -c "pnpm dev"                # override startup command for this session
treely /path/to/project -c "pnpm dev"   # override both (flags may go anywhere)
```

Positional paths are resolved against your shell's working directory, so `treely .`, `treely ../other-repo`, and absolute paths all work. Flags and the positional path may appear in any order.

On first run, a setup wizard prompts for your project path and the startup command to run (e.g. `npm run dev`). This configures a single project — the one you'll use most often.

**Using a different project or command:**

Treely stores one project + startup command in its config. You can point it at a different project or change the command for a single session by passing a positional path and/or the `-c` / `--command` flag:

```bash
treely ~/Source/other-project
treely -c "npm run dev:debug"
```

These overrides do not change your saved config — the next plain `treely` invocation will still use your defaults. If the daemon is already running a dev server for a different project, treely will prompt before stopping it and switching.

**Keys:**

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `enter` / `space` | Activate worktree |
| `R` | Restart daemon |
| `q` / `ctrl+c` | Quit TUI (daemon and dev server keep running) |
| `K` _(shift+k)_ | Stop dev server, kill daemon, and quit |

## Development

For contributors building and testing locally:

```bash
go build ./...          # build all packages
go run ./cmd/treely     # run without building
go test ./...           # run all tests
go vet ./...            # static analysis
```

To install locally (macOS/Linux):

```bash
./scripts/install.sh
```

## Requirements

- Go 1.21+
- Git with worktree support

## Docs

- [Architecture](docs/architecture.md)
