# Left out

What weaver leaves out, or ignores, and why: not worth the code, or out
of scope. Kept so they aren't reported again. What is still to do is in
[GitHub Issues](https://github.com/medilies/weaver/issues).

## Ignored until a user needs it

- **Any tmux but 3.4.** CI and releases run on ubuntu-24.04 for Ubuntu's
  tmux 3.4, not ubuntu-latest; re-pin when GitHub retires that runner.
  CI's macOS job builds 3.4 from its release, Homebrew having only the
  newest. Another version may differ in what weaver reads back: 3.4
  prints a `\` before some `$` (see `verbatim` in `infra/tmux.go`).
  Older than 3.2 fails for sure: restoration needs `new-window -b`. Hence
  no tmux version check when weave runs.

## Left

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
- **One tmux call for capture.** `Capture` could use the one pane
  listing (`projectPanes`) the warnings, tasks and the preview read, but
  still asks tmux four or five times per pane: not worth changing while
  bind is interactive, since splitting free-text paths and labels out of
  one line adds parsing risk.

## Left out of the config

As other session managers have them:

- `layout` presets (`tiled`, `main-vertical`): splits already describe
  more, and a grid writes them out explicitly.
- A flex layout (nested rows and columns of sized panes): it is
  `#{window_layout}`'s tree, saying what splits already say, more deeply
  nested. `wrap`, `gap`, `justify-content` and `align-items` mean
  nothing to tmux, whose panes always fill the window, so the name
  promises more than it can do. A grid is the other notation.
- `synchronize-panes`, a raw tmux `options` field: rarely needed.
- `sleep` delays (tmuxp's `sleep_before`): [`wait`](https://github.com/medilies/weaver/issues/19) would do the same job more
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
  restoring brings back closed windows, and [lazy windows](https://github.com/medilies/weaver/issues/21) would put the choice
  in the config.
