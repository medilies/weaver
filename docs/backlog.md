# weaver backlog

Everything left to do or decide, in the order to do it: the browser
ideas, the config ideas and what is left of the quality report (review
of 2026-11-06, revised 2026-11-07), merged on 2026-11-08, then the
first brief and its later notes, sorted on 2026-11-09, and the first
use of tasks, scheduled on 2026-11-09. Their history is
in `git log -- docs/ideas.md docs/config-ideas.md
docs/quality-report.md specs.md`.

## How the order is chosen

From the project rules in `CLAUDE.md`:

- **Finish what exists before adding.** Test gaps come first; a
  deferred finding goes with the task it waits for.
- **Show what the config already says before adding to it.** The
  browser can draw far more than it does, with no change to `domain` or
  `app`.
- **Domain before forms.** Rules go into `domain` before a form or the
  browser needs them; the forms only collect answers.
- **Config fields only when a config needs them.** A field no config
  uses yet waits in [By need](#by-need), with what would call for it.
- **weaver runs only what a config says, and knows nothing of git.**
  Ideas that act on the user's behalf, or need git, are
  [Parked](#parked).
- **Words first.** A new concept gets a glossary entry in `CLAUDE.md`
  before it gets code.

Sizes: XS (a line or two), S (up to about 30 lines plus a test, in one
or two layers), M (up to a few hundred lines, or several layers), L (all
layers), XL (mostly open questions).

## Order

Nothing scheduled. The exited-command warning is done (v0.4.0); what comes next
is picked from [By need](#by-need) when its trigger fires.

## By need

### Settings (by item)

**When**: one of these bothers you enough to want it changed. Each is a
weaver-wide setting, not a project's config, so the first one brings a
settings file, which needs a glossary entry first.

- The config form's keys for splitting panes (tmux's `%` and `"` today),
  or reading them from the user's tmux bindings.
- The selected colour (cyan), the config form's layout.

Each waits for a config or a user that needs it; the trigger says what
that looks like. They are listed from the smallest.

### Typing an exited command again (S)

**When**: a crashed dev server is restarted by hand often enough that
reaching its pane is the slow part. A browser key on a pane with the
exited-command warning types its command into it again, as a task is
typed: through `TaskRunner.Type`, after `Busy` says the shell is idle.

### A mark on a running task (S)

**When**: someone loses track of which tasks still run. The task list
marks a task whose pane is busy, from the same idle check
(`SessionInspector.Idle`). A task run apart in a throwaway window isn't
its pane's, so it needs the window's own pane checked.

### A clean shutdown when killing (M)

**When**: a dev server leaves state behind (a lock file, a stale PID,
an unflushed log) when its session is killed. After the teardown
commands, the kill sends Ctrl-C to every pane and waits, with a
timeout, until each shell is idle, then kills the session. It relies on
the idle check meaning "the shell is idle", as `Idle` and `Busy` do.

### A failed teardown seen from inside the session (XS)

**When**: `weave kill` run in a pane of the session it kills hides a
failing teardown command, since the pane goes with the session. weave
would print the error and wait for Enter before the kill.

### The config in the browser (XS)

**When**: someone misses the config's YAML in the details pane, which
the drawing replaced. `E` opens the file, and a config that can't be
read shows why. Bring it back behind a key (`y`):
`ProjectOverview.Config()` still carries it.

### Setup and teardown in the config form (S)

**When**: someone wants to change setup or teardown commands without
`weave edit --raw`. The config form keeps them as they are; the project
card would list them like `before`, added, edited, removed and
reordered the same way.

### Grid sizes in cells (S)

**When**: a config wants a fixed-width column or row beside ones that
share the rest, such as a 30-cell file tree.

A grid's `columns` and `rows` take `%` and `fr` only: a split's `-l`
is the new pane's size, so a cell size on a leading track depends on
the window's width. Either size the split from the terminal's size
(`TmuxOptions.Size`), split with `-b` so the new pane is the fixed one,
or apply a `select-layout` string with tmux's checksum. Freeze would
still write splits; writing a grid when a session fits a small one is
its own decision.

### The browser full screen inside tmux (S to M)

**When**: running `weave` in a small pane keeps getting in the way.
Inside tmux, `weave` runs in the pane it was typed in. Three ways, from
the least code:

- **A binding the user writes**, no code: `bind-key W display-popup -E
  -w 100% -h 100% weave`. weave already works in a popup (no
  `$TMUX_PANE`), switching sessions included. Could go in the README.
- **weave re-running itself in a full-size popup** when inside tmux.
  It acts on the user's behalf, so it is a weaver-wide setting, the
  first (see [Settings](#settings-by-item)).
- **The browser in its own tmux session**, weave switching to it and
  back. Open questions: its name clashes with a project's session (a
  project named `weaver` has the session `weaver`); it shows in tmux's
  session list and among the sessions `weave freeze` offers to capture;
  weaver would start a session that isn't a project's, so *owned
  session* must leave it out; and whether it is killed or kept once a
  project opens. Needs a glossary entry first.

### Window miniatures (S)

**When**: projects with many windows, and a details pane tall enough
to spare the lines.

The window list above the drawing becomes small drawings of every window, the
selected one highlighted:

```text
 api              app              repo
┌──────┬────┐    ┌──────┬────┐    ┌───────────┐
│      │    │    │      │    │    │           │
│      ├────┤    │      ├────┤    │           │
└──────┴────┘    └──────┴────┘    └───────────┘
```

`drawing` already draws at any size; this is `windowList` calling it small.

### Window `env` (S)

**When**: a config sets the same variable in every pane of a window.

```yaml
windows:
  - name: api
    env: { APP_ENV: local }
```

`before` already applies at three levels; `env` skips the window. Each
variable is set by the project, then the window, then the pane, the last
one winning.

- `domain`: `Window` gets an `env` and a `WithEnv`. `planPane`
  (`domain/plan.go`) becomes
  `p.env.Overlay(window.env).Overlay(pane.env)`.
- `infra`: `windowFile.Env`, `windowFileOf`, and
  `$ref: "#/definitions/env"` in the window's schema.
- Test: a window's variable overrides the project's, and a pane's
  overrides the window's.
- The browser's levels and the config form show the extra level once
  it exists.

### Focus (S)

**When**: the pane you work in isn't the first one created, such as a
wide editor split off a narrow log pane.

```yaml
windows:
  - name: shell
  - name: editor
    focus: true
    panes:
      - name: logs
      - name: nvim
        split: { direction: right, size: 70% }
        focus: true
```

Windows and splits are created with `-d` (`Tmux.newWindow`, `Tmux.fill`),
so you always land in the first window's first pane. That stays the
default. tmuxinator has it as `startup_window` and `startup_pane`, tmuxp
as `focus: true`.

- `domain`: at most one focused window per project and one focused pane
  per window, checked in `WithWindows` and `NewWindow` (two new errors).
  The plans carry the choice.
- `infra`: `Tmux.Start` ends with a `select-pane` per window and one
  `select-window`. `Tmux.Restore` selects the focused pane in the
  windows it adds, without switching away from the window you're in.
- Freeze reads `#{window_active}` and `#{pane_active}` in `Capture`, so
  a captured session keeps where you were.
- Schema: `focus` boolean on windows and panes.
- Test (tmux): after `Start`, the focused window and pane are active.

### Grid windows in the config form (M)

**When**: someone wants to change a grid window's layout without
editing its YAML.

The config form draws a grid window and selects its panes, but its
keys that split, remove and resize panes are refused: the domain
refuses changing a grid's panes (`ErrChangingGridPanes`), and the form
shows its areas as text. Editing it as a grid means changing its
`areas`, `columns` and `rows`, with new `Grid` methods in `domain`
first. Turning a grid window into splits, or back, is an action of its
own, never a side effect of another key. Saving a grid window lists its
panes in creation order, not as the config listed them; keep the
config's order if the form makes that visible.

### Secrets read at start (M)

**When**: a config would hold a secret.

```yaml
env:
  APP_ENV: local
  DB_PASSWORD: { from: pass show shop/db }
```

`env` values are literal, so a secret ends up in plain YAML, likely in a
dotfiles repo. `from` runs a command the config names and uses its
output.

- Where the command runs matters more than how it is parsed. In the
  pane's startup script (`shellCommand`, `infra/tmux_script.go`), as
  `DB_PASSWORD="$(pass show shop/db)" || printf …; export DB_PASSWORD`.
  The value then never passes through weaver, tmux's arguments or `ps`;
  only the command does. Resolving it in weaver and exporting the value
  would put it in the `sh -c` argument, visible in `ps` and in
  `#{pane_start_command}`.
- What that costs: the command runs once per pane (`pass` and `op`
  cache their unlock, so usually fine), and a failure leaves the
  variable empty and prints a warning in the pane instead of stopping
  the open.
- `domain`: an `EnvVar` is either literal or from a command; `Overlay`
  doesn't change.
- `infra`: a value decodes as a string or `{from: …}` (a custom
  `UnmarshalYAML`), and the schema uses `oneOf`.
- The browser summary and `weave show` print
  `DB_PASSWORD from pass show shop/db`. weaver never has the value to
  print.
- The warnings can check that the `from` program is on `PATH`.

### Live preview (M)

**When**: the exited-command warning isn't enough to tell whether a
pane is still doing its job, or a task's output is wanted without
leaving the browser.

Like tmux's `choose-tree` preview: `capture-pane` each pane of a
running session and fill the drawing's boxes with their last lines,
drawn from tmux's real geometry instead of the plan, so panes closed or
added show against the config. `infra/tmux_layout.go` already parses
`#{window_layout}` for freeze, so the geometry costs almost nothing;
what is left is `capture-pane` on the tmux port and a refresh while the
browser is open. The refresh loop is when to build the overviews
concurrently: the browser rebuilds every overview, one after another,
before each view, which is fast while they only check directories and
programs.

### `wait` (M)

**When**: a pane's command must run only once something answers: the
front end after the API, the migrations after Postgres. Comes after
setup commands, which cover setup that must finish before the panes
open; `wait` orders the panes among themselves.

```yaml
      - name: front
        dir: front
        wait: { port: 8000 }               # localhost:8000 accepts connections
        command: npm run dev
      - name: migrate
        dir: back
        wait: { port: 5432, timeout: 60s }
        command: php artisan migrate
```

- weaver exits once it attaches, so it can't do the waiting. The pane
  has to.
- A hidden subcommand, `weave wait tcp localhost:8000 --timeout 60s`,
  typed as `<weave> wait … && npm run dev`, with weave's absolute path
  (`os.Executable`). It needs no `nc` or `curl`, works in any shell,
  shows what it waits for, and Ctrl-C skips it. The alternative, waiting
  in the startup script, keeps the history clean, but the keys weaver
  types meanwhile echo on screen until the shell reads them.
- `domain`: `Pane.Waiting(…)`, kept apart from the command, so that
  `Command.Program()` and the warnings still see `npm`, not `weave`.
  Only with a `command` at first: waiting before a `prefill` relies on
  type-ahead, which is uncertain.
- Start with `port` (host default `localhost`) and `timeout`; `http` (a
  2xx answer) if needed later.
- Freeze can't see waits: replacing a config drops them, as it drops
  comments.
- A new subcommand reserves its name (`reservedNames` in
  `domain/project.go`), which breaks a registered project already named
  `wait`. Released configs may be, so decide how: a migration message,
  or no new subcommand.

### Lazy windows (L)

**When**: a project has heavy windows you don't always want (an
emulator, a log tail). It wants a second argument to `weave`, which
takes at most one today (`cmd/weave/root.go`); design it with
`weave run` below if both come.

```yaml
windows:
  - name: app
  - name: emulator
    lazy: true
    panes:
      - prefill: nimbus emulator
```

A lazy window is in the config, but `weave nimbus` doesn't create it;
`weave nimbus emulator` (the window completed from the config), or the
browser, does.

- `domain`: `Window.Lazy()`. `Plan()` leaves lazy windows out, and
  `Restoration` doesn't bring back a missing lazy window. Opening one on
  request uses the same placement rule (after the window listed before
  it), so split that logic out of `Restoration` into a placement for one
  window. At least one window must not be lazy.
- `app`: a new use case, `OpenWindow`. It starts the session without
  its lazy windows if needed, adds the window, then attaches with it
  selected.
- Browser: lazy windows marked in the window list (`emulator · lazy`), and a
  key with a small picker to open one (a new `Action`).
- The warnings check a lazy window only when it is being opened.
- Freeze from a session where the window isn't open drops it from the
  config it writes: warn, or carry lazy windows over from the existing
  config.

### `weave run` (S)

**When**: running a task from the command line, without the browser,
comes up. `weave run <project> <task>`, both completed. It reserves
`run` as a project name, as `wait` would, and shares its second-argument
syntax with lazy windows.

## Parked

Against the project's rules as they stand. Each says what would have to
change to bring it back.

- **Plugins, the long-term goal.** Extraordinary features through
  plugins, maintained by external maintainers and interested teams,
  without loading the core with code. That needs a solid pluggable
  system first; variables and hooks are steps towards it, and the git
  plugin and worktree variants below wait on it. Deferred on 2026-11-09:
  tasks come first. *plugin*, *hook* and *variable* need glossary
  entries before any design.

- **Most recently opened first, in the list.** weaver would keep a
  record of when each project was opened, besides the configs. Back if
  the list grows long enough that the filter isn't enough.
- **`history`**: put a pane's usual commands in its shell history, so ↑
  brings them back. weaver would type into the shell's history and leave
  lines piling up in `$HISTFILE` (bash and zsh save it on exit), which
  the user never wrote as commands to run, and each shell needs its own
  syntax (bash `history -s`, zsh `print -s`, fish neither). `note`
  already shows them, and tasks run them. Back if `note` and tasks prove
  not enough; the shell details (`ignorespace`, `HIST_IGNORE_SPACE`,
  `erasedups`, ordering) are in `docs/config-ideas.md` in the history.
- **Hooks beyond setup and teardown.** A general hook mechanism is an
  abstraction with one user so far. Setup and teardown commands are the
  first two hooks; a third need is when to design the general shape
  (one `hooks:` key or top-level keys, what weaver hands a hook as
  environment variables, such as `WEAVER_PROJECT` and `WEAVER_ROOT`).
  *hook* needs a glossary entry first.
- **A git plugin.** weaver knows nothing of git (`5e8f663`, `0355ed1`,
  then `repositories:` removed). Cloning missing repositories can come
  back only as a separate program, such as `weaver-git`, named in a
  config's setup commands (`weaver-git clone`), keeping its own list of
  repositories. Showing a branch needs another hook: a command whose
  output is a line of the browser's summary, with a time limit, since
  setup commands run once with output to the terminal. It isn't
  part of `weave`; plugins found on `PATH` as git finds `git-foo` would
  add dispatch code to the core. *plugin* needs a glossary entry first.
- **Worktree variants** (`weave shop@fix-123`: the shop layout on a
  branch, beside the main session). Mostly git, so only through the git
  plugin. Open questions: which repositories get the branch; per-variant
  ports (a `WEAVER_VARIANT` variable, or `env` overrides per variant);
  `@` in names (free in project names, check tmux `-t` targets, and the
  `@weaver_project` tag and `Find`'s check of who started the session
  must include the variant); removing worktrees with uncommitted
  changes. A root that holds no repository can't have variants.

## Left and ignored

Not worth the code, or out of scope. Kept so they aren't reported again.

**Ignored** until a user needs it:

- **macOS.** `os.UserConfigDir()` returns `~/Library/Application
  Support` there and ignores `XDG_CONFIG_HOME`, so the help text
  (`cmd/weave/root.go`) and README promise the wrong location. Either
  document the macOS path, or resolve `$XDG_CONFIG_HOME` (falling back
  to `~/.config`) on every OS, which moves existing macOS configs and
  would have to keep reading the old directory. On a case-insensitive
  filesystem, freezing `shop` when `Shop.yml` exists overwrites it
  without the replace prompt (the TODO in `domain/project.go`). CI
  builds only for Linux, though darwin binaries ship.
- **Any tmux but 3.4.** CI and releases run on ubuntu-24.04 for Ubuntu's
  tmux 3.4, not ubuntu-latest; re-pin when GitHub retires that runner.
  Another version may differ in what weaver reads back: 3.4 prints a `\`
  before some `$` (see `verbatim` in `infra/tmux.go`). Older than 3.2
  fails for sure: restoration needs `new-window -b`.

**Left**:

- **A tmux version check when weave runs.** Follows from the above.
- **Schema stricter than the code.** `ParseSize` uses `strconv.Atoi`, so
  it accepts `+5`, `040` and ` 40% `, which the schema rejects;
  `ParseDirection` and the root accept surrounding whitespace. That is
  the safe way round: the editor warns, and weave still loads the
  config. Tightening the parsers could reject released configs that load
  today, and every other value object trims its input (`oneLine`).
- **`weave edit` races.** Edits are lost if the config is deleted while
  the editor is open (`ErrProjectNotFound`), and nothing protects
  against a lost update from a concurrent `weave freeze`.
- **Config file names that differ only by spaces.** `shop .yml` lists as
  `shop`, because `NewProjectName` trims; `FindByName` then reads
  `shop.yml`, and with both files present `Names()` lists `shop` twice.
  The fix would skip, with a warning, any file whose stem isn't exactly
  the name.
- **Freezing a pane with a job.** It is frozen in its shell's directory,
  read from `/proc`, so `make -C sub dev` from the root keeps the root.
  A subshell that changes directory first loses it:
  `(cd api && exec sleep 300)` is frozen with the root and the prefill
  `sleep 300`. A shell a wrapper forks, as a `default-command` of
  `sh -c 'bash -i; true'` does, is taken for a job. Telling shells apart
  would be guesswork.
- **Freezing a pane started as another shell.** `tmux new-window zsh`
  where `default-shell` is bash is frozen with `zsh` typed at its
  prompt, for the same reason.
- **A symlink in a root that leads out of it.** `/srv/shop/api` to
  `/data/api` is frozen as the absolute `/data/api`: tmux and `/proc`
  report directories resolved. Editing the config keeps the link.
- **GoReleaser's version format.** It stamps `{{.Version}}` (`0.2.0`),
  while `go install …@v0.2.0` reports `v0.2.0`. Stamping `{{.Tag}}`
  gives one form, and `TestReleaseBuildsReportTheStampedVersion` would
  expect `v0.2.0`.
- **The changelog filters.** `^docs:`, `^test:` and `^chore:` miss
  scoped and other types, so `build(deps): …`, `ci: …` and merge commits
  reach the release notes. If they should go:
  `^(docs|test|chore|ci|build)(\(.+\))?!?:` plus `^Merge`.
- **Where `requireTool` lives**: in `local_workspace_test.go`, though
  the tmux tests use it too.

**Left out** of the config, as other session managers have them:

- `layout` presets (`tiled`, `main-vertical`): splits already describe
  more, and a grid writes them out explicitly.
- A flex layout (nested rows and columns of sized panes): it is
  `#{window_layout}`'s tree, saying what splits already say, more deeply
  nested. `wrap`, `gap`, `justify-content` and `align-items` mean
  nothing to tmux, whose panes always fill the window, so the name
  promises more than it can do. A grid is the other notation.
- `synchronize-panes`, a raw tmux `options` field: rarely needed.
- `sleep` delays (tmuxp's `sleep_before`): `wait` does the same job more
  reliably.
- Templates and variables passed when opening (tmuxinator's ERB
  `key=value`, smug's `-v`): there is one config per project, and `env`
  covers the rest.
- A config committed inside the repository (`.tmuxp.yaml`,
  `.laio.yaml`): it breaks "one file per project in the config folder",
  and only pays off when a team shares it.
- Saving pane contents to bring back after a reboot (tmux-resurrect):
  weaver rebuilds from the config, which is the better source of truth.
- Opening some windows from the command line (smug's `proj:win1,win2`):
  restoring brings back closed windows, and lazy windows put the choice
  in the config.

## Shared groundwork

- The drawing's border drawing (`infra/tea_drawing.go`) serves the live
  preview.
- `CommandRunner` (`infra.ShellRunner`) serves setup and teardown
  commands, and any later hook.
- The idle-shell check serves tasks (`Busy`), the exited-command
  warning (`Idle`) and the clean shutdown after teardown.
- One tmux pane listing (`projectPanes`) serves the exited-command
  warning and tasks; `Capture` could use it too, but still asks tmux four
  or five times per pane: not worth changing while freeze is
  interactive, since splitting free-text paths and labels out of one
  line adds parsing risk.
- A second CLI argument serves lazy windows and `weave run`: design both
  together.
- The `sh` startup script (`shellCommand`) carries secrets, and maybe
  `wait`.
