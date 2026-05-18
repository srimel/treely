# Treely

This file provides guidance to agents when working with code in this repository.

## Commands

```bash
go build ./...          # build all packages
go test ./...           # run all tests
go test ./internal/config/...   # run a single package's tests
go vet ./...            # static analysis
go run ./cmd/treely     # run without building
```

## Architecture

Treely is a single binary that runs in two modes: **TUI client** (default) and **daemon** (forked automatically). The binary forks itself with `--daemon` on first invocation if no socket exists.

### Process model

```
treely (TUI mode)  ──── Unix socket (~/.treely/daemon.sock) ────  treely --daemon
                                newline-delimited JSON                    │
                                                                    child dev server
                                                                    (sh -c <startup_command>)
```

The daemon runs detached (new session via `Setsid`) and owns the child dev server process. The TUI is stateless — it reconnects on each `treely` invocation. Daemon logs go to `~/.treely/daemon.log`.

### IPC protocol

Commands sent TUI → daemon: `{"cmd":"list"}`, `{"cmd":"activate","worktree":"/abs/path"}`, `{"cmd":"stop"}`.

The daemon pushes `{"event":"state_changed","worktrees":[...]}` to the connected client whenever state changes (after activate or on child process crash). The TUI calls `client.Send("list")` on init and then blocks on `<-client.Events` in a tea.Cmd loop.

**Key constraint:** `internal/daemon` and `internal/client` deliberately duplicate the `Command`/`Event`/`Worktree` types — they are separate packages with no shared dependency.

### First-run flow

`cmd/treely/main.go` calls `config.Load()`. On `os.IsNotExist`, it runs `tui.WizardModel` as a separate `tea.Program`. After `p.Run()` returns, the final model is type-asserted to `tui.WizardModel` and `wz.Result` is read — this is the only way wizard output crosses back to `main`.

### Worktree discovery

The daemon runs `git -C <project_path> worktree list --porcelain` on every `list` command and parses the output in `daemon.parseWorktrees`. A worktree is marked `"active"` only when its path matches `state.ActiveWorktree` AND `d.proc != nil` (the dev server is actually running).

### Crash handling

When the child dev server exits for any reason, the goroutine in `daemon.activate` detects it via `proc.Wait()`, nils out `d.proc`, writes an empty `state.yaml`, and pushes a `state_changed` event. Crashed worktrees appear as `"inactive"` — no separate crashed state.

### Files at `~/.treely/`

| File          | Owner                                        |
| ------------- | -------------------------------------------- |
| `config.yaml` | TUI (written once by wizard, read by daemon) |
| `state.yaml`  | Daemon (active worktree path + PID)          |
| `daemon.sock` | Daemon (deleted on clean exit)               |
| `daemon.log`  | Daemon stdout/stderr                         |

### `-p` flag

Overrides `cfg.ProjectPath` in memory after config is loaded. Does not write to `config.yaml`. The daemon always uses the path from `config.yaml` for worktree discovery.
