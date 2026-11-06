# weaver quality report

Date: 2026-11-06, on `main` at `cc4e803` plus the `Confirm` dialog change, since committed as `b4d0292`.

`go build ./...`, `go vet ./...`, `go test ./...` and `golangci-lint run ./...` all pass with 0 issues.

Findings #1 and #2 were reproduced with scratch tests on a copy of the repo. The rest were checked by reading the code.

## Bugs and edge cases

Most severe first.

### 1. A `~` typed in the freeze form changes meaning once saved

**Where:** `internal/session/infra/huh_forms.go:350` (`paneAnswers.pane`), `internal/session/infra/project_file.go:345` (`dirOf`), `expandHome` in decode.

**Problem:** the form passes `~/x` to `domain.NewPaneDir` as a relative path, which resolves to `<root>/~/x`. Saving writes `dir: ~/x`, and decoding then expands that to `$HOME/x`.

**Reproduced:** before saving, the pane resolves to `/srv/p/~/x`. After a reload it resolves to `/home/ilies/x`.

**Same cause, root field:** the root validator (`huh_forms.go:150`) uses `domain.NewProjectRoot`. Typing `~/dev/shop` is rejected as "must be an absolute path", although the README and schema accept `~/` roots.

**Fix:** run `expandHome` on the Root and Directory answers before the domain constructors, in both the validators and `project()`/`pane()`.

### 2. Saving a symlinked config replaces the link with a plain 0600 file

**Where:** `internal/session/infra/file_project_repository.go:186-199` (`writeFileAtomic`), used by `ReplaceDocument` (`:118`) and `Save` (`:135`).

**Problem:** the temp file is created beside the link, and `os.Rename` replaces the link itself.

**Reproduced:** after `ReplaceDocument`, the link was a plain `-rw-------` file, and the target in the dotfiles repo still held the old content.

**Impact:** configs managed with stow or another dotfiles manager quietly stop syncing after `weave edit` or after a `weave freeze` that replaces a config.

**Fix:** if the path exists, resolve it with `filepath.EvalSymlinks` before creating the temp file, and keep the existing file's mode.

### 3. A failed restoration leaves a half-built window that is never restored again

**Where:** `internal/session/infra/tmux.go:105-140` (`Restore`), `:349` (`fill` sets `@weaver_window` before splitting).

**Problem:** if a split fails (for example "no space for new pane" in a small window), `Restore` returns the error without removing the window. The window already carries its tag, so the next `Find` counts it as present and `weave <project>` never fixes it. `Start` cleans up with `kill-session`, but `Restore` has no equivalent.

**Fix:** run `kill-window -t <windowID>` when `newWindow` succeeded but `fill` failed, or set the tag last.

### 4. Panes with `env:` or `note:` can start a different shell from plain panes

**Where:** `internal/session/infra/tmux_script.go:30` (`exec "${SHELL:-/bin/sh}" -l`).

**Problem:** plain panes get tmux's `default-shell` and `default-command`, but the wrapper ignores both.

**Scenario:** with `set -g default-shell /usr/bin/zsh` and `SHELL=/bin/bash`, panes with `env:` run bash and the others run zsh. A custom `default-command` is skipped too.

**Fix:** pass env with `-e VAR=val` on `new-session`, `new-window` and `split-window`, so only `note:` needs the wrapper. The README already requires tmux 3.2, which supports `-e`. Alternatively, read `show-options -gv default-shell` and honour `default-command` in the wrapper.

### 5. On macOS, the documented config location is wrong

**Where:** `cmd/weave/main.go` (`os.UserConfigDir()`), with the text in `cmd/weave/root.go:14` and `README.md:97`.

**Problem:** on macOS, `os.UserConfigDir()` returns `~/Library/Application Support` and ignores `XDG_CONFIG_HOME`. The help text and README promise `~/.config/weaver/projects` and `$XDG_CONFIG_HOME`, and macOS binaries are shipped.

**Fix:** either document the macOS path, or resolve `$XDG_CONFIG_HOME`, falling back to `~/.config`, on every OS.

### 6. A repository counts as cloned as soon as its directory exists

**Where:** `internal/session/app/open_project.go:145` (`cloneMissing`), `internal/session/infra/git_workspace.go:30` (`Exists`), and the browser's `RepoStatus.Cloned`.

**Scenario:** with `repositories: {.: url}` and an empty root that already exists (for example after `mkdir -p ~/dev/shop` on a new machine), nothing is cloned. The panes open in an empty directory, and the browser shows "✓ cloned".

**Fix:** treat a missing or empty directory as not cloned. `git clone` accepts an empty target.

### 7. `weave kill` ends any session whose name looks like a project name

Low severity, and partly by design.

**Where:** `internal/session/app/kill_project.go:33` (`HandleCurrent`) and `Handle`.

**Scenario:** tmux names its default sessions `0`, `1` and so on, which are valid project names. Running `weave kill` inside one of them kills it, with no check that it is a registered project or an owned session, and no confirmation. The glossary defines kill as ending *a project's* session.

**Fix:** require the name to be registered (or the session to be owned), or ask for confirmation.

## Consistency

### Glossary wording

- The form labels say "Run first…" and the freeze help says "commands to run first". The glossary term is *before commands*.
- The form says "Only type it" where the glossary term is *prefill*.
- Code says "label" (`@label`, `paneAnswers.label`) where the glossary says pane name.
- `docs/config-ideas.md:177-238` proposes a `stop:` config key and "stop" commands, but the glossary rules out "stop" in favour of "kill".

### Concepts missing from the glossary

`Workspace`, `ProjectDraft`, `ProjectStatus`/`RepoStatus`, and the browser's "summary" and "notice" have no glossary entries. CLAUDE.md says a new concept gets an entry before it gets code.

### Logic in the forms, against the layer rule

These rules sit in `internal/session/infra/huh_forms.go` but belong in `domain`:

- `:232` re-checks that window names are unique.
- `:223` defaults the first window's name to the project name, duplicating `defaultWindow`.
- `:301` turns captured dirs into root-relative paths. That is a `ProjectRoot` concern, for example `ProjectRoot.Relative(path)`.
- The validator repeats the `label`/`DefaultPaneName` handling and the rule that a single pane needs no name.

### Schema and code disagree

- `ParseSize` (`internal/session/domain/layout.go:174-180`) uses `strconv.Atoi`, so it accepts `+5`, `040` and padded values such as ` 40% `. The schema pattern rejects all three.
- `ParseDirection` and the root accept surrounding whitespace, and the schema doesn't.
- Fix by tightening the parsers to the schema's grammar.

### Error message

`ErrNotInsideTmux` (`internal/session/domain/errors.go:73`) says "not inside tmux; name a session". For `weave kill` run outside tmux it should say to name a project. Make the message neutral, or wrap it in `HandleCurrent`.

### README

- "If a step fails, nothing is left half-built" is not true for restoration (bug #3).
- The architecture diagram leaves out the `SessionFinder` and `SessionLocator` ports.

## Test gaps

- No test that a clone failure in `OpenProject` stops the start, meaning `Start` is never called. There is no test for a failed `Restore` either.
- `TestTmuxStartLeavesNothingWhenAStepFails` only fails inside the first window. A failure in a later window, where cleanup has to kill the session by ID, is not covered.
- Decoding `~` is only tested for `root`, not for a pane's `dir:`. There is no save/reload test for a relative dir starting with `~` (bug #1).
- No test that `ReplaceDocument` and `Save` keep a symlink and the file mode (bug #2).
- Env and note panes are only tested with `SHELL=/bin/sh` and no `default-shell` set, so bug #4 cannot show up in tests.
- `bareRepository` isolates `HOME` and `GIT_CONFIG_NOSYSTEM`, but not `XDG_CONFIG_HOME` or `GIT_CONFIG_GLOBAL`. The user's `$XDG_CONFIG_HOME/git/config` can leak into the git tests (`Detect`, `WorkingTree`).

## The `Confirm` dialog (b4d0292)

These apply to the dialog in `internal/session/infra/huh_forms.go`:

- Esc does nothing, because huh's only quit key is ctrl+c, while the browser quits on esc and q. Add esc as a cancel or "No" key.
- `confirmHelp` copies huh's help text by hand to size the dialog, and will drift if huh's keymap or labels change.
- "Add another window?" now appears centred, between full-screen freeze steps that sit at the top left.

## Minor issues

### `weave edit`

- An editor that exits non-zero (vim's `:cq`) shows "Error: running vi: exit status 1" instead of "Nothing saved."
- If the config is deleted while the editor is open, the edits are lost (`ErrProjectNotFound`).
- Nothing protects against a lost update from a concurrent `weave freeze`.

### Messages and browser behaviour

- `weave freeze nosuch` shows tmux's "can't find session: =nosuch", including the internal `=`.
- The browser asks "Kill X's session…?" for a project that isn't running, then shows "session not running".
- After a delete, the browser's cursor jumps to the top of the list.

### Determinism and speed

- `toRepos` iterates over a map, so with several bad repositories the reported error changes from run to run.
- `Capture` runs three `display-message` calls per pane, where one `list-panes -F` would do.

### macOS

- `HandleDir` compares `os.Getwd()`, which may pass through a symlink such as macOS's `/tmp`, with roots written as real paths.
- The existing TODO about case-insensitive filesystems is a real bug. Freezing `shop` when `Shop.yml` exists overwrites it without the replace prompt, because `Names()` returns "Shop".
