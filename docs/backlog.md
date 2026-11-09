# weaver backlog

Everything left to do or decide, in the order to do it: the browser
ideas, the config ideas and what is left of the quality report (review
of 2026-11-06, revised 2026-11-07), merged on 2026-11-08, then the
first brief and its later notes, sorted on 2026-11-09, the first use
of tasks, and v0.5.0, both scheduled on 2026-11-09, then v0.6.0,
scheduled on 2026-11-09 with the triggers dropped. Their history is
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
- **Config fields only when a config needs them.** Each of the
  [Candidates](#candidates) says the use that calls for it.
- **weaver runs only what a config says, and knows nothing of git.**
  Ideas that act on the user's behalf, or need git, are
  [Parked](#parked).
- **Words first.** A new concept gets a glossary entry in `CLAUDE.md`
  before it gets code.

Sizes: XS (a line or two), S (up to about 30 lines plus a test, in one
or two layers), M (up to a few hundred lines, or several layers), L (all
layers), XL (mostly open questions).

## Order

v0.5.0 is done: window `env`, focus, setup and teardown in the config
form, the config in the browser, a failed teardown seen from inside the
session, rerun and a mark on a running task.

v0.6.0, scheduled on 2026-11-09: settings, and grids finished. No new
command; breaking changes are allowed, but none is planned. The
backlog's triggers are dropped: what comes next is picked from the
[Candidates](#candidates) by judgement, each still with a concrete use.

### 2. Settings (M)

Weaver-wide preferences, never a project's config (glossary:
*settings*, *selected colour*).

```yaml
# yaml-language-server: $schema=settings.schema.json
selected-colour: "#5fafd7"     # or an ANSI number, 0-255; cyan unless set
keys:
  split-right: "|"
  split-below: "-"
  remove: x
  shrink: H
  grow: L
```

- `os.UserConfigDir()/weaver/settings.yml`, beside `projects/`, edited by
  hand only: no command, no form, no `--raw`. A missing file means the
  defaults.
- weave writes `settings.schema.json` wherever it writes
  `weaver.schema.json`, so it is there before the file is; the docs show
  the `$schema` line to start the file with. A test checks the defaults
  only use declared fields.
- An invalid file stops weave with an error naming the file and the
  field, as an invalid config is refused. A key already taken in the
  config form (`u`, `j/k/h/l`, `enter`, …), or two actions on one key,
  is refused, naming both.
- `infra` only: an `infra.Settings` loaded by `cmd/weave` and handed to
  `TeaConfigForm` and the browser. No port: no use case reads it.
- The keys: the five drawing keys of the config form
  (`pressedOnDrawing`), never read from the user's tmux bindings (ADR
  0001). The colour: every `lipgloss.Color("6")` (`pickedStyle`,
  `selectedStyle` and the drawing's); green, red and yellow stay.
- "The config form's layout", listed before as a setting, is dropped
  until someone can say what it would change.
- Docs: `browser.md` and `config-form.md` say "selected colour" instead
  of "cyan"; a settings page beside `config.md`.

### 3. Grid sizes in cells (S)

A fixed-width column or row beside ones that share the rest, such as a
30-cell file tree:

```yaml
grid:
  areas:
    - [tree, editor]
  columns: [30, 1fr]
```

- A bare number is cells, as for splits and tmux's `-l`. An axis with a
  cell track needs a `%` or `fr` track to take the rest; all cells is
  refused (a new error), as the first pane takes what splits leave.
- How tmux gets them: a split's `-l` is the new pane's size, so a cell
  size on a leading track depends on the window's width. Spike first:
  apply a `select-layout` string with tmux's checksum, serialised from
  the grid's `LayoutCell`, and check what tmux does to a fixed track
  when the window's size differs from the string's. If it rescales it,
  fall back to sizing the splits from the terminal's size
  (`TmuxOptions.Size`).
- Schema: the track pattern takes a bare number. Freeze still writes
  splits.

### 4. Grid windows in the config form (M)

The config form draws a grid window and selects its panes, but refuses
its keys that split, remove and resize panes (`ErrChangingGridPanes`)
and shows its areas as text.

- The window's card edits `areas` (one line per row), `columns` and
  `rows` as text fields, each checked by `NewGrid` as it is typed; the
  drawing follows. The drawing's keys stay refused on a grid: reshaping
  it with keys would need `Grid` methods for adding tracks and merging
  cells, each with its own edge cases.
- `domain`: a grid window takes a new grid whose names match its panes
  (a pane renamed or added in the areas is a pane added or renamed,
  checked there, never in the form).
- Saving keeps the panes in the config's order, not their creation
  order, now that the pane list and the diff show it.

### 5. A grid turned into splits (S)

One explicit action in the config form, never a side effect of another
key: the grid's cut (`LayoutCell`, `CarvedPane`) becomes the window's
splits, as capture does. Not the other way round: not every split tree
has a tidy grid, and freeze already writes splits.

### 6. Window miniatures (S)

The browser's window list becomes small drawings of every window, the
selected one in the selected colour, when the details pane has about
five lines to spare, and stays names otherwise:

```text
 api              app              repo
┌──────┬────┐    ┌──────┬────┐    ┌───────────┐
│      │    │    │      │    │    │           │
│      ├────┤    │      ├────┤    │           │
└──────┴────┘    └──────┴────┘    └───────────┘
```

`drawing` already draws at any size; this is `windowList` calling it
small, keeping ○ and clicks (`hotspot`). The config form keeps its
window list of names, with `+ window`.

## Candidates

Each says what it is for; picked by judgement, not by a trigger.

### A clean shutdown when killing (M)

**Use**: a dev server leaves state behind (a lock file, a stale PID,
an unflushed log) when its session is killed. After the teardown
commands, the kill sends Ctrl-C to every pane and waits, with a
timeout, until each shell is idle, then kills the session. It relies on
the idle check meaning "the shell is idle", as `Idle` and `Busy` do.

### The browser full screen inside tmux (S to M)

**Use**: running `weave` in a small pane keeps getting in the way.
Inside tmux, `weave` runs in the pane it was typed in. Three ways, from
the least code:

- **A binding the user writes**, no code: `bind-key W display-popup -E
  -w 100% -h 100% weave`. weave already works in a popup (no
  `$TMUX_PANE`), switching sessions included. Could go in the README.
- **weave re-running itself in a full-size popup** when inside tmux.
  It acts on the user's behalf, so it is a setting, off unless set
  (see [Settings](#2-settings-m)).
- **The browser in its own tmux session**, weave switching to it and
  back. Open questions: its name clashes with a project's session (a
  project named `weaver` has the session `weaver`); it shows in tmux's
  session list and among the sessions `weave freeze` offers to capture;
  weaver would start a session that isn't a project's, so *owned
  session* must leave it out; and whether it is killed or kept once a
  project opens. Needs a glossary entry first.

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
- The browser summary and `weave show` print
  `DB_PASSWORD from pass show shop/db`. weaver never has the value to
  print.
- The warnings can check that the `from` program is on `PATH`.

### Live preview (M)

**Use**: the exited-command warning isn't enough to tell whether a
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

**Use**: a pane's command must run only once something answers: the
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
  `wait`: see the reserved-name rule in
  [Shared groundwork](#shared-groundwork). Deferred on 2026-11-09: its
  need isn't clear yet.

### Lazy windows (L)

**Use**: a project has heavy windows you don't always want (an
emulator, a log tail). It wants a second argument to `weave`, which
takes at most one today (`cmd/weave/root.go`). Deferred on 2026-11-09.

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
- **A Silksong plugin.** Added on 2026-11-09. weaver is named after the
  Weavers of Hollow Knight: Silksong; this plugin would swap the words
  weaver shows (browser, config form, CLI messages) for weaving and
  silk words, for those who want the theme. The core keeps the plain
  words of the glossary. Candidates: *mend* for restoration, *woven*
  for an owned session, *snapped thread* for an exited command, *fray*
  for a warning, *loom* for the browser, *spool* for the task list,
  *threading* for setup commands, *unravel* for teardown, *cut* for
  kill, *pattern* for a project, *warp* and *weft* for a grid's
  columns and rows. Generic words only, never the game's proper names
  (Hornet, Pharloom, Needolin). Waits on the plugin system above, and
  on the shown words being gathered in one place a plugin can replace;
  today they are spread over `infra/tea_*.go` and `cmd/weave`.

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
- `weave run <project> <task>`: running a task without the browser. No
  new commands (2026-11-09); the browser runs tasks.
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
- A second CLI argument serves lazy windows.
- The reserved-name rule, decided on 2026-11-09 for the first new
  subcommand: when a registered project's name clashes with it, weave
  refuses with an error naming the config file to rename, and only when
  such a config exists. It becomes an ADR when that subcommand ships.
- The `sh` startup script (`shellCommand`) carries secrets, and maybe
  `wait`.
