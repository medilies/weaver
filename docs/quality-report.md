# weaver quality report

Review of 2026-11-12, of the whole codebase on `main` at `3ac4ac3`, tagged v0.6.0. Numbers continue from the review of 2026-11-06, whose findings were fixed or moved to `docs/backlog.md` (`git log -- docs/quality-report.md` has them). What the backlog already lists isn't repeated here.

`go build ./...`, `go vet ./...`, `go test ./...` and `golangci-lint run ./...` all pass with 0 issues, `go test -race` passes for `domain`, `app` and `cmd/weave`, and `go mod tidy -diff` changes nothing.

Line numbers refer to `3ac4ac3`. Findings marked **Tested** were reproduced on a copy of the repo: with scratch tests against private tmux 3.4 servers, or with the built `weave` and a throwaway `XDG_CONFIG_HOME`. The rest were checked by reading the code.

## Triage

Severity is High, Med or Low, and fix size XS, S or M, as in the backlog. Each item is **Open** until triaged.

| # | Finding | Severity | Fix | Status |
| --- | --- | --- | --- | --- |
| 17 | A task is typed into a command that replaced its pane's shell with `exec` | Med | S | **Fixed** |
| 18 | Invalid settings also stop `--help`, `--version` and completion | Low | S | **Fixed** |
| 19 | A note line holding a newline breaks the drawing | Low | XS | **Fixed** |
| 20 | A config named `<name>.yaml` is skipped without a warning | Low | XS | **Fixed** |
| — | Three stale comments and docs | Low | XS | **Fixed** in `16050e8`, two of them; the third isn't named |
| — | Binding under a registered name asks twice | Low | S | **Fixed** in `4a85158` |

## Bugs and edge cases

Most severe first.

### 17. A task is typed into a command that replaced its pane's shell with `exec`

**Tested.**

**Where:** `internal/session/infra/tmux.go:437` (`Busy`), against `idleShells` (`:647`).

**Problem:** `Busy` finds a pane's shell idle when the pane's process leads its terminal's foreground process group. A command run with `exec` keeps the shell's process ID, so it leads that group too, and the pane counts as idle while the command runs. `Idle` doesn't make that mistake: `idleShells` also checks, by its name, that the process is still the shell. `docs/backlog.md:57` and `:344` say `Busy` and `Idle` share that check; they don't.

**Reproduced:** a pane with `command: exec cat`. Once it runs `cat`, `Busy` returns false, and `Type` sends `C-e C-u` and the task's command into `cat`, which echoes it back.

**Impact:** a pane runs `exec npm run dev` and offers a `migrate` task. Running the task from the task list types its command into npm, where it never runs, and puts the user in that pane. By `RunningCommands`, the task list would then mark `migrate` with ● as still running, since the pane isn't idle. In a pane running `exec psql` or `exec vim`, the text would be read as that program's input. `RerunCommand` checks `Busy` too, but the browser offers `r` only on a pane with the exited-command warning, which `Idle` already keeps from such panes.

**Fix:** after the copy-mode check, ask `idleShells` about the pane's process, and find the pane busy unless ps reports it idle:

```go
 idle, seen, err := t.idleShells(project, []string{pid})
 if err != nil {
  return false, err
 }
 return !seen[pid] || !idle[pid], nil
```

`Busy`'s comment already says this: busy when a job other than its shell is in the foreground. A pane ps can't see is then asked about rather than typed into, as `Busy` already does when ps fails, and so is a pane whose shell's name is longer than the 15 characters Linux's ps prints, which `Idle` never finds idle either (`shellNames`, `:678`). With this change, the test below passes, and so do `go test ./...` and `golangci-lint run ./...`; at `3ac4ac3` it fails with `got busy false`:

```go
func TestTmuxBusyAfterExec(t *testing.T) {
 server := newTmuxServer(t)
 plan := planOf(t, "execd", fmt.Sprintf(`{"root": %q, "windows": [
  {"name": "srv", "panes": [{"command": "exec cat"}]}
 ]}`, t.TempDir()))
 if err := server.tmux().Start(plan); err != nil {
  t.Fatalf("unexpected error: %v", err)
 }
 deadline := time.Now().Add(5 * time.Second)
 for server.query(t, "display-message", "-p", "-t", "=execd:srv", "#{pane_current_command}") != "cat" {
  if time.Now().After(deadline) {
   t.Fatal("the pane never ran cat")
  }
  time.Sleep(50 * time.Millisecond)
 }
 busy, err := server.tmux().Busy(plan.Name(), plan.Windows()[0], 0)
 if err != nil || !busy {
  t.Fatalf("got busy %t (err %v), want busy", busy, err)
 }
}
```

### 18. Invalid settings also stop `--help`, `--version` and completion

**Tested.**

**Where:** `cmd/weave/main.go:39`: `wire` loads the settings before cobra reads the arguments.

**Problem:** `docs/settings.md:18` says weave refuses to start while `settings.yml` is invalid, and scenario 16 of `docs/agents/terminal-testing.md` checks it with `weave list`. Since `wire` runs first, the refusal also stops what never uses the settings: the help, the version, `weave completion` and Tab completion.

**Reproduced:** with `keys: { remove: u }` in `settings.yml`, `weave --version`, `weave --help`, `weave completion bash` and `weave __complete kill ''` each print `Error: …/settings.yml: keys.remove: key taken: "u" already does undo` and exit 1.

**Impact:** with `source <(weave completion bash)` in `~/.bashrc`, as the README says (`README.md:82`), every new shell prints that error and completes nothing until the file is fixed.

**Fix:** the settings only reach the confirmation dialog, the config form and the browser. Either load them only in the commands that show those (`weave`, `weave bind`, `weave edit`), or at least let the help, the version and completion through. With the first, `weave list` no longer refuses, so scenario 16 needs another command; with either, `docs/settings.md:18` says which commands refuse.

### 19. A note line holding a newline breaks the drawing

**Tested.**

**Where:** `internal/session/domain/layout.go:468` (`Pane.WithNote` takes any text), `internal/session/infra/tea_drawing.go:308` (`paneLines`).

**Problem:** names, dirs and commands go through `oneLine`; note lines don't, and the schema allows any string in `note`. So a block scalar in a `note` list is a valid config:

```yaml
note:
  - |
    first
    second
```

The pane prints it as two lines, then an empty one for the newline the block keeps at its end. `paneLines` hands it to the drawing as one line, newlines and all.

**Reproduced:** an 8-row drawing of that pane beside another came out as 10 lines, the borders out of line from the note down. `undrawnPanes` (`tea_drawing.go:241`) shows the same text when the drawing doesn't fit, and the config form's note rows probably do too (`tea_config_form.go:2078`; not checked).

**Fix:** where note lines are shown, starting with `paneLines`, split them on newlines and drop the final one, so every config that loads today stays valid. Refusing control characters in `WithNote`, as `oneLine` does, would refuse configs in the released format.

### 20. A config named `<name>.yaml` is skipped without a warning

**Tested.**

**Where:** `internal/session/infra/file_project_registry.go:58-62` (`Names`).

**Problem:** only `.yml` files are configs, and any other file is skipped silently, while a `.yml` file whose stem isn't a valid name gets a warning. `.yaml` is an easy name to give a config written by hand.

**Reproduced:** with `nimbus.yaml` beside `shop.yml`, `weave list` lists only `shop`, and `weave show nimbus` says `project not found: nimbus`.

**Fix:** warn about `.yaml` files with `warnSkipped`, e.g. `warning: skipping …/nimbus.yaml: configs end in .yml; rename it to register it`.

## Consistency

### Binding under a registered name asks twice

Saving asks `Replace blog's registered config with this?` under the diff (`internal/session/infra/tea_config_form.go:2237`), then the use case, `BindProject` since its rename, asks `blog is already registered. Replace it?` (`internal/session/app/bind_project.go`, then named after the old word). Scenario 11 tests both, so it is deliberate: the use case asks whatever form filled the project in. The cost is the same question answered twice; worth a look when the bind flow next changes.

## Read closely, nothing found

- Quoting: tmux arguments (`argument`, `formatArgument`), pane names in borders (`#{q/h:@label}`), values read back (`sized`, `verbatim`, `startArgs`), and the startup script's `shellQuote`.
- Configs: strict decoding (unknown fields, a second document, a bare `~`, a null grid), canonical encoding, atomic saves through a symlink.
- Layouts: grid cuts and sizes in cells, `#{window_layout}` parsing, creation order, `WithoutPane` handing a pane's space back.
- Sessions: `Start` cleaning up after a failed step, restoration placing windows, a kill whose teardown commands fail from inside the session, `weave .` with nested and symlinked roots.
- `weave edit --raw`: Esc answers No, which reopens the editor on what was typed, so edits aren't lost.

## Not covered

- The browser, the config form and the confirmation dialog weren't driven in a terminal: they were read, and one drawing was rendered in a test.
- Nothing ran on macOS or on a tmux other than 3.4.
