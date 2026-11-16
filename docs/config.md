# Project config

A project is a directory, its root, and its config, a YAML file. The
root is what the panes' directories are relative to. Keep the config
in the root, to version it with the code it opens and share it with a
team, or anywhere else.

## Registering

[`settings.yml`](settings.md) lists the projects under `projects:`, each
by its root, absolute or starting with `~/`:

```yaml
projects:
  - ~/dev/shop                            # its config is ~/dev/shop/weaver.yml
  - root: ~/work/client-api
    config: ~/.config/weaver/client-api.yml  # relative to the root, or absolute
    name: client                          # instead of the config's, or client-api
```

- `weave bind` adds the directory it runs in, first; `d` in the
  [browser](browser.md) takes a project off the list. Editing the list
  by hand does the same.
- The config is the one the entry names, else the first of
  `weaver.yml`, `weaver.yaml`, `.weaver.yml` and `.weaver.yaml` in the
  root. `weave bind` writes a new one as `weaver.yml`.
- One project per root: a root listed twice shows in the browser as a
  problem until one entry goes. Several roots may share a config.
- A project whose root or config is missing, or whose config isn't
  valid, stays in the browser, marked with a red ✗.
- A config may be a symbolic link, as dotfiles managers make: saving
  replaces the file it points to and keeps the link.

Registering a config is when you trust it: weave runs its commands
whenever you open the project, as written then, a teammate's later
commits included.

## Names

A project is named by its entry's `name:`, else by its config's
`name:`, else after its root's directory: `~/dev/weaver.nvim` is
`weaver.nvim`. A name may be anything but `.`, `..` or one holding `/`.

- Two projects may share a name: the browser shows their roots beside
  it, and `weave api` asks you to name one by a directory instead.
- A project named after a command, such as `kill`, opens with
  `weave open kill`.
- Its tmux session is `w/` and its name, or `w/` and its root when
  another session has that name already, `.` and `:` written `_`. See
  [sessions](sessions.md#sessions-and-their-projects).

The smallest config is an empty file: one window with one shell in the
root.

## Toolchains

Here a back end and a front end use different toolchain versions.
weaver knows no toolchain: `before` types the commands that set one up,
as you would. An alias only changes what you type; tools that run `php`
themselves, such as `composer`, need the right `php` first on `PATH`:

```yaml
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
| `name` | The project's name, unless its entry in `settings.yml` names it; see [Names](#names). |
| `env` | Variables for every pane, and for the `setup` and `teardown` commands. Values are literal. |
| `setup` | Commands run once in the root, in order, before `weave <project>` starts the session, such as `docker compose up -d --wait`. Each runs in its own `sh -c`, with `env` but not `before`, and its output in your terminal. A failing one stops the open, and no session is started. Not run when the session is already running. |
| `teardown` | Commands run in the root, in order, before `weave kill` ends a session weaver started, once its panes were interrupted, such as `docker compose down`, run like `setup`. The session is killed even when one fails; the commands after it don't run. |
| `before` | Commands typed and run in every pane, in order, before the pane's own command. They are typed as soon as the pane opens, so one that reads the terminal (`ssh`, a password prompt) takes the lines typed after it. |
| `clear` | `true` hides the `before` commands once they ran: `clear` is typed after them, then the `note` printed. They are also kept out of the shell history where it leaves out commands starting with a space: fish always does, bash with `HISTCONTROL=ignorespace` or `ignoreboth`, zsh with `setopt HIST_IGNORE_SPACE`. tmux's scrollback keeps them, unless the terminal type is tmux's own, such as its default `tmux-256color`. Off unless set; a window's or a pane's `clear` replaces it. |
| `windows` | Windows in order, with unique `name`s. None: one window named after the project. |
| `windows[].dir` | Where the window's panes start, unless they have their own `dir`, written like a pane's. Default: the root. |
| `windows[].focus` | `true` lands `weave <project>` in this window when it starts the session, instead of the first. At most one window. |
| `windows[].env` | Variables for every pane of the window, over the project's, per variable. |
| `windows[].before` | Commands run in every pane of the window, after the project's `before`. |
| `windows[].clear` | Whether the window's panes clear their `before` commands, over the project's `clear`. |
| `windows[].grid` | Lays the window's panes out by naming them, instead of `split`s; see [Grid](#grid). |
| `windows[].panes` | Panes in creation order (in any order with a `grid`). None: one shell in the window's `dir`. |
| `name` (pane) | Shown in the pane's top border. Required and unique when a window has several panes or a `grid`. |
| `focus` (pane) | `true` selects this pane in its window, instead of the first. At most one pane per window. |
| `dir` | Where the pane's shell starts, instead of its window's `dir`: relative to the root, not to the window's `dir` (`.` for the root itself), or absolute, or starting with `~/` (`"~"` in quotes for the home directory). |
| `split` | How every pane but the first is made, unless the window has a `grid`: `direction` (`right` or `below`), `size` (`40%`, or `20` cells; default half of the split pane), `of` (the name of the pane it splits; default the previous one). |
| `before` (pane) | Commands run in the pane, after its project's and window's `before`. |
| `clear` (pane) | Whether the pane clears its `before` commands, over its window's and project's `clear`. |
| `command` | Typed into the shell and run, so it stays in the shell history. |
| `prefill` | Typed at the prompt but not run: press Enter to run it. Not with `command`. |
| `note` | Lines printed before the shell starts, or after the `before` commands when the pane clears them. A line holding newlines, such as a `\|` block, prints as the lines it holds. |
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
| `areas` | Rows of pane names, one per column. A pane covers the rectangle of slots holding its name. Every pane is named, and none has a `split`. |
| `columns`, `rows` | One size per column or row: a percentage of the window (`60%`), a number of terminal cells (`30`), or a weight sharing out what the others leave (`2fr`). Columns or rows in cells need one in `fr` to take the rest. Default: equal. |

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

## Validation, comments and schema

Unknown fields are errors: they are usually typos. Comments are kept
only by editing a config raw (`E` in the browser): the config form
rewrites it, see [saving](config-form.md#saving).

When the config form saves a config, it points the config at its JSON
Schema, as published for your weave's version, with a first line of
`# yaml-language-server: $schema=https://…/weaver.schema.json`, so
editors such as VS Code (with its YAML extension) complete fields and
flag mistakes as you type. A config holding `root:` is refused: the
root is its entry's in `settings.yml`.
