# weaver backlog

Everything left to do or decide.

## How the order is chosen

Every candidate stands on the same footing: none waits for a trigger,
none is deferred. They are ordered by complexity, the smallest first,
and, at the same size, by necessity: what fixes a nuisance felt today
before what adds something new. The exceptions: what waits
[until a user asks](#until-a-user-asks), since the browser already does
it, [Parked](#parked) ideas, which break a project rule as it
stands, [Plugins](#plugins), which live outside the core, and
[macOS](#macos), its own list of work.

`CLAUDE.md`'s rules still shape how each is built; ideas that run what
no config says, or need git, are [Parked](#parked).

Sizes: XS (a line or two), S (up to about 30 lines plus a test, in one
or two layers), M (up to a few hundred lines, or several layers), L (all
layers), XL (mostly open questions).

## Order

v0.5.0 is done: window `env`, focus, setup and teardown in the config
form, the config in the browser, a failed teardown seen from inside the
session, rerun and a mark on a running task.

v0.6.0 is done: settings (the selected colour and the drawing keys),
grid sizes in cells, grid windows edited and turned into splits in the
config form, and window miniatures.

v0.7.0 is done: bind (formerly freeze), edit, list and show left to
the browser, the screens styled by one theme, the live preview, start, running a task while staying
in the browser, the task list's ● marks kept up to date, running
projects marked by a green name, and `weave kill` safe with a zoomed
window on tmux 3.4.

Since v0.7.0: the browser kept on screen while an action runs, `w` to
run a task in a throwaway window, the terminal tests, run in a real
terminal emulator,
configs ending in `.yaml`, the config form unit-tested through its
model, releases as deb packages in a signed apt repository, and a tmux
command given up on after 5s.

## Candidates

Each says what it is for.

### The browser full screen inside tmux (S to M)

**Use**: running `weave` in a small pane keeps getting in the way.
Inside tmux, `weave` runs in the pane it was typed in. Three ways, from
the least code:

- **A binding the user writes**, no code: `bind-key W display-popup -E
  -w 100% -h 100% weave`. weave already works in a popup (no
  `$TMUX_PANE`), switching sessions included. Could go in the README.
- **weave re-running itself in a full-size popup** when inside tmux.
  A setting, off unless set: `popup`, below.
- **The browser in its own tmux session**, weave switching to it and
  back. Open questions: its name clashes with a project's session (a
  project named `weaver` has the session `weaver`); it shows in tmux's
  session list and among the sessions `weave bind` offers to capture;
  weaver would start a session that isn't a project's, so *owned
  session* must leave it out; and whether it is killed or kept once a
  project opens. Needs a glossary entry first.

### Settings: the theme, and the others they could hold (S each, M for the theme)

**Use**: weaver's look and keys are fixed but for the selected colour
and the drawing keys, so a terminal theme where yellow is unreadable,
or keys that clash with a user's habits, can't be changed.

Settings today ([Settings](settings.md)): `selected-colour`, and
`keys:` for the drawing (`split-right`, `split-below`, `remove`,
`shrink`, `grow`).

**Editing the theme** (M). The theme (`infra/theme.go`) is the
selected colour, the shades (bright, plain, muted) and the state
colours (green, yellow, red). A `theme:` key would set each of them:

```yaml
theme:
  selected: "#5fafd7"   # replaces selected-colour, which stays read
  ok: 2                 # green: what runs or is fine
  warning: "#d7af5f"    # yellow: what would likely fail, or has exited
  problem: 1            # red: what fails
  bright: { bold: true }
  muted: { faint: true }  # or a colour, for terminals that ignore faint
  border: rounded       # rounded, normal, thick, double: the cards' borders
```

- Each value as `selected-colour` is today: an ANSI number or
  `"#rrggbb"`. The state colours default to the terminal's own basic
  colours, as now, so a terminal theme still sets them.
- `selected-colour` stays read, as released, and is the same as
  `theme.selected`; setting both is refused.
- The marks (●, ○, ✗, ›) stay as they are: they are words of the
  glossary, not styling.
- Open question: named themes, or a theme file of its own, for users
  sharing one.
- `theme` is already in the glossary; `theme:` as a settings key needs
  it to say so.

**Other possible settings**, S each, the default being today's
behaviour:

| Setting | Does | Default |
| --- | --- | --- |
| `keys:` for the browser | every browser key | today's keys |
| `keys:` for the rest of the config form | undo, redo, save, moving panes | today's keys |
| `preview` | whether the browser starts with the preview or the drawing (`p` still switches) | on |
| `preview-every` | how often the preview and the task list's marks are read again | `1s` |
| `miniatures` | whether the window list draws miniatures | on |
| `mouse` | whether the browser and the config form take clicks | on |
| `editor` | the editor `E` opens a config in, over `$VISUAL` and `$EDITOR` | `$VISUAL`, else `$EDITOR`, else `vi` |
| `tmux-timeout` | how long weave waits on a tmux command before giving up | `5s` |
| `popup` | inside tmux, the browser re-runs itself full screen in a popup | off |

- Settings stay weaver-wide: nothing here changes what a project's
  config means, so no project default (`clear`, `env`) belongs in them.
- Each comes with its field in `settings.schema.json` and a line in
  `docs/settings.md`.

### Reload (M)

**Use**: a pane got into a bad state, or its config changed, and the
only way back today is killing the project and opening it again.

In the browser, `R` asks in the status line: "Reload pane back, or the
whole api window? (p/w, anything else cancels)". Asking doubles as the
confirmation, since reloading ends what runs. Only in an owned session.

- A pane is restarted in place, its size and position kept, with its
  dir, env, `before` commands, `clear`, note and `command` or
  `prefill`, all from the config as stored now.
- A window is replaced in place, at the same position, its panes laid
  out again by its splits or grid, the pane `focus` sets selected.
- Throwaway windows of its tasks are left alone.
- A pane the session lacks is refused, pointing to reloading its window;
  a window the session lacks is restored, as open would.
- The pane the browser runs in, or the window holding it, is refused,
  as for tasks.
- You stay in the browser; the preview shows the result.
- It reuses restoration's placement and `projectPanes`/`pick` (see
  [Shared groundwork](#shared-groundwork)).

### Live overviews (M)

**Use**: while the browser is open, the summary and the project list's
● marks and green names show where things stood when it last built the
overviews, while the preview and the task list's marks beside them are
kept up to date. Building the overviews concurrently, on the browser's
tick (`previewTick`, with its numbered asks), keeps them as live as the
preview.

### A clean shutdown when killing (M)

**Use**: a dev server leaves state behind (a lock file, a stale PID,
an unflushed log) when its session is killed. After the teardown
commands, the kill sends Ctrl-C to every pane and waits, with a
timeout, until each shell is idle, then kills the session. It relies on
the idle check meaning "the shell is idle", as `Idle` and `Busy` do.
The pane weave runs in is left out, as for tasks.

### Secrets read at start (M)

**Use**: a config would hold a secret.

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
- The browser summary and details print
  `DB_PASSWORD from pass show shop/db`. weaver never has the value to
  print.
- Setup and teardown commands get the project's env from `ShellRunner`,
  in weave's own process: `from` either runs there, the value passing
  through weave, or is left out of their env.
- The warnings can check that the `from` program is on `PATH`.

### `wait` (M)

**Use**: a pane's command must run only once something answers: the
front end after the API, the migrations after Postgres. Setup commands
cover what must finish before the panes open; `wait` orders the panes
among themselves.

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
- `port` (host default `localhost`) and `timeout`.
- Bind can't see waits: replacing a config drops them, as it drops
  comments.
- A new subcommand reserves its name (`reservedNames` in
  `domain/project.go`), which breaks a registered project already named
  `wait`: see the reserved-name rule in
  [Shared groundwork](#shared-groundwork).

### Open once the panes are ready (L, M once readiness is settled)

**Use**: opening a project lands you in panes still working through
their `before` commands, `clear` and note, the screen scrolling under
you.

Open puts you in the session once every pane it created is ready: its
`before` commands ran, `clear` typed if set, its note printed, and its
`command` started or its `prefill` typed. A pane with none of these is
ready at once, so a plain project attaches as today.

- Meanwhile weave lists the panes not yet ready under `window › pane`,
  crossing each off as it gets there; enter or ctrl+c attaches at once.
  No time limit: a slow `npm install` in `before` is normal.
- Only open waits. Start leaves you where you are, so it doesn't.
  Reopening an owned session waits for the panes of the windows it
  restores, and only those.
- Always on, no config field.
- Open question: how weave tells a pane is ready. Start types
  everything ahead (`before`, then `clear` and the note if it clears,
  then the `command` or `prefill`) and
  the shell works through it alone, so weave can't tell from what it
  sent. The idle-shell check sees a shell at its prompt, but not
  whether lines typed ahead are still waiting, and a `command` pane is
  never idle once ready. Whatever the answer, it must not run anything
  the config doesn't say, nor leave traces in the pane's history.
- `wait`, below, stays separate from ready.

### Lazy windows (L)

**Use**: a project has heavy windows you don't always want (an
emulator, a log tail). It wants a second argument to `weave`, which
takes at most one today (`cmd/weave/root.go`).

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
- Bind from a session where the window isn't open drops it from the
  config it writes: warn, or carry lazy windows over from the existing
  config.

## Until a user asks

Done only when a user asks for them, since the browser already does
what they would.

### `weave edit`, `weave list` and `weave show` again (S)

**Use**: a user asks to edit, list or show configs from the shell, or a
script needs one of them. They left the CLI in v0.7.0 for the browser,
which does each ([ADR 0003](adr/0003-edit-list-and-show-leave-the-cli.md)).

- `weave edit [--raw] <project>`: the config form, or the config raw in
  the editor, keeping it only if valid; `Saved <project>.`, `No
  changes.` or `Nothing saved.`. `EditProject` still does both.
- `weave list`: each registered project and `running` or `-`, aligned.
  `ListProjects` still does it, for completion.
- `weave show <project>`: the config as stored, through
  `app.ProjectDocuments`.
- Their names are still reserved, so adding one back breaks no config.
  Bring back only the one asked for, and its terminal scenario.

### `weave run <project> <task>` (S)

**Use**: running a task from the shell or a script, without the
browser, which runs tasks. A new subcommand: see the reserved-name rule
in [Shared groundwork](#shared-groundwork).

## Parked

Against the project's rules as they stand. Each says what would have to
change to bring it back.

- **Most recently opened first, in the list.** The browser's list
  sorted by when each project was last opened, instead of by name, so
  the few projects in use are at the top without typing a filter.
  - It breaks the rule that weaver keeps nothing but the settings and
    the configs: it needs a record of when each project was opened,
    such as `$XDG_STATE_HOME/weaver/opened.yml` (`~/.local/state`
    unless set), written by every open. Bringing it back means allowing
    that record, and giving it a glossary entry.
  - Half of it needs no record: tmux knows when a session was last
    attached (`#{session_last_attached}`), so running projects could
    be sorted by that alone. Not-running ones, and those whose session
    was killed, would still fall back to their names.
  - Open questions: whether start counts as opening, and running a
    task; whether a config edited recently counts (its file's
    modification time would do without a record, but tells editing,
    not use); whether the order is a setting; and that the list then
    reorders under the cursor after each open, so the browser keeps
    the cursor on the project it was on.

## Plugins

Outside the core. The plugin system and hooks come first, as the
foundation; each plugin after them needs both. *plugin*, *hook* and
*variable* need glossary entries before any design.

- **Plugins, the long-term goal.** Extraordinary features through
  plugins, maintained by external maintainers and interested teams,
  without loading the core with code. That needs a solid pluggable
  system first; variables and hooks are steps towards it.
- **Hooks, foundational.** What plugins plug into: points in what
  weave does where it runs a command the config names. Setup and
  teardown commands are the first two; others would come where a plugin
  needs one. Designing them means the general shape: one `hooks:` key or
  top-level keys, and what weaver hands a hook as environment variables,
  such as `WEAVER_PROJECT` and `WEAVER_ROOT`.
- **A Silksong plugin.** weaver is named after the
  Weavers of Hollow Knight: Silksong; this plugin would swap the words
  weaver shows (browser, config form, CLI messages) for weaving and
  silk words, for those who want the theme. The core keeps the plain
  words of the glossary, but for *bind* (ADR 0002). Candidates:
  *unbind* for delete, *weave* for open, *mend* for restoration,
  *woven* for an owned session, *snapped thread* for an exited
  command, *fray* for a warning, *loom* for the browser, *spool* for
  the task list, *threading* for setup commands, *unravel* for
  teardown, *cut* for kill, *pattern* for a project, *warp* and *weft*
  for a grid's columns and rows. Generic words only, never the game's
  proper names (Hornet, Pharloom, Needolin). It needs the shown words
  gathered in one place a plugin can replace; today they are spread over `infra/tea_*.go` and
  `cmd/weave`.
- **A git plugin.** weaver knows nothing of git. Cloning missing repositories can come
  back only as a separate program, such as `weaver-git`, named in a
  config's setup commands (`weaver-git clone`), keeping its own list of
  repositories. Showing a branch needs another hook: a command whose
  output is a line of the browser's summary, with a time limit, since
  setup commands run once with output to the terminal. It isn't
  part of `weave`; plugins found on `PATH` as git finds `git-foo` would
  add dispatch code to the core.
- **Worktree variants** (`weave shop@fix-123`: the shop layout on a
  branch, beside the main session). Mostly git, so only through the git
  plugin. Open questions: which repositories get the branch; per-variant
  ports (a `WEAVER_VARIANT` variable, or `env` overrides per variant);
  `@` in names (free in project names, check tmux `-t` targets, and the
  `@weaver_project` tag and `Find`'s check of who started the session
  must include the variant); removing worktrees with uncommitted
  changes. A root that holds no repository can't have variants.

## macOS

What weaver needs to work as documented on macOS, where darwin binaries
ship but nothing is tested.

- **The config location.** `os.UserConfigDir()` returns
  `~/Library/Application Support` there and ignores `XDG_CONFIG_HOME`,
  so the help text (`cmd/weave/root.go`) and the docs promise the wrong
  location. Either document the macOS path, or resolve
  `$XDG_CONFIG_HOME` (falling back to `~/.config`) on every OS, which
  moves existing macOS configs and would have to keep reading the old
  directory.
- **Names differing only in case.** On a case-insensitive filesystem,
  `shop` and `Shop` are one config (the TODO in `domain/project.go`):
  binding `shop` when `Shop.yml` exists finds that config and asks
  before replacing it, but the two names still overwrite each other.
  Comparing names case-insensitively would fix it; it also covers
  reserved names (`Edit` passes today).
- **CI.** It builds and tests only on Linux. A macOS runner, with tmux
  from Homebrew, would run the tests; the terminal tests only if tui-test
  ships an engine for macOS.

## Left and ignored

Not worth the code, or out of scope. Kept so they aren't reported again.

**Ignored** until a user needs it:

- **Any tmux but 3.4.** CI and releases run on ubuntu-24.04 for Ubuntu's
  tmux 3.4, not ubuntu-latest; re-pin when GitHub retires that runner.
  Another version may differ in what weaver reads back: 3.4 prints a `\`
  before some `$` (see `verbatim` in `infra/tmux.go`). Older than 3.2
  fails for sure: restoration needs `new-window -b`. Hence no tmux
  version check when weave runs.

**Left**:

- **Unit tests of the browser's model.** Its projects come as
  `app.ProjectOverview`s, which only `app.BrowseProjects` builds, from a
  session inspector: an `infra` test would wire the whole use case to
  show one screen. Its choices are tested in `app`, through a scripted
  browser, and its screens by the terminal tests.
- **Charm's `teatest`** (`github.com/charmbracelet/x/exp/teatest/v2`).
  Its golden files repeat the `LooksAsBefore`
  snapshots, which also check a real terminal's drawing; its `WaitFor`
  only helps with late output, and the config form has none, while the
  preview needs tmux anyway. Experimental, with no tagged version, and
  built against a Bubble Tea v2 release candidate.
- **Schema stricter than the code.** `ParseSize` uses `strconv.Atoi`, so
  it accepts `+5`, `040` and ` 40% `, which the schema rejects;
  `ParseDirection` and the root accept surrounding whitespace. That is
  the safe way round: the editor warns, and weave still loads the
  config. Tightening the parsers could reject released configs that load
  today, and every other value object trims its input (`oneLine`).
- **Editing races.** Edits are lost if the config is deleted while
  the editor is open (`ErrProjectNotFound`), and nothing protects
  against a lost update from a concurrent `weave bind`.
- **Binding a pane with a job.** It is captured in its shell's directory,
  read from `/proc`, so `make -C sub dev` from the root keeps the root.
  A subshell that changes directory first loses it:
  `(cd api && exec sleep 300)` is captured with the root and the prefill
  `sleep 300`. A shell a wrapper forks, as a `default-command` of
  `sh -c 'bash -i; true'` does, is taken for a job. Telling shells apart
  would be guesswork.
- **Binding a pane started as another shell.** `tmux new-window zsh`
  where `default-shell` is bash is captured with `zsh` typed at its
  prompt, for the same reason.
- **A symlink in a root that leads out of it.** `/srv/shop/api` to
  `/data/api` is captured as the absolute `/data/api`: tmux and `/proc`
  report directories resolved. Editing the config keeps the link.

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

- `CommandRunner` (`infra.ShellRunner`) serves setup and teardown
  commands, and any later hook.
- The idle-shell check serves tasks (`Busy`), the exited-command
  warning (`Idle`) and the clean shutdown after teardown.
- One tmux pane listing (`projectPanes`) and its matching rule (`pick`)
  serve the exited-command warning, tasks and the preview; `Capture`
  could use it too, but still asks tmux four or five times per pane:
  not worth changing while bind is interactive, since splitting
  free-text paths and labels out of one line adds parsing risk.
- Restoration's placement of one window serves lazy windows and reload.
- The browser's tick (`previewTick`), with its numbered asks, serves the
  preview and the task list's marks, and would serve live overviews.
- The reserved-name rule, for the first new subcommand: when a registered project's name clashes with it, weave
  refuses with an error naming the config file to rename, and only when
  such a config exists. It becomes an ADR when that subcommand ships.
- The `sh` startup script (`shellCommand`) carries secrets.
