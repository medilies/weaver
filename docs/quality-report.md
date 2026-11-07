# weaver quality report

Date: 2026-11-06, on `main` at `cc4e803` plus the `Confirm` dialog change, since committed as `b4d0292`.

Revised 2026-11-07. Every finding was re-checked against the code at `7f8a02f`, and line numbers in the findings refer to that commit:

- The old #4 was mostly wrong and its fix unsafe. It is now #14, with a new fix.
- The `confirmHelp` item was out of date, and several other fixes changed.
- Two fresh reviews, one of `infra` and one of the other layers and the tooling, added bugs #3, #5, #6 and #8–#13, the "Tooling and release" section, and new items in the other sections.

On 2026-11-06, `go build ./...`, `go vet ./...`, `go test ./...` and `golangci-lint run ./...` all passed with 0 issues. The revision ran without a Go toolchain. Findings marked **Reproduced** or **Tested** were actually run, with tmux experiments on private tmux 3.4 servers. The rest were checked by reading the code.

## Triage

Done 2026-11-07, against the project's aim of staying simple and small. macOS stays out of scope until it has users, and so do making sure weave works on tmux 3.2 (see Tooling) and the git edge cases the follow-up reviews found. Severity is High, Med or Low. Fix size is XS (a line or two), S (up to about 30 lines plus a test) or M (several layers, or guesswork). Each item is **Fixed**, **Deferred** (until the idea named, or the fix named), **Open** (worth doing, not done yet), **Left** (not worth the code) or **Ignored** (out of scope, as macOS).

The fixes were checked with Go 1.26.10: `go build`, `go vet`, `go test` and `golangci-lint` pass. The freeze form, the browser's Esc and `weave kill` under `run-shell` were also checked by hand in a throwaway tmux server.

### Bugs

| # | Bug | Severity | Fix | Status |
| --- | --- | --- | --- | --- |
| 3 | weave in a popup or `run-shell` | High, for popup bindings | S | **Fixed** |
| 8 | Credentials copied from remotes | Med–High | XS | **Fixed** |
| 2 | Saving replaces a symlinked config | Med | S | **Fixed**, with `Sync` |
| 1 | `~` in the form's root and `--root=` | Low | S | **Fixed** |
| 4 | Half-built restored window | Low | XS | **Fixed** |
| 6 | Repository typed twice ends the freeze | Low | S | **Fixed** |
| 10 | `WorkingTree` outlives its bound | Low | XS | **Fixed** (`WaitDelay`; no process group) |
| 11 | 0x0 terminal size | Low | XS | **Fixed** |
| 13 | Submodules offered as repositories | Low, Med once #7 is fixed | XS | **Fixed**; a root that is itself a submodule is still found |
| 9 | Pane directory from the foreground job | Low | S | **Open**, optional |
| 5 | Pane started with a command frozen as a shell | Low | M | **Deferred** to config idea #4 |
| 7 | Empty directory counts as cloned | Low | M | **Deferred**, never before #13 |
| 14 | `env:` and `note:` panes skip `default-command` | Low | M | **Deferred** to config idea #5 |
| 15 | `weave kill` in a session named like a project | By design | — | **Deferred** to config idea #6 |
| 12 | Unreachable server counts as not running | Low | S, but tied to tmux's error wording | **Left** |
| 16 | macOS config location | — | — | **Ignored** |

Corrections to the findings below:

- #1: a `~/x` pane directory typed in the form was already saved as `dir: ~/x`, which loads as `$HOME/x`: what the user meant. Only the root and `--root=` needed fixing, plus the docs.
- #5: the proposed fix would also prefill a pane started as `zsh` or `bash` with its shell. A fix has to tell shells apart, which is guesswork.
- #6: only the project screen's answers were lost. The window screens come after it.
- `go 1.26` (Tooling): Go 1.26's `go` command rejects it. The build asks for `go mod tidy`, which writes `go 1.26.0` back. **Fixed** instead with `toolchain go1.26.10` in `go.mod`, which setup-go's `go-version-file` reads before the `go` line. It has to be bumped when a 1.26.x patch ships; CLAUDE.md says so.

### Other sections

- **Fixed:** the glossary wording, `ErrNotInsideTmux`, every README item (but see restoring below), the `Problem` comment, `specs.md`, tests before releasing, the `Confirm` dialog items, and the marked minor items and test gaps.
- **Open:** a hint in the error when a restored window's directory is missing, `#` in pane labels (`#{q/h:…}` works on tmux 3.4), the concepts missing from the glossary, the browser offering to kill a session that isn't running, `HandleDir` and symlinks, unquoted `~` in configs, and the unmarked test gaps.
- **Deferred:** the items in "Fixes that wait for an idea" below.
- **Left:** schema leniency (as the report says), the `weave edit` races, config file names that differ only by spaces, the GoReleaser version format, the changelog filters, and a tmux version check when weave runs.

### Fixes that wait for an idea

| Fix | Waits for | Why |
| --- | --- | --- |
| #14 | config idea #5 secrets (and #7 `wait`, if it waits in the script) | They change the end of the pane startup script (`shellCommand`). |
| #15, and naming the `stop:` key | config idea #6 `start` and `stop` | #6 makes kill load the project, which settles what killing the session you're in means. The glossary bans "stop". |
| #5, and `Capture`'s three calls per pane | config idea #4 | #4 adds a pane listing with process IDs. Keep `foreground` meaning "the shell is idle": #4 and #6's clean shutdown rely on it. |
| Form logic moved to `domain` (`ProjectRoot.Relative`) | config idea #2 window `dir` | #2 reworks `PaneDir` and `dirOf`. |
| Building overviews concurrently | browser idea #2 live preview | Its refresh loop changes how overviews are built. #10 covers the worst case. |
| Cursor after a delete | browser idea #3 better list | Filtering changes how the selection works. |

Some fixes come before the ideas that build on them. #3 is groundwork for tasks in popups (config idea #10). #4 comes before focus (#3) and lazy windows (#9), which extend restoration.

### From the follow-up reviews

- **Fixed:**
  - Esc on "Edit it again?" threw away the user's edits once Esc meant No. The question is now "Discard your changes?", so No and Esc edit again.
  - A failed save also names the config, not only the temporary file.
  - A working tree is still read when git finished but a child of it held its output open past `WaitDelay`.
  - `weave freeze --root=~user/x` is rejected, as such a root is in a config, instead of being read as relative to the current directory.
- **Ignored**, git edge cases, until a user runs into one:
  - Freezing offers a linked `git worktree` under the root as a repository of its own.
  - A root inside a superproject offers that superproject's other submodules.
  - `withoutCredentials` keeps the credentials of a URL that `url.Parse` rejects, and re-escapes a path that mixes `%2F` with characters needing escapes.

## Bugs and edge cases

Most severe first, then the ignored ones. Findings marked **Ignored** are out of scope: macOS, tmux 3.2 and git edge cases. They stay listed so they aren't reported again.

### 1. A `~` typed in the freeze form changes meaning once saved

**Status:** **Fixed.**

**Where:** `internal/session/infra/huh_forms.go:350` (`paneAnswers.pane`), `internal/session/infra/project_file.go:345` (`dirOf`), `expandHome` in decode.

**Problem:** the form passes `~/x` to `domain.NewPaneDir` as a relative path, which resolves to `<root>/~/x`. Saving writes `dir: ~/x`, and decoding then expands that to `$HOME/x`.

**Reproduced:** before saving, the pane resolves to `/srv/p/~/x`. After a reload it resolves to `/home/ilies/x`.

**Same cause, root field:** the root validator (`huh_forms.go:150`) uses `domain.NewProjectRoot`. Typing `~/dev/shop` is rejected as "must be an absolute path", although the README and schema accept `~/` roots.

**Same cause, `--root=`:** bash doesn't expand a `~` after `--root=`, since `--root` isn't a variable name. `FreezeProject.draft` (`internal/session/app/freeze_project.go:82-87`) then joins `~/dev/shop` to the current directory. `Detect` finds nothing, and accepting the form registers a root that doesn't exist.

**Fix:** read the answers the way configs are read. Add `parseRoot` and `parsePaneDir` beside `parseEnv` in `form_text.go`: each expands `~` and then calls the domain constructor. Use them in decode (`toProject`, `toPane`), in the validators and in `project()`/`pane()`, so the `~` rule lives in one place.

`app` can't import `infra`, so `cmd/weave/freeze.go` should expand `--root` with the same rule before building the `FreezeRequest`.

**Also undocumented:** neither the README's `dir` row nor the schema's `dir` description mentions `~/`, although decoding expands it and saving writes it (`dirOf` calls `contractHome`).

### 2. Saving a symlinked config replaces the link with a plain 0600 file

**Status:** **Fixed.**

**Where:** `internal/session/infra/file_project_repository.go:186-199` (`writeFileAtomic`), used by `ReplaceDocument` (`:118`) and `Save` (`:135`).

**Problem:** the temp file is created beside the link, and `os.Rename` replaces the link itself.

**Reproduced:** after `ReplaceDocument`, the link was a plain `-rw-------` file, and the target in the dotfiles repo still held the old content.

**Impact:** configs linked one by one, as stow does when `~/.config/weaver/projects` already exists, quietly stop syncing after `weave edit` or after a `weave freeze` that replaces a config. A linked `projects` directory is not affected.

**Fix:** when the path is a symlink, resolve it with `filepath.EvalSymlinks` and write beside the target. A dangling link makes `EvalSymlinks` fail: report that rather than replace the link.

The 0600 mode is a separate choice. `writeFileAtomic` documents owner-only files on purpose, and git tracks only the executable bit, so the mode doesn't break syncing.

### 3. Inside a tmux popup or `run-shell`, weave acts as if it ran outside tmux

**Status:** **Fixed.**

**Where:** `cmd/weave/main.go:40-43` (the pane comes only from `$TMUX_PANE`), `internal/session/infra/tmux.go:247` (`Current`) and `:291` (`Attach`).

**Problem:** tmux gives the commands of `display-popup` and `run-shell` `$TMUX` but no `$TMUX_PANE`. weave then has no pane, so `Attach` execs `tmux attach-session`, which tmux refuses inside tmux ("sessions should be nested with care"). The session is started, but the user is never switched to it. `weave kill` and `weave freeze .` fail with "not inside tmux".

**Scenario:** `bind w display-popup -E weave`, a common way to bind a project picker.

**Tested on tmux 3.4:** commands run by `display-popup -E` and by `run-shell` both see `TMUX=…` and no `TMUX_PANE`.

**Fix:** tell "inside tmux" (`$TMUX`) apart from "weave's own pane" (`$TMUX_PANE`):

- Inside tmux, `Attach` uses `switch-client`, whether or not there is a pane.
- Without a pane, `Current` runs `display-message -p '#{session_name}'` without `-t`.

Keep `Pane` meaning weave's own pane. `Capture` skips that pane's foreground job, so filling `Pane` with the active pane would drop that pane's command. `TMUX= weave` keeps working as it does now.

### 4. A failed restoration leaves a half-built window that is never restored again

**Status:** **Fixed.**

**Where:** `internal/session/infra/tmux.go:105-140` (`Restore`), `:349` (`fill` sets `@weaver_window` before splitting).

**Problem:** if a split fails (for example "no space for new pane" in a small window), `Restore` returns the error without removing the window. The window already carries its tag, so the next `Find` counts it as present and `weave <project>` never fixes it. `Start` cleans up with `kill-session`, but `Restore` has no equivalent.

**Reproduced on tmux 3.4:** a window tagged `@weaver_window`, then split until "no space for new pane", stays in the session with its tag.

**Fix:** run `kill-window -t <windowID>` when `fill` fails after `newWindow` succeeded. Setting the tag last is not enough on its own: the broken window would stay, and the next open would add a second window with the same name.

### 5. Freezing loses the command of a pane started with one

**Status:** **Deferred** to config idea #4.

**Where:** `internal/session/infra/tmux.go:231-244` (`foreground`), called at `:200-204`.

**Problem:** a pane started with a command, such as `tmux new-window 'npm run dev'` or a script's `split-window 'htop'`, runs the program as the pane's own process. The terminal's foreground process group is then that process, which `foreground` reads as a shell idle at its prompt. The frozen pane becomes a bare shell. This breaks the `SessionCapturer.Capture` contract ("the commands running in them typed at their prompts").

**Tested on tmux 3.4:** a pane started as `sleep 300` has a `tpgid` equal to its `pane_pid`, and a `pane_start_command` of `"sleep 300"`.

**Fix:** when the foreground group is the pane's own process, treat the pane as idle only if `#{pane_start_command}` is empty or is weaver's own wrapper. Otherwise prefill the pane process's args.

Prefilling them every time would be wrong: an idle pane would get `-bash`, and a weaver pane with `env:` or `note:` would get `/bin/bash -l`.

### 6. A repository typed twice in the freeze form ends the freeze

**Status:** **Fixed.**

**Where:** `internal/session/infra/huh_forms.go:157` (the validator only runs `parseRepos`), `:195` (`project()` calls `domain.NewRepos`).

**Problem:** the field's check passes when the user types `back = …` while `back` is ticked under "Repositories found" (they all start ticked), or types a directory twice. After the screen, `domain.NewRepos` returns "two repositories share a directory", `Fill` fails, and every answer is lost. `HuhForms` promises that mistakes show as they are typed.

**Fix:** build the repositories in one helper, `projectAnswers.repos(found)`. It takes the typed lines plus the ticked found ones and passes them through `domain.NewRepos`. Call it from the text field's validator and from `project()`. The multi-select comes first in the same group, so `a.keep` is set when the text field is checked.

Keep the check in `project()` too, since the user can go back and tick a box again.

### 7. A repository counts as cloned as soon as its directory exists

**Status:** **Deferred**, never before #13.

**Where:** `internal/session/app/open_project.go:145` (`cloneMissing`), `internal/session/infra/git_workspace.go:30` (`Exists`), and the browser's `RepoStatus.Cloned`.

**Scenario:** with `repositories: {.: url}` and an empty root that already exists (for example after `mkdir -p ~/dev/shop` on a new machine), nothing is cloned. The panes open in an empty directory, and the browser shows "✓ cloned".

**Fix:** add a `Workspace` method that says whether a repository's directory is cloned: it exists and isn't empty. `git clone` accepts an empty target. Use it in `cloneMissing` and for `RepoStatus.Cloned`.

Don't change `Exists`: pane directories are checked with it, and an empty pane directory is fine.

### 8. Freezing copies credentials from remote URLs into the config

**Status:** **Fixed.**

**Where:** `internal/session/infra/git_workspace.go:146-150` (`originOf`).

**Problem:** `git remote get-url origin` prints the URL as configured, for example `https://me:ghp_secret@github.com/me/x.git`. `Detect` offers it ticked, and saving writes it to `<name>.yml`, which the browser shows and a dotfiles repo may push.

**Tested:** `git remote get-url origin` prints a URL's user name and password unchanged.

**Fix:** for `http` and `https` remotes, drop all of the user info (`u.User = nil`), not just the password: a GitHub token is often the user name (`https://ghp_…@github.com/…`). Keep the user of `ssh://` and scp-style `git@host:path` remotes, which isn't a secret.

### 9. Freezing takes a pane's directory from its foreground job, not its shell

**Status:** **Open**, optional.

**Where:** `internal/session/infra/tmux.go:184` and `:197-199` (`#{pane_current_path}`).

**Problem:** tmux reports the directory of the pane's foreground process. Take a pane whose shell sits in the root and runs `make -C sub dev`. It is frozen with `dir: sub` and the prefill `make -C sub dev`, which then looks for `sub/sub`.

**Tested on tmux 3.4:** with the shell in `proj` and `(cd sub && exec sleep 300)` in the foreground, `pane_current_path` is `proj/sub`, while `/proc/<pane_pid>/cwd` is `proj`.

**Fix:** when a foreground job was found, read the shell's own directory from `/proc/<pane_pid>/cwd`. Keep `pane_current_path` for idle panes and as the fallback.

### 10. The browser can wait far longer than `WorkingTree`'s one-second bound

**Status:** **Fixed**, with `WaitDelay` only.

**Where:** `internal/session/infra/git_workspace.go:69-75`.

**Problem:** when the context expires, only git itself is killed. `Output` reads git's stdout and stderr through pipes until every writer has closed them, and git's children inherit those pipes: a submodule's own `git status`, or a `core.fsmonitor` hook. With `WaitDelay` unset, `WorkingTree` only returns once they exit. The browser calls it once per project, one project after another, every time it is shown.

**Tested:** with a script that mirrors `os/exec`'s behaviour, git was killed at 1 s. The pipes closed at 4 s with a 4-second fsmonitor hook, and at 8 s with a slow submodule.

**Fix:** set `cmd.WaitDelay`, for example to 100 ms. Optionally start git in its own process group and kill the whole group, so orphaned children don't keep running.

### 11. A terminal that reports a 0x0 size makes opening fail

**Status:** **Fixed.**

**Where:** `internal/session/infra/terminal.go:12-22` (`TerminalSize`), used by `newSession` (`tmux.go:317-321`).

**Problem:** a pseudo-terminal whose size was never set reports 0 columns and 0 rows, without an error. `TerminalSize` passes that on, and `new-session -x 0 -y 0` fails. This happens under `script`, expect, and some CI harnesses.

**Tested on tmux 3.4:** `new-session -d -x 0 -y 0` fails with "width too small". A fresh pty reports 0x0: `script -qc 'stty size' /dev/null </dev/null` prints `0 0`.

**Fix:** report a size only when both numbers are positive, and otherwise try the next file. Without a size, tmux falls back to 80x24.

### 12. A tmux server weave can't reach counts as not running

**Status:** **Left.**

**Where:** `internal/session/infra/tmux.go:57-59` (`Find`), `:271-273` (`Running`), `:283-285` (`Kill`), and since its fix, `Capture`, so `weave freeze <session>` on a server weave can't reach says the session isn't running.

**Problem:** every exit error is read as "no such session, or no server". But tmux also exits 1 when it can't connect, for example to a socket it may not open. When that happens:

- `weave list` and the browser show nothing running.
- `weave kill shop` says "session not running".
- Opening clones missing repositories before `new-session` fails with the real error.

**Tested on tmux 3.4:** with the socket at mode 000, `list-sessions` prints "error connecting to … (Permission denied)" and exits 1.

**Fix:** read an exit as "not running" only when stderr says so: "no server running", "can't find session", or "error connecting to … (No such file or directory)". Return every other error. `run` already builds it with tmux's message.

### 13. Freezing offers git submodules as repositories of their own

**Status:** **Fixed.**

**Where:** `internal/session/infra/git_workspace.go:107-151` (`Detect`, `originOf`).

**Problem:** a submodule checked out in one of the root's directories is its own top level and has an origin, so `Detect` offers it, ticked.

**Scenario:** on a new machine, a root cloned without `--recurse-submodules` leaves the submodule's directory empty. weave skips it (bug #7). Once #7 is fixed, weave would clone it as a separate repository instead of initialising the submodule.

**Tested:** for a submodule at `top/lib`, `rev-parse --show-toplevel` gives `top/lib`, and `--show-superproject-working-tree` prints `top`.

**Fix:** run `git rev-parse --show-toplevel --show-superproject-working-tree` as one call, and skip a directory that has a superproject. Consider cloning with `--recurse-submodules`.

### 14. Panes with `env:` or `note:` skip tmux's `default-command`

**Status:** **Deferred** to config idea #5.

Low severity.

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

- **Fixed.** The form labels say "Run first…" and the freeze help says "commands to run first". The glossary term is *before commands*.
- **Fixed.** The form says "Only type it" where the glossary term is *prefill*.
- **Fixed.** The root command's short help says "Start or attach a project's tmux session". The glossary word is *open*.
- **Fixed.** The glossary defines *session* as the tmux session named after a project. But `weave freeze [session]`, `SessionLister` and `SessionCapturer` mean any tmux session.
- **Fixed.** The code calls a pane's name both `PaneName` and label (`paneAnswers.label`, the `@label` pane option), and the glossary has no entry for it. Renaming `paneAnswers.label` is cheap. Keep `@label`: running sessions store it, so renaming it would lose the labels when freezing a session an older weave started.
- `docs/config-ideas.md:177-238` proposes a `stop:` config key and "stop" commands. These are teardown commands run before a kill, a new concept rather than another word for kill, but the glossary rules out "stop". Give the concept another name and a glossary entry before it gets code.

### Concepts missing from the glossary

`Workspace`, `ProjectDraft`, `ProjectStatus`/`RepoStatus`, capture (`SessionCapturer`), program (`ErrMissingProgram`), and the browser's "summary" and "notice" have no glossary entries. CLAUDE.md says a new concept gets an entry before it gets code.

### Logic in the forms, against the layer rule

These rules sit in `internal/session/infra/huh_forms.go` but belong in `domain`:

- `:232` re-checks that window names are unique. The validator could ask the domain instead, by calling `project.WithWindows` with the earlier windows and the new one.
- `:301` turns captured dirs into root-relative paths, and `dirOf` (`project_file.go:345`) does the same when saving. Both could use a `ProjectRoot.Relative(path)`.
- The validator repeats the `label`/`DefaultPaneName` handling and the rule that a single pane needs no name.
- `:223` pre-fills the first window's name with the project name, as `defaultWindow` names it. This one is weak: it is only a suggestion the user can change.

### Schema and code disagree

- `ParseSize` (`internal/session/domain/layout.go:174-180`) uses `strconv.Atoi`, so it accepts `+5`, `040` and padded values such as ` 40% `. The schema pattern rejects all three.
- `ParseDirection` and the root accept surrounding whitespace, and the schema doesn't.
- Leave both. A schema stricter than the parser is the safe way round: the editor warns, and weave still loads the config. Tightening the parsers could reject configs in the released format that load today, and every other value object trims its input (`oneLine`).

### Error message

**Fixed.** `ErrNotInsideTmux` (`internal/session/domain/errors.go:73`) says "not inside tmux; name a session". For `weave kill` run outside tmux it should say to name a project. Make the message neutral, or wrap it in `HandleCurrent`.

### README

- **Fixed.** "If a step fails, nothing is left half-built" is written about starting a session, but reads as true of restoration too, which it isn't (bug #4). Fixing #4 makes it true for both.
- **Fixed** in the README; the error still gives no hint (**Open**). Restoring windows never clones. A repository added to the config while its owned session runs makes `weave <project>` fail with "directory not found" for its window. This is deliberate: `BrowseProjects.warnings` says so, and a test pins the warning. But the README doesn't say so, and the error gives no hint. Either clone the missing repositories that hold the restored panes' directories, or document the behaviour and add a hint to the error.
- **Fixed.** The architecture diagram leaves out the `SessionFinder` and `SessionLocator` ports.
- **Fixed.** The name rule (`README.md:99`) leaves out that a name starts with a letter or a digit.
- **Fixed.** The `split` row doesn't say that `of` takes a pane's name, though the schema does.
- **Fixed.** The warnings paragraph says panes with `before` commands aren't checked, but not that panes whose `env:` sets `PATH` aren't either.
- **Fixed.** The keys table lists q but not esc.

### Other docs

- **Fixed.** `ProjectOverview.Problem` (`internal/session/app/browse_projects.go:53`) says the rest of the overview is then empty. But when `Exists` fails in `warnings()` (`:244`), the project, repositories, working tree and session are already filled.
- **Fixed.** `specs.md` is the first brief (`weaver globex`, "config json", "tui"), and it contradicts the released CLI and the glossary. Mark it as historical, or update it.

## Tooling and release

- **Fixed** with a `toolchain` line, not the fix proposed here; see the triage. `go.mod` says `go 1.26.0`, and setup-go installs exactly the patch it names. CI and release binaries were therefore built with Go 1.26.0, without any of the 1.26.x fixes. Writing `go 1.26` makes setup-go pick the latest 1.26.x, and go.mod stays the one source of the version.
- **Fixed** for the tests: `release.yml` publishes without running the tests or the linter. It now runs the tests. The linter already runs on every push to `main`.
- **Left.** GoReleaser stamps `{{.Version}}` (`0.2.0`), while `go install …@v0.2.0` reports `v0.2.0`. Stamp `{{.Tag}}` to get one form, and update `TestReleaseBuildsReportTheStampedVersion`, which expects `0.2.0`.
- **Left.** The changelog filters (`^docs:`, `^test:`, `^chore:`) miss scoped and other types, so `build(deps): …`, `ci: …` and merge commits reach the release notes. If the intent is to drop those, use something like `^(docs|test|chore|ci|build)(\(.+\))?!?:` plus `^Merge`.
- **Ignored**, tmux 3.2 only, until weave has users there: CI installed only Ubuntu's tmux (3.4), so the README's "tmux 3.2 or newer" was untested. Checking the version when weave runs is **Left**.

  tmux's changelog shows nothing weaver uses that is newer than 3.2: `new-window -b`, which restoration needs, arrived in exactly 3.2. The changelog doesn't say whether 3.2 reads `#` in window names and directories as 3.4 does. A CI job on ubuntu-22.04 (tmux 3.2a) now runs the tests with `continue-on-error`: a failure there is a hint, not a bug, until a 3.2 user reports one. GitHub will likely retire that runner around April 2027.
- **Ignored**, macOS only: CI builds only for Linux, although darwin binaries ship.

## Test gaps

- **Fixed.** No test that a clone failure in `OpenProject` stops the start, meaning `Start` is never called. There is no test for a failed `Restore` either.
- `TestTmuxStartLeavesNothingWhenAStepFails` only fails inside the first window. A failure in a later window, where cleanup has to kill the session by ID, is not covered.
- Decoding `~` is only tested for `root`, not for a pane's `dir:`. There is no save/reload test for a relative dir starting with `~` (bug #1).
- **Fixed.** No test that `ReplaceDocument` and `Save` keep a symlink (bug #2). `ReplaceDocument` is tested; `Save` shares `writeFileAtomic`.
- `Capture` isn't tested with a pane started with a command (bug #5) or a foreground job in another directory (#9). Names that differ only by spaces: **Fixed.**
- `TestTmuxStartPassesTextVerbatim` covers a `#` in window names and directories, but not in pane labels.
- Untested:
  - a `Size` of 0x0 (#11)
  - `Kill` and `Running` against a server they can't reach (#12)
  - **Fixed:** `Detect` with a submodule (#13) or a remote holding credentials (#8)
  - **Fixed:** the `WorkingTree` bound (#10)
- Env and note panes are only tested without a `default-command`, so bug #14 cannot show up in tests.
- **Fixed.** `bareRepository` isolates `HOME` and `GIT_CONFIG_NOSYSTEM`, but not `XDG_CONFIG_HOME` or `GIT_CONFIG_GLOBAL`. The user's `$XDG_CONFIG_HOME/git/config` can leak into the git tests (`Detect`, `WorkingTree`). Setting `GIT_CONFIG_GLOBAL=/dev/null` (git 2.32 or newer) shuts out both.
- **Fixed.** "prefers the innermost root when roots nest" (`open_project_test.go:306`) would also pass with a first-match implementation, because the inner project, `back`, sorts before `shop`. Give it a name that sorts after, such as `shop-back`.
- **Fixed.** "orders the root first and parents before nested repositories" (`repo_test.go:74`) would pass without `cloneOrder`'s `.` cases, since every directory in it sorts after `.` anyway. Add one that sorts before it, such as `-old`.
- **Fixed.** "the last of running and prefilling wins" (`layout_test.go:197`) checks the command, not that `Submits()` is false.
- **Fixed.** `TestLocalBuildsReportDev` and `buildVersion`'s comment assume that local builds report `(devel)`. Since Go 1.24, `go build` in a git checkout stamps a pseudo-version, so a local build shows that, not "dev".

## The `Confirm` dialog (b4d0292)

These apply to the dialog in `internal/session/infra/huh_forms.go`:

- **Fixed.** Esc does nothing, because huh 2.0.3's only quit key is ctrl+c, while the browser quits on esc and q. Add esc to the confirm field's "No" keys (`KeyMap.Confirm.Reject`, which holds `n` and `N`) rather than making it cancel. Cancelling "Add another window?" aborts the whole freeze and loses everything typed so far.
- **Fixed.** "Add another window?" now appears centred, between full-screen freeze steps that sit at the top left.

## Minor issues

### `weave edit`

- **Fixed.** An editor that exits non-zero (vim's `:cq`) shows "Error: running vi: exit status 1", without saying whether anything was saved. Reporting it is intentional and tested ("reports an editor that fails" in `editor_test.go`), and git does the same. Keep the error, but add that nothing was saved.
- **Left.** If the config is deleted while the editor is open, the edits are lost (`ErrProjectNotFound`).
- **Left.** Nothing protects against a lost update from a concurrent `weave freeze`.

### Freeze and capture

- **Fixed.** `Capture` trims window and pane names only after making them unique (`tmux.go:166`, `:205-209`). tmux keeps `x` and `x ` apart, both become `x`, and freezing fails with "pane names must be unique". Trim before `unique`.
- **Fixed.** `parseEnv` (`form_text.go:14-28`) silently keeps the last of two lines that set the same name, while a config with a duplicate key is rejected. Reject duplicates in the validator.
- Pane labels holding `##` or `#[` are drawn as formats in their borders (`tmux.go:399`): `a##b` shows as `a#b`, and `#[reverse]` changes the style. Window names and directories are kept verbatim. `#{q/h:@label}` shows labels verbatim on tmux 3.4. Check that it exists in tmux 3.2.

### Configs

- **Left.** A config file named `shop .yml` or ` shop.yml` lists as `shop`, because `NewProjectName` trims (`file_project_repository.go:64`). `FindByName` then reads `shop.yml`, which is another file or none at all. With both files present, `Names()` lists `shop` twice. Skip, with a warning, any file whose stem isn't exactly the name.
- An unquoted `~` is YAML's null, which decodes as an empty string with no error. `dir: ~` silently means the root, not home, and `root: ~` reports an empty root, although the schema allows `~`. Saving is safe, since it writes `"~"`. Document the quotes, or reject a null `dir:` or `root:` with a hint.
- **Fixed.** `NewProjectRoot` (`internal/session/domain/project.go:58`) doesn't reject control characters, while pane dirs and repositories do (`oneLine`).
- **Fixed.** `writeFileAtomic` doesn't `Sync` the temp file before renaming it. On filesystems without ext4's `auto_da_alloc`, such as XFS, a crash right after a save can leave an empty config.

### Messages and browser behaviour

- **Fixed.** `weave freeze nosuch` shows "tmux list-windows: exit status 1: can't find session: nosuch" (tmux 3.4). With no tmux server running, it shows tmux's connection error instead ("no server running on …" or "error connecting to …"). Have `Capture` turn tmux's exit error into one wrapping `ErrSessionNotRunning`, as `Kill` does.
- **Fixed.** Config errors name Go types, such as "field comand not found in type infra.paneFile" or "cannot unmarshal !!str … into []string". They reach `weave edit`, `weave show` and the browser. Replace `projectFile`, `windowFile`, `paneFile` and `splitFile` with project, window, pane and split.
- **Fixed.** `Command.Program` (`internal/session/domain/layout.go:278`) misses builtins such as `unsetopt`, `unalias`, `shopt`, `print` and `disown`. It also takes `#` for a program, so a prefill like `# run migrations` warns "program not found: #".
- The browser asks "Kill X's session…?" for a project that isn't running, then shows "session not running". `BrowseProjects.act` could check `sessions.Find` before asking.
- **Deferred** to browser idea #3. After a delete, the browser's cursor jumps to the top of the list, because `act` selects the deleted name. Select its neighbour instead.

### Determinism and speed

- **Fixed.** `toRepos` iterates over a map, so with several bad repositories the reported error changes from run to run. Iterate over the sorted keys.
- **Deferred** to browser idea #2. The browser rebuilds every overview, one after another, before each view (`browse_projects.go:196`), running `git status` once per project with its own one-second bound. With many projects in large repositories, it takes seconds to appear, and again after each action. Build the overviews concurrently, or under one deadline (see also bug #10).
- **Deferred** to config idea #4. `Capture` runs three `display-message` calls per pane, where one `list-panes -F` would do. It's not worth changing on its own: freeze is interactive, and splitting free-text paths and labels out of one line of output adds parsing risk. #4's pane listing is the time to share one.

### Paths

- `HandleDir` compares `os.Getwd()`, which keeps a symlink the user changed directory through, with roots as written. When one goes through a symlink and the other doesn't, no project is found. This is not only macOS's `/tmp`: any symlinked directory does it. Compare both after `filepath.EvalSymlinks`.

### macOS (ignored)

- **Ignored**, macOS only: the existing TODO about case-insensitive filesystems is a real bug. Freezing `shop` when `Shop.yml` exists overwrites it without the replace prompt, because `Names()` returns "Shop".
