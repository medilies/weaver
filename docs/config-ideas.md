# Config ideas

Today's fields (`root`, `env`, `before`, `windows`,
`panes`, `dir`, `split`, `command`, `prefill`, `note`) describe how a
session looks when it opens. These ideas fill in what is missing: when
things run (setup, start order, shutdown), which windows or copies open,
and what sits around the terminal (secrets, one-off tasks). They are
ordered from the easiest to build to the hardest; see
[Suggested order](#suggested-order) for what to build first. Where
another session manager (tmuxinator, tmuxp, smug) already has the
idea, the section says so.

| # | Idea | Config | Touches | Effort |
| --- | --- | --- | --- | --- |
| 1 | Window `env` | `windows[].env` | domain, file, schema | S |
| 2 | Window `dir` | `windows[].dir` | domain, file, schema | S |
| 3 | Focus | `windows[].focus`, `panes[].focus` | domain, file, schema, tmux, freeze | S |
| 4 | Warn when a command exited | none | tmux, read model, browser | M |
| 5 | Secrets read at start | `env: {X: {from: …}}` | domain, file, schema, startup script, browser | M |
| 6 | `start` and `stop` | `start`, `stop` | domain, open and kill use cases, new port | M |
| 7 | `wait` | `panes[].wait` | domain, file, schema, tmux, hidden subcommand | M |
| 8 | `history` | `panes[].history` | domain, file, schema, tmux, per shell | M–L |
| 9 | Lazy windows | `windows[].lazy` | domain, new use case, tmux, CLI, browser | L |
| 10 | Tasks | `tasks` | all layers, plus tmux popups | L |
| 11 | Worktree variants | per-variant env | all layers, plus git worktrees | XL |
| 12 | Hooks and a git plugin | `hooks`, maybe | open use cases, `CommandRunner`, browser | M, plugin apart |

## 1. Window `env`

```yaml
windows:
  - name: api
    env: { APP_ENV: local }
```

`before` already applies at three levels (project, window, pane); `env`
skips the window. Each variable is set by the project, then the window,
then the pane, the last one winning.

**Implementation:**

- `domain`: `Window` gets an `env` and a `WithEnv`. `planPane`
  (`domain/plan.go`) becomes
  `p.env.Overlay(window.env).Overlay(pane.env)`.
- `infra`: `windowFile.Env`, `windowFileOf`, and
  `$ref: "#/definitions/env"` in the window's schema.
- Test: a window's variable overrides the project's, and a pane's
  overrides the window's.

## 2. Window `dir`

```yaml
  - name: api
    dir: api
    panes:
      - name: api shell
      - name: task dev
        split: { direction: right, size: 40% }
        command: task dev
      - name: psql
        dir: .
        split: { direction: below }
        command: nimbus psql
```

The nimbus example in the README sets `dir: api` and `dir: app` twice
each. A pane's `dir` replaces its window's. Both are relative to `root`,
not to each other, so a `dir` reads the same wherever it appears.

**Implementation:**

- The catch: `PaneDir`'s zero value is the root, and `NewPaneDir(".")`
  returns that zero value too, so "no `dir`" and `dir: .` can't be told
  apart. `Pane` needs to know whether its dir was set (a flag set by
  `Pane.In`), so that `dir: .` can opt out of the window's.
- `encodeProject` writes the window's `dir` as it is. Freeze could
  hoist a dir shared by every pane of a window, but needn't at first.
- The directory checks and `ProjectOverview.Warnings()` read the plan,
  so they follow without changes.

## 3. Focus

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
default. `focus` is for configs where the pane you work in isn't the
first one created, such as a wide editor split off a narrow log pane.
tmuxinator has it as `startup_window` and `startup_pane`, tmuxp as
`focus: true`.

**Implementation:**

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

## 4. Warn when a pane's command exited

No config. In the browser summary:

```text
 ⚠ api › task dev: back at the prompt, its command exited
```

weaver knows which panes it typed a command into
(`Startup.Submits()`). In a session weaver started, such a pane whose
shell is idle again means the server crashed or was stopped.

**Implementation:**

- `infra`: `foreground(pid)` already tells whether a pane's shell is
  idle; freeze uses it. A new tmux method lists a session's panes with
  their window tag (`@weaver_window`), label (`@label`) and pid.
- `domain`: a method on a session port (`SessionFinder` or a new one)
  returning, per window, the panes whose shell is idle.
- `app`: `ProjectOverview` gets the panes whose command exited. Only
  panes with a `command`, not `prefill` or a plain shell, and only in
  sessions weaver started.
- Don't compare the command line: `ps` loses quotes, pipes and
  variables (see `Capture`). A shell that is idle when it should be
  running something is enough.
- Caveat: a command you stopped on purpose (Ctrl-C) warns too. It is a
  hint, not an error.
- Later: a browser key that types the command again.

## 5. Secrets read at start

```yaml
env:
  APP_ENV: local
  DB_PASSWORD: { from: pass show shop/db }
```

`env` values are literal, so a secret ends up in plain YAML, likely in a
dotfiles repo. `from` runs a command and uses its output.

**Implementation:**

- Where the command runs matters more than how it is parsed.
  Recommended: in the pane's startup script (`shellCommand`,
  `infra/tmux_script.go`), as
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
  `DB_PASSWORD from pass show shop/db`. weaver never has the value
  to print.
- The health warnings can check that the `from` program is on `PATH`.

## 6. `start` and `stop`

```yaml
start:
  - docker compose up -d
stop:
  - docker compose down
```

Some things a project needs run outside its panes: containers started
with `-d`, daemons, ssh tunnels. `start` commands run in `root`, in
order, once, when `weave <project>` creates the session. `stop` commands
run in `root`, in order, before `weave kill` kills it. smug has
`before_start` and `stop`, tmuxinator `on_project_first_start` and
`on_project_stop`.

`weave kill` runs tmux's `kill-session` (`Tmux.Kill`). The shells get
SIGHUP, but anything that left the pane keeps running, so without
`stop` every kill leaks containers.

An earlier draft left `start` out because a pane's `command` can do the
same. It does it worse:

- It runs at the same time as the other panes, so they can't count on
  it without `wait` (#7).
- When it fails, the session still opens, half-working.
- It runs again each time restoring re-creates its window, and it takes
  up a pane.

`start` runs before anything is built, like the directory checks, so a
failure leaves no session behind.

**Implementation:**

- `domain`: `Project.Start()` and `Project.Stop()`, lists of `Command`.
  Add a new port to run a command in a directory with an env
  (`CommandRunner`). `infra` implements it with `sh -c`, with output to
  the terminal. Both lists use it.
- `before` commands don't apply to either list. They are typed into
  interactive shells (`nvm use` is a shell function), while these run in
  `sh`. The project's `env` does apply. Say so in the schema.
- `start`, in `app`: `OpenProject.start` runs the commands before
  `checkDirs`, so a `start` command can create a pane's directory, as
  cloning would, and before `sessions.Start`. A failing command stops the
  open, and nothing is built. Restoring windows and attaching don't run
  `start`.
- `stop`, in `app`: `KillProject` holds a `SessionKiller` and a `SessionLocator` today. It
  would also load the project from the `ProjectRegistry`. When the
  config can't be read, kill anyway, as now: you asked to kill it. When
  a `stop` command fails, still kill the session and report the failure.
- Browser: `enter` and `x` run their use case after the Bubble Tea
  program exits (`BrowseProjects.act`), so the output reaches the
  terminal. A failure returns to the browser as its notice, and the
  redraw hides the output. The error has to carry the command's last
  lines of stderr itself.
- The health warnings check that `start` and `stop` programs are on
  `PATH`, as they do for pane commands.
- Freeze can't see either list: replacing a config drops them, as it
  drops comments.
- Second step, optional: send Ctrl-C to every pane and wait, with a
  timeout, until its shell is idle (`foreground` again), so dev servers
  shut down cleanly before the kill.
- `d` deletes the config but keeps the session running, so no `stop`
  there.

## 7. `wait`

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

A pane's command runs once something answers: the front end after the
API, the migrations after Postgres. weaver then decides the order things
start in, not only where they go.

**Implementation:**

- weaver exits once it attaches, so it can't do the waiting. The pane
  has to.
- Recommended: a hidden subcommand, `weave wait tcp localhost:8000
  --timeout 60s`, typed as `<weave> wait … && npm run dev`, with weave's
  absolute path (`os.Executable`). It needs no `nc` or `curl`, works in
  any shell, shows what it waits for, and Ctrl-C skips it.
- Alternative: wait in the startup script, before the shell starts.
  That keeps the history clean, but the keys weaver types meanwhile echo
  on screen until the shell reads them.
- `domain`: `Pane.Waiting(…)`, kept apart from the command, so that
  `Command.Program()` and the health warnings still see `npm`, not
  `weave`. Allowed only with a `command` at first: waiting before a
  `prefill` relies on type-ahead, which is uncertain.
- Start with `port` (host default `localhost`); `http` (a 2xx answer)
  if needed later.
- Freeze can't see waits: replacing a config drops them, as it drops
  comments.

## 8. `history`

```yaml
      - name: front
        dir: front
        history:
          - npm i
          - npm run dev
```

The shop example uses `note` as a cheat sheet ("Usual commands: npm i /
npm run dev"). `history` puts those commands in the pane's shell
history, so ↑ brings them back ready to run.

**Implementation:**

- It depends on the shell, which weaver only knows from `$SHELL`: bash
  `history -s 'npm i'`, zsh `print -s 'npm i'`. fish differs: fall back
  to printing them as a note.
- weaver types these lines, so they show on screen and could land in the
  history themselves. Start each with a space, which bash's
  `ignorespace` (part of Ubuntu's default `ignoreboth`) and zsh's
  `HIST_IGNORE_SPACE` (not on by default) keep out, then `clear`.
- bash and zsh save the session's history to `$HISTFILE` on exit, so
  the added lines pile up across sessions unless duplicates are erased
  (`erasedups`, `HIST_IGNORE_ALL_DUPS`).
- Order: add these first, then `before`, then the command, so the
  pane's own command is the most recent entry.
- Tests need real bash and zsh in the private tmux server, skipped when
  missing.
- Tasks (#10) cover part of the same need without touching the shell.

## 9. Lazy windows

```yaml
windows:
  - name: app
  - name: emulator
    lazy: true
    panes:
      - prefill: nimbus emulator
```

A lazy window is in the config, but `weave nimbus` doesn't create
it. Heavy things (an emulator, containers, a log tail) start only when
you ask: `weave nimbus emulator`, or from the browser.

**Implementation:**

- `domain`: `Window.Lazy()`. `Plan()` leaves lazy windows out, and
  `Restoration` doesn't bring back a missing lazy window. Opening one on
  request uses the same placement rule (after the window listed before
  it), so split that logic out of `Restoration` into a placement for one
  window.
- At least one window must not be lazy: a session needs a window.
- `app`: a new use case, `OpenWindow`. It starts the session without
  its lazy windows if needed, adds the window, then attaches with it
  selected.
- CLI: `weave` takes at most one argument (`cmd/weave/root.go`). A
  second one would be the window, completed from the config. This
  clashes with tasks (#10): settle both syntaxes together.
- Browser: lazy windows listed in the summary (`emulator · lazy`), and
  a key with a small picker to open one (a new `Action`).
- Health warnings: check a lazy window only when it is being opened.
- Freeze from a session where the window isn't open drops it from the
  config it writes. Warn about it, or carry lazy windows over from the
  existing config.

## 10. Tasks

```yaml
tasks:
  migrate:
    dir: back
    command: php artisan migrate
  seed:
    dir: back
    command: php artisan db:seed
  deploy-staging:
    command: ./deploy staging
```

Named one-off commands, run with `weave run shop migrate` or from the
browser, in the right `dir`, with the project's `env` and `before`. It
works as a personal task runner for repos you can't add a Makefile to,
such as the acme ones.

**Implementation:**

- Where it runs: inside tmux, in a popup (`display-popup -E -d <dir>`)
  that stays open on exit to show the result. Outside tmux, in the
  current terminal.
- `before` commands are often shell functions (`nvm use`), so run
  `$SHELL -ic '<before>; <command>'`, not `sh -c`. Startup is slower
  (rc files are read), and `-i` behaves differently from one shell to
  another.
- `domain`: `Task` (name, dir, env, before, command), close to a pane
  without a split, reusing its value objects. `Project.Tasks()`.
- `app`: a `RunTask` use case, and a port to run it in a popup or the
  terminal.
- CLI: `weave run <project> <task>`, with both completed, and
  `weave run . migrate`. Settle the syntax with lazy windows' (#9).
- Browser: tasks listed in the summary; `t` to pick one and run it.
  The health warnings check task programs too.
- Schema: `tasks` maps a name to a task object.

## 11. Worktree variants

```sh
weave shop@fix-123   # the shop layout, on branch fix-123, beside the main session
```

Review a PR or try a fix without stopping what runs in the main
session. This one is mostly open questions:

- **Where**: a parallel root, such as `~/dev/shop@fix-123/`, with one
  `git worktree` per repository the git plugin (#12) knows of. With
  several repositories (back, front), which ones get the branch? Those
  that have it, the others staying on their default branch? A root that
  holds no repository can't have variants.
- **Ports**: two copies of `task dev` both want port 8000. Each variant
  needs its own values, either through a variable weaver exports
  (`WEAVER_VARIANT=1`, with `PORT=$((8000 + WEAVER_VARIANT))` in the
  command) or through `env` overrides per variant. That is the only
  config it needs.
- **Names**: project names don't allow `@`, which leaves it free for
  variants. Check that tmux accepts it in `-t` targets. The session's
  `@weaver_project` tag, and `Find`'s check of who started the session,
  have to include the variant.
- **Lifecycle**: `weave kill shop@fix-123` kills the session. Removing
  the worktrees, and refusing when they have uncommitted changes, is
  another command or flag.
- **Browser**: variants listed under their project, each with its own
  branch line.
- **Touches**: worktree add and remove, tmux, the CLI, completion and
  the browser. weaver no longer runs git (`0355ed1`), so the worktree half
  belongs in the git plugin (#12), not in the core.

## 12. Hooks and a git plugin

weaver runs only what a config says, and its core knows nothing of git
(`5e8f663`, `0355ed1`, then `repositories:` removed). What git did for
it before (cloning missing repositories, the branch line in the browser)
can come back without coming back into the core: weaver offers hooks,
and a git plugin uses them.

**Hooks**: commands weaver runs at named moments, written in the config
like every other command. `start` and `stop` (#6) are the first two,
built there; this idea only names the general shape. Other moments,
when a need shows up: a command whose output is a line of the browser's
summary, with a time limit.

```yaml
start:
  - weaver-git clone   # clones the repositories that are missing
```

**The git plugin**: a separate program, `weaver-git`, not part of
`weave`. It keeps its own list of repositories and remotes, outside the
project's config, and does the git work: clone what's missing, report
the branch, maybe add worktrees for #11. Nothing runs unless a config names it.

Open questions:

- What weaver hands a hook: the project's name, root and config path as
  environment variables (`WEAVER_PROJECT`, `WEAVER_ROOT`)?
  `weave show` already prints the config.
- Plugins as plain commands in hooks, or `weave git …` finding
  `weave-git` on `PATH`, as git finds `git-foo`. The second adds
  dispatch code to the core.
- Whether hooks go under one `hooks:` key or stay top-level keys, as
  `start` and `stop` are in #6.
- *hook* and *plugin* need entries in the glossary before they get code.

## Left out

- `layout` presets (`tiled`, `main-vertical`): splits already describe
  more.
- `synchronize-panes`, a raw tmux `options` field: rarely needed.
- `sleep` delays (tmuxp's `sleep_before`): `wait` (#7) does the same job
  more reliably.
- Templates and variables passed when opening (tmuxinator's ERB
  `key=value`, smug's `-v`): there is one config per project, and `env`
  covers the rest.
- A config committed inside the repository (`.tmuxp.yaml`,
  `.laio.yaml`): it breaks "one file per project in the config folder",
  and only pays off when a team shares it.
- Saving pane contents to bring back after a reboot (tmux-resurrect):
  weaver rebuilds from the config, which is the better source of truth.
- Opening some windows from the command line (smug's `proj:win1,win2`):
  restoring brings back closed windows, and lazy windows (#9) put the
  choice in the config.

## Suggested order

1. **Focus (#3)**: small, and noticed every time a session opens.
2. **`start` and `stop` (#6)**: one change, since both use the new
   `CommandRunner` port. `stop` fixes leaked containers.
3. **Window `env` and `dir` (#1, #2)**: small, whenever a config would
   use them.
4. The rest by need. `wait` (#7) can come after `start`: `start` covers
   setup that has to finish before the panes open, and `wait` orders
   the panes among themselves.

## Shared groundwork

- `CommandRunner` serves both halves of #6, and every hook in #12.
- The idle-shell check (`foreground`) serves #4 and the clean shutdown
  in #6.
- A second CLI argument serves #9 and #10: design both together.
- The `sh` startup script (`shellCommand`) carries #5 and maybe #7.
