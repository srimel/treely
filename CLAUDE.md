# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build ./...                          # build all packages
go run ./cmd/treely                     # run TUI without building
go run ./cmd/treely -- --daemon         # run daemon in foreground (debug)
go test ./...                           # run all tests
go test ./internal/daemon/...           # run a single package's tests
go test -run TestParseWorktrees ./internal/daemon   # run a single test
go vet ./...                            # static analysis
./scripts/install.sh                    # local install (macOS/Linux)
```

The binary self-forks with `--daemon` on first invocation if no socket exists at `~/.treely/daemon.sock`. To restart the daemon during development, use `treely --restart-daemon` or press `R` in the TUI.

## Architecture

Treely is a **single binary that runs in two modes**: TUI client (default) and daemon (`--daemon`). They communicate over a Unix socket (`~/.treely/daemon.sock`) using **newline-delimited JSON**. The TUI is stateless and reconnects on every invocation; the daemon is detached (`Setsid` on Unix) and owns the child dev server.

### Constraints that span files

- **Deliberate type duplication.** `internal/daemon` and `internal/client` each define their own `Command`, `Event`, and `Worktree` structs with no shared package. Do not introduce a shared types package — the duplication decouples the two sides of the socket so either can evolve independently as long as the JSON wire format is preserved.

- **Wizard result crosses back via type assertion.** `cmd/treely/main.go` runs the first-run wizard as a separate `tea.Program`. After `p.Run()` returns, the final model is type-asserted to `tui.WizardModel` and `wz.Result` is read. This is the only path for wizard output to reach `main`.

- **Active status is computed, not stored.** A worktree is `"active"` only when its path matches `state.ActiveWorktree` **and** `d.proc != nil`. Worktree discovery is always fresh (`git worktree list --porcelain` on every `list` command); there is no cached worktree state.

- **No "crashed" state.** When the child dev server exits for any reason, the goroutine in `daemon.activate` (waiting on `proc.Wait()`) nils `d.proc`, writes an empty `state.yaml`, and pushes a `state_changed` event. Crashed worktrees appear as `"inactive"`.

- **Process group lifecycle.** `StartProcess` runs `sh -c <startup_command>` with `Setpgid: true`. `Stop` signals the entire group with `SIGTERM`, polls for exit, then escalates to `SIGKILL` after 5s — this is the only way to guarantee a dev server's grandchildren release their ports before the next worktree is activated.

- **Single client at a time.** `Server.Accept` closes the previous client whenever a new one connects. Multiple TUI instances can launch, but only the latest receives daemon `Push` events.

- **`-p` flag is in-memory only.** It overrides `cfg.ProjectPath` after load but never writes to `config.yaml`. The daemon always uses the path persisted in `config.yaml` for worktree discovery — `-p` only affects the TUI's view.

- **Bare-repo layout fallback.** `daemon.findGitRoot` checks one level of subdirectories if `ProjectPath` itself is not a git repo, supporting the bare-repo-plus-linked-worktrees layout (e.g. `~/Source/my-app/my-app.git`).

### TUI event loop

`tui.Model.Init` returns a `tea.Batch` of two commands: one that sends `"list"`, and `waitForEvent` which blocks on `<-client.Events`. Every `eventMsg` handler re-arms `waitForEvent`, so the event loop is self-sustaining. A closed `Events` channel produces an `errMsg` that quits the program.

### Files at `~/.treely/`

| File          | Owner                                        |
| ------------- | -------------------------------------------- |
| `config.yaml` | TUI (written once by wizard, read by daemon) |
| `state.yaml`  | Daemon (active worktree path + PID)          |
| `daemon.sock` | Daemon (deleted on clean exit)               |
| `daemon.log`  | Daemon stdout/stderr                         |

### Platform-specific code

Daemon detachment and process group handling are split by build tag: `fork_unix.go` / `fork_windows.go` for `SysProcAttr`, and `process_unix.go` / `process_windows.go` for `StartProcess`/`Stop`. CI matrix runs Go 1.22 and 1.23 on Ubuntu only (see `.github/workflows/ci.yml`); Windows code is build-tag-gated but not exercised in CI.

## See also

- `docs/architecture.md` — fuller diagrams of process lifecycle, IPC, and TUI loop
