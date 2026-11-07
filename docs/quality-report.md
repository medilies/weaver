# weaver quality report

What is left of the review of 2026-11-06 and its revisions of 2026-11-07. Fixed findings, and findings about code since removed (weaver no longer runs git or clones repositories), have been taken out; `git log -- docs/quality-report.md` has them. Bugs keep their original numbers.

Line numbers refer to `7f8a02f` unless stated, so some have drifted. Findings marked **Tested** were run, with tmux experiments on private tmux 3.4 servers. The rest were checked by reading the code.

## Triage

Done against the project's aim of staying simple and small. macOS stays out of scope until it has users, and so does making sure weave works on tmux 3.2 (see Tooling). Severity is High, Med or Low. Fix size is XS (a line or two), S (up to about 30 lines plus a test) or M (several layers, or guesswork). Each item is **Open** (worth doing, not done yet), **Deferred** (until the idea named), **Left** (not worth the code) or **Ignored** (out of scope, as macOS). Left and ignored items stay listed so they aren't reported again.

### Bugs

| # | Bug | Severity | Fix | Status |
| --- | --- | --- | --- | --- |
| 5 | Pane started with a command frozen as a shell | Low | M | **Deferred** to config idea #4 |
| 14 | `env:` and `note:` panes skip `default-command` | Low | M | **Deferred** to config idea #5 |
| 15 | `weave kill` in a session named like a project | By design | — | **Deferred** to config idea #6 |
| 12 | Unreachable server counts as not running | Low | S, but tied to tmux's error wording | **Left** |
| 16 | macOS config location | — | — | **Ignored** |

### Other sections

- **Open:**
  - `$` in captured names (Freeze and capture)
  - freezing under a symlinked root (Paths)
  - a hint in the error when a restored window's directory is missing (README)
  - the test gaps
- **Deferred:** the items in "Fixes that wait for an idea" below.
- **Left:** schema leniency, the `weave edit` races, config file names that differ only by spaces, the GoReleaser version format, the changelog filters, a tmux version check when weave runs, and where `requireTool` lives.

### Fixes that wait for an idea

| Fix | Waits for | Why |
| --- | --- | --- |
| #14 | config idea #5 secrets (and #7 `wait`, if it waits in the script) | They change the end of the pane startup script (`shellCommand`). |
| #15, and naming the `stop:` key | config idea #6 `start` and `stop` | #6 makes kill load the project, which settles what killing the session you're in means. The glossary bans "stop". |
| #5, and `Capture`'s three calls per pane | config idea #4 | #4 adds a pane listing with process IDs. Keep `foreground` meaning "the shell is idle": #4 and #6's clean shutdown rely on it. |
| Form logic moved to `domain` (`ProjectRoot.Relative`) | config idea #2 window `dir` | #2 reworks `PaneDir` and `dirOf`. |
| Building overviews concurrently | browser idea #2 live preview | Its refresh loop changes how overviews are built. Overviews only check directories and programs, which is fast. |
| Cursor after a delete | browser idea #3 better list | Filtering changes how the selection works. |

## Bugs and edge cases

### 5. Freezing loses the command of a pane started with one

**Status:** **Deferred** to config idea #4.

**Where:** `foreground` in `internal/session/infra/tmux.go`, called from `capturePanes`.

**Problem:** a pane started with a command, such as `tmux new-window 'npm run dev'` or a script's `split-window 'htop'`, runs the program as the pane's own process. The terminal's foreground process group is then that process, which `foreground` reads as a shell idle at its prompt. The frozen pane becomes a bare shell. This breaks the `SessionCapturer.Capture` contract ("the commands running in them typed at their prompts").

**Tested on tmux 3.4:** a pane started as `sleep 300` has a `tpgid` equal to its `pane_pid`, and a `pane_start_command` of `"sleep 300"`.

**Fix:** when the foreground group is the pane's own process, treat the pane as idle only if `#{pane_start_command}` is empty or is weaver's own wrapper. Otherwise prefill the pane process's args.

Prefilling them every time would be wrong: an idle pane would get `-bash`, and a weaver pane with `env:` or `note:` would get `/bin/bash -l`. The fix would also prefill a pane started as `zsh` or `bash` with its shell, so it has to tell shells apart, which is guesswork.

### 12. A tmux server weave can't reach counts as not running

**Status:** **Left.**

**Where:** `internal/session/infra/tmux.go:57-59` (`Find`), `:271-273` (`Running`), `:283-285` (`Kill`), and `Capture`, so `weave freeze <session>` on a server weave can't reach says the session isn't running.

**Problem:** every exit error is read as "no such session, or no server". But tmux also exits 1 when it can't connect, for example to a socket it may not open. When that happens:

- `weave list` and the browser show nothing running.
- `weave kill shop` says "session not running", and so does the browser's kill, without asking.

**Tested on tmux 3.4:** with the socket at mode 000, `list-sessions` prints "error connecting to … (Permission denied)" and exits 1.

**Fix:** read an exit as "not running" only when stderr says so: "no server running", "can't find session", or "error connecting to … (No such file or directory)". Return every other error. `run` already builds it with tmux's message.

### 14. Panes with `env:` or `note:` skip tmux's `default-command`

**Status:** **Deferred** to config idea #5.

**Where:** `internal/session/infra/tmux_script.go:30` (`exec "${SHELL:-/bin/sh}" -l`).

**Problem:** plain panes run tmux's `default-command` when one is set, for example `reattach-to-user-namespace -l zsh`. The wrapper always ends by starting a login shell, so panes with `env:` or `note:` skip it.

**Not a problem: the shell itself.** tmux sets `SHELL` to `default-shell` in every new pane, even one started with an explicit command. The wrapper therefore starts the same shell as plain panes.

**Tested on tmux 3.4:**

- With `SHELL=/bin/sh` and `set -g default-shell /bin/bash`, a pane started as `/bin/sh -c 'echo $SHELL'` prints `/bin/bash`.
- tmux runs `default-command` as `$SHELL -c '<command>'`.

**Fix:** read `show-options -gv default-command`. When it is set, end the wrapper with `exec "$SHELL" -c` followed by the quoted command, as tmux runs it, instead of `exec "$SHELL" -l`.

**Not `-e`:** `new-session -e` sets the session's environment, not just the first pane's. The first pane's `env:` would reach every later window and pane, including the ones the user opens. `new-window -e` and `split-window -e` are pane-local, but tmux reads a trailing `;` in their values as a command separator, so the values would need `argument()`.

### 15. `weave kill` ends any session whose name looks like a project name

**Status:** **Deferred** to config idea #6.

Low severity, and by design: `cc4e803` made a plain `weave kill` behave like `tmux kill-session`.

**Where:** `internal/session/app/kill_project.go:33` (`HandleCurrent`).

**Scenario:** tmux names its default sessions `0`, `1` and so on, which are valid project names. Running `weave kill` inside one of them kills it. Nothing checks that it is an owned session, and nothing asks first. The glossary defines kill as ending *a project's* session.

**Fix, if it should change:** have `HandleCurrent` check that the session is owned, through `SessionFinder.Find`. A registered-name check would wrongly refuse after `d` deletes the config of a running session, which stays owned.

Don't ask for confirmation, so `weave kill` stays usable in scripts. Leave `weave kill <project>` as it is, since the user names the session.

### 16. On macOS, the documented config location is wrong (ignored)

**Ignored:** macOS only.

**Where:** `cmd/weave/main.go` (`os.UserConfigDir()`), with the text in `cmd/weave/root.go:14` and `README.md:97`.

**Problem:** on macOS, `os.UserConfigDir()` returns `~/Library/Application Support` and ignores `XDG_CONFIG_HOME`. The help text and README promise `~/.config/weaver/projects` and `$XDG_CONFIG_HOME`, and macOS binaries are shipped.

**Fix:** either document the macOS path, or resolve `$XDG_CONFIG_HOME`, falling back to `~/.config`, on every OS. The second option moves existing macOS configs. It would have to keep reading `~/Library/Application Support/weaver/projects` when the new directory doesn't exist.

## Consistency

### Glossary wording

- `docs/config-ideas.md:177-238` proposes a `stop:` config key and "stop" commands. These are teardown commands run before a kill, a new concept rather than another word for kill, but the glossary rules out "stop". Give the concept another name and a glossary entry before it gets code.

### Logic in the forms, against the layer rule

**Deferred** to config idea #2. These rules sit in `internal/session/infra/huh_forms.go` but belong in `domain`:

- `:232` re-checks that window names are unique. The validator could ask the domain instead, by calling `project.WithWindows` with the earlier windows and the new one.
- `:301` turns captured dirs into root-relative paths, and `dirOf` (`project_file.go:345`) does the same when saving. Both could use a `ProjectRoot.Relative(path)`.
- The validator repeats the `label`/`DefaultPaneName` handling and the rule that a single pane needs no name.
- `:223` pre-fills the first window's name with the project name, as `defaultWindow` names it. This one is weak: it is only a suggestion the user can change.

### Schema and code disagree

**Left.**

- `ParseSize` (`internal/session/domain/layout.go:174-180`) uses `strconv.Atoi`, so it accepts `+5`, `040` and padded values such as ` 40% `. The schema pattern rejects all three.
- `ParseDirection` and the root accept surrounding whitespace, and the schema doesn't.

A schema stricter than the parser is the safe way round: the editor warns, and weave still loads the config. Tightening the parsers could reject configs in the released format that load today, and every other value object trims its input (`oneLine`).

### README

- **Open.** A pane directory added to the config while its owned session runs makes `weave <project>` fail with "directory not found" for the restored window. This is deliberate, and the README and the browser's warnings say so. The error itself could hint that the directory has to be created first.

## Tooling and release

- **Left.** GoReleaser stamps `{{.Version}}` (`0.2.0`), while `go install …@v0.2.0` reports `v0.2.0`. Stamp `{{.Tag}}` to get one form, and update `TestReleaseBuildsReportTheStampedVersion`, which expects `0.2.0`.
- **Left.** The changelog filters (`^docs:`, `^test:`, `^chore:`) miss scoped and other types, so `build(deps): …`, `ci: …` and merge commits reach the release notes. If the intent is to drop those, use something like `^(docs|test|chore|ci|build)(\(.+\))?!?:` plus `^Merge`.
- **Left.** `requireTool` lives in `local_workspace_test.go`, though the tmux tests use it too.
- **Ignored**, tmux 3.2 only, until weave has users there. CI's main jobs install Ubuntu's tmux 3.4. Checking the version when weave runs is **Left**.

  tmux's changelog shows nothing weaver uses that is newer than 3.2: `new-window -b`, which restoration needs, arrived in exactly 3.2. A CI job on ubuntu-22.04 (tmux 3.2a) runs the tests with `continue-on-error`: a failure there is a hint, not a bug, until a 3.2 user reports one. GitHub will likely retire that runner around April 2027.
- **Ignored**, macOS only: CI builds only for Linux, although darwin binaries ship.

## Test gaps

**Open.**

- `TestTmuxStartLeavesNothingWhenAStepFails` only fails inside the first window. A failure in a later window, where cleanup has to kill the session by ID, is not covered.
- Decoding `~` is only tested for `root`, not for a pane's `dir:`. There is no save/reload test for a relative dir starting with `~`.
- `Capture` isn't tested with a pane started with a command (bug #5).
- Untested:
  - a `Size` of 0x0
  - `Kill` and `Running` against a server they can't reach (#12)
- Env and note panes are only tested without a `default-command`, so bug #14 cannot show up in tests.

## Minor issues

### `weave edit`

- **Left.** If the config is deleted while the editor is open, the edits are lost (`ErrProjectNotFound`).
- **Left.** Nothing protects against a lost update from a concurrent `weave freeze`.

### Freeze and capture

- **Open.** `Capture` reads labels with `display-message -p` (`capturePanes` in `tmux.go`), and tmux 3.4 escapes a `$` followed by a name or `{` in its output. A pane named `run $HOME` is frozen as `run \$HOME`. Window names read the same way would be too.
- **Left.** A pane with a job is frozen in its shell's directory, read from `/proc` since `f65e161`: `make -C sub dev` from the root is frozen with `dir:` the root, so its prefill finds `sub`. A subshell that changes directory first loses that directory instead: `(cd api && exec sleep 300)` is frozen with `dir:` the root and the prefill `sleep 300`. ps can't give back the `cd`, and commands like `make -C` are the more common case. A shell a wrapper forks, as a `default-command` of `sh -c 'bash -i; true'` does, is taken for a job, so the pane gets the wrapper's directory and its shell as a prefill: bug #5's guesswork about telling shells apart.
- **Deferred** to config idea #4. `Capture` runs three `display-message` calls per pane, where one `list-panes -F` would do. It's not worth changing on its own: freeze is interactive, and splitting free-text paths and labels out of one line of output adds parsing risk. #4's pane listing is the time to share one.

### Configs

- **Left.** A config file named `shop .yml` or ` shop.yml` lists as `shop`, because `NewProjectName` trims (`file_project_repository.go:64`). `FindByName` then reads `shop.yml`, which is another file or none at all. With both files present, `Names()` lists `shop` twice. Skip, with a warning, any file whose stem isn't exactly the name.

### Browser

- **Deferred** to browser idea #3. After a delete, the browser's cursor jumps to the top of the list, because `act` selects the deleted name. Select its neighbour instead.
- **Deferred** to browser idea #2. The browser rebuilds every overview, one after another, before each view. They only check directories and programs, so this is fast; build them concurrently when the live preview reworks the refresh.

### Paths

- **Open.** Freezing a project whose root is written through a symlink records its panes' directories outside the root. tmux and `/proc` both report resolved directories, and the form and `dirOf` make them relative to the root as written. `weave .` solved the same problem through `Workspace.RealPath`.

### macOS (ignored)

- **Ignored**, macOS only: the existing TODO about case-insensitive filesystems is a real bug. Freezing `shop` when `Shop.yml` exists overwrites it without the replace prompt, because `Names()` returns "Shop".
