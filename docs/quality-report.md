# weaver quality report

What is left of the review of 2026-11-06 and its revisions of 2026-11-07. Fixed findings, and findings about code since removed (weaver no longer runs git or clones repositories), have been taken out; `git log -- docs/quality-report.md` has them. Bugs keep their original numbers.

Line numbers refer to `7f8a02f` unless stated, so some have drifted. Findings marked **Tested** were run, with tmux experiments on private tmux 3.4 servers. The rest were checked by reading the code.

## Triage

Done against the project's aim of staying simple and small. macOS stays out of scope until it has users, and so does making sure weave works on tmux 3.2 (see Tooling). Severity is High, Med or Low. Fix size is XS (a line or two), S (up to about 30 lines plus a test) or M (several layers, or guesswork). Each item is **Open** (worth doing, not done yet), **Deferred** (until the idea named), **Left** (not worth the code) or **Ignored** (out of scope, as macOS). Left and ignored items stay listed so they aren't reported again.

### Bugs

| # | Bug | Severity | Fix | Status |
| --- | --- | --- | --- | --- |
| 16 | macOS config location | — | — | **Ignored** |

### Other sections

- **Open:** the test gaps.
- **Deferred:** the items in "Fixes that wait for an idea" below.
- **Left:** schema leniency, the `weave edit` races, config file names that differ only by spaces, the GoReleaser version format, the changelog filters, a tmux version check when weave runs, and where `requireTool` lives.

### Fixes that wait for an idea

| Fix | Waits for | Why |
| --- | --- | --- |
| Naming the `stop:` key | config idea #6 `start` and `stop` | The glossary bans "stop". |
| `Capture`'s four calls per pane | config idea #4 | #4 adds a pane listing with process IDs. Keep `foreground` meaning "the shell is idle": #4 and #6's clean shutdown rely on it. |
| Form logic moved to `domain` (`ProjectRoot.Relative`) | config idea #2 window `dir` | #2 reworks `PaneDir` and `dirOf`. |
| Building overviews concurrently | browser idea #2 live preview | Its refresh loop changes how overviews are built. Overviews only check directories and programs, which is fast. |
| Cursor after a delete | browser idea #3 better list | Filtering changes how the selection works. |

## Bugs and edge cases

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
- Untested: a `Size` of 0x0.

## Minor issues

### `weave edit`

- **Left.** If the config is deleted while the editor is open, the edits are lost (`ErrProjectNotFound`).
- **Left.** Nothing protects against a lost update from a concurrent `weave freeze`.

### Freeze and capture

- **Left.** A pane with a job is frozen in its shell's directory, read from `/proc` since `f65e161`: `make -C sub dev` from the root is frozen with `dir:` the root, so its prefill finds `sub`. A subshell that changes directory first loses that directory instead: `(cd api && exec sleep 300)` is frozen with `dir:` the root and the prefill `sleep 300`. ps can't give back the `cd`, and commands like `make -C` are the more common case. A shell a wrapper forks, as a `default-command` of `sh -c 'bash -i; true'` does, is taken for a job, so the pane gets the wrapper's directory and its shell as a prefill. Telling shells apart would be guesswork.
- **Left.** A pane tmux started with a command is frozen with that command as a prefill, unless it is weaver's startup script or tmux's `default-command` or `default-shell`. A pane started as another shell, as `tmux new-window zsh` where `default-shell` is bash, is frozen with `zsh` typed at its prompt, for the same reason.
- **Deferred** to config idea #4. `Capture` runs four or five `display-message` calls per pane, where one `list-panes -F` would do. It's not worth changing on its own: freeze is interactive, and splitting free-text paths and labels out of one line of output adds parsing risk. #4's pane listing is the time to share one.

### Configs

- **Left.** A config file named `shop .yml` or ` shop.yml` lists as `shop`, because `NewProjectName` trims (`file_project_repository.go:64`). `FindByName` then reads `shop.yml`, which is another file or none at all. With both files present, `Names()` lists `shop` twice. Skip, with a warning, any file whose stem isn't exactly the name.

### Browser

- **Deferred** to browser idea #3. After a delete, the browser's cursor jumps to the top of the list, because `act` selects the deleted name. Select its neighbour instead.
- **Deferred** to browser idea #2. The browser rebuilds every overview, one after another, before each view. They only check directories and programs, so this is fast; build them concurrently when the live preview reworks the refresh.

### Paths

- **Left.** A symlink inside a root that leads out of it, as `/srv/shop/api` to `/data/api`, is frozen as the absolute `/data/api`. tmux and `/proc` report directories resolved, and nothing tells which link the shell went through. Editing the config keeps the link.

### macOS (ignored)

- **Ignored**, macOS only: the existing TODO about case-insensitive filesystems is a real bug. Freezing `shop` when `Shop.yml` exists overwrites it without the replace prompt, because `Names()` returns "Shop".
