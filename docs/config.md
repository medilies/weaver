# Project config

A project is one config file at `~/.config/weaver/projects/<name>.yml`
(`$XDG_CONFIG_HOME/weaver/projects` when that is set). The file name is
the project's name and its tmux session's name: letters, digits, `-` and
`_`, starting with a letter or a digit, and not one of weave's commands.

The smallest config opens one window with one shell in the root:

```yaml
root: ~/dev/weaver
```

## Toolchains

A back end and a front end on different toolchain versions. weaver
knows no toolchain: `before` types whatever commands set one up into the
pane's shell, as you would. An alias only
changes what you type: tools that run `php` themselves, such as
`composer`, need a `php` first on `PATH` instead:

```yaml
root: ~/dev/shop
windows:
  - name: shop
    panes:
      - name: back
        dir: back
        before: ["alias php=php8.4"]
      - name: front
        dir: front
        split: { direction: right, size: 30% }
        before: ["nvm use 20"]
        note:
          - "Usual commands:"
          - "  npm i"
          - "  npm run dev"
```

## Fields

| Field | Meaning |
| --- | --- |
| `root` | Directory the session is rooted at: absolute, or starting with `~/`. Required. The home directory itself is `"~"`, in quotes: YAML reads a bare `~` as null. |
| `env` | Variables for every pane, and for the `setup` and `teardown` commands. Values are literal. |
| `setup` | Commands run once in `root`, in order, before `weave <project>` starts the session, such as `docker compose up -d --wait`. Each runs in its own `sh -c`, with `env` but not `before`, and its output in your terminal. A failing one stops the open, and no session is started. Not run when the session is already running. |
| `teardown` | Commands run in `root`, in order, before `weave kill` ends a session weaver started, such as `docker compose down`, run like `setup`. The session is killed even when one fails; the commands after it don't run. Killed from inside that session, whose pane goes with it, weave shows the failure and waits for Enter first. |
| `before` | Commands typed and run in every pane, in order, before the pane's own command. They are typed as soon as the pane opens, so one that reads the terminal (`ssh`, a password prompt) takes the lines typed after it. |
| `clear` | `true` hides the `before` commands once they ran: `clear` is typed after them, a space first so shells ignoring such lines keep it out of their history, then the `note` printed. Only the screen is cleared: tmux's scrollback keeps them. Off unless set; a window's or a pane's `clear` replaces it. |
| `windows` | Windows in order, with unique `name`s. None: one window named after the project. |
| `windows[].dir` | Where the window's panes start, unless they have their own `dir`, written like a pane's. Default: `root`. |
| `windows[].focus` | `true` lands `weave <project>` in this window when it starts the session, instead of the first. At most one window. |
| `windows[].env` | Variables for every pane of the window, over the project's, per variable. |
| `windows[].before` | Commands run in every pane of the window, after the project's `before`. |
| `windows[].clear` | Whether the window's panes clear their `before` commands, over the project's `clear`. |
| `windows[].grid` | Lays the window's panes out by naming them, instead of `split`s; see [Grid](#grid). |
| `windows[].panes` | Panes in creation order (in any order with a `grid`). None: one shell in the window's `dir`. |
| `name` (pane) | Shown in the pane's top border. Required and unique when a window has several panes or a `grid`. |
| `focus` (pane) | `true` selects this pane in its window, instead of the first. At most one pane per window. Freezing a session focuses the window and panes that were active. |
| `dir` | Where the pane's shell starts, instead of its window's `dir`: relative to `root`, not to the window's `dir` (`.` for `root` itself), or absolute, or starting with `~/` (`"~"` in quotes for the home directory). |
| `split` | How every pane but the first is made, unless the window has a `grid`: `direction` (`right` or `below`), `size` (`40%`, or `20` cells; default half of the split pane), `of` (the name of the pane it splits; default the previous one). |
| `before` (pane) | Commands run in the pane, after its project's and window's `before`. |
| `clear` (pane) | Whether the pane clears its `before` commands, over its window's and project's `clear`. |
| `command` | Typed into the shell and run, so it stays in the shell history. |
| `prefill` | Typed at the prompt but not run: press Enter to run it. |
| `note` | Lines printed before the shell starts, or after the `before` commands when the pane clears them. |
| `env` (pane) | Overrides the window's and the project's, per variable. |
| `tasks` | Commands the pane offers to run from the browser, in a session weaver started: each a `command` and, optionally, a `name` to show instead. No two share a name, nor two unnamed ones a command. See [tasks in the browser](browser.md#tasks). |

## Grid

A `grid` lays a window's panes out by naming them in rows and columns,
sized as shares of the window rather than of the pane being split. This
window is the `app` window of the README's nimbus example:

```yaml
  - name: app
    dir: app
    grid:
      columns: [60%, 40%]
      rows: [70%, 30%]
      areas:
        - [app shell, flutter run]
        - [app shell, emulator]
    panes:
      - name: app shell
      - name: flutter run
        command: nimbus app
      - name: emulator
        dir: .
        prefill: nimbus emulator
```

| Field | Meaning |
| --- | --- |
| `areas` | Rows of pane names, one per column. A pane covers the rectangle of cells holding its name. Every pane is named, and none has a `split`. |
| `columns`, `rows` | One size per column or row: a percentage of the window (`60%`), a number of cells (`30`), or a weight sharing out what the others leave (`2fr`). Columns or rows in cells need one in `fr` to take the rest. Default: equal. |

A size in cells is exact when the window opens, such as a 30-cell file
tree beside an editor taking the rest:

```yaml
    grid:
      columns: [30, 1fr]
      areas:
        - [tree, editor]
```

Like every pane in tmux, it then grows or shrinks with the terminal.

tmux only cuts a window in straight lines, each piece cut again, so a
grid whose panes no straight line across separates is refused, such as
four panes turning around a fifth:

```text
a a b
d e b
d c c
```

`weave freeze` writes splits.

## Validation, comments and schema

Unknown fields are errors: they are usually typos. Comments are kept by
`weave edit --raw`, but not when `weave freeze` replaces a config or the
config form saves it: weave edit asks first when a config has some.

When weaver saves a config, it writes `weaver.schema.json` next to it
and points the config at it with a first line of
`# yaml-language-server: $schema=./weaver.schema.json`, so editors such
as VS Code (with its YAML extension) complete fields and flag mistakes
as you type.

Configs from v0.1.0 may still have `repositories:`, which weaver no
longer reads: delete it with `weave edit --raw <project>`. Saving also
refreshes `weaver.schema.json`, which until then still allows it.
