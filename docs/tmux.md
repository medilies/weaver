# tmux for weaver

weaver puts each project in a tmux session. This is the tmux you need
to use one, if you have never used tmux before.

- A **session** is a project's set of windows. It keeps running when you
  leave it, and `weave <project>` takes you back.
- A **window** fills the terminal. Its names are in tmux's status line.
- A **pane** is one shell in a window, beside or under the others.

## A starting config

tmux reads `~/.tmux.conf` when its server starts. Create it with:

```tmux
set -g mouse on             # click panes and windows, drag borders, scroll
set -g base-index 1         # windows numbered from 1, like the keyboard
setw -g pane-base-index 1   # and panes
set -g renumber-windows on  # no gaps after closing a window
set -g escape-time 10       # Esc reaches programs at once
```

Apply it to a running tmux with `tmux source-file ~/.tmux.conf`.

## The keys you need

Every tmux key is the prefix, `Ctrl+b`, then a key. Press and release
`Ctrl+b`, then press the key.

| Keys | Does |
| --- | --- |
| `Ctrl+b` `d` | leave the session, which keeps running |
| `Ctrl+b` `w` | pick a session or window from a list |
| `Ctrl+b` `n` / `p` | the next or previous window |
| `Ctrl+b` `1` … `9` | the window with that number |
| `Ctrl+b` arrow | the pane in that direction |
| `Ctrl+b` `z` | zoom the pane to the whole window, and back |
| `Ctrl+b` `[` | scroll back with the arrows or Page Up; `q` stops |
| `Ctrl+b` `?` | every key |

With `mouse on`, clicking a pane or a window name, dragging a border and
scrolling work without the prefix.

Inside a session, run `weave` in any pane to browse your projects, and
`weave <project>` to switch to another project. See
[Opening, killing and binding](sessions.md).

## Theming

This makes tmux look like weaver's demo in the README:
[Catppuccin](https://catppuccin.com) Mocha, the status line on top. Add
it to `~/.tmux.conf`:

```tmux
set -as terminal-features ",*:RGB"
set -g base-index 1
setw -g pane-base-index 1
set -g status-position top
set -g status-justify left
set -g status-style "bg=#1e1e2e,fg=#cdd6f4"
set -g status-left ""
set -g status-right "#[bg=#313244,fg=#cba6f7,bold] #S "
set -g window-status-separator " "
set -g window-status-format "#[bg=#313244,fg=#cdd6f4] #W #[bg=#45475a] #I "
set -g window-status-current-format "#[bg=#cba6f7,fg=#11111b,bold] #W #[bg=#f5c2e7] #I "
set -g window-style "bg=#1e1e2e,fg=#cdd6f4"
set -g window-active-style "bg=#1e1e2e,fg=#cdd6f4"
set -g pane-border-style "fg=#6c7086"
set -g pane-active-border-style "fg=#cba6f7"
set -g message-style "bg=#1e1e2e,fg=#94e2d5"
```

- The status line shows the windows, the current one in mauve, and the
  session's name on the right.
- `window-style` and `window-active-style` paint the panes in Mocha's
  background. Leave them out to keep your terminal's.
- The first line lets tmux show these colours exactly. It needs a
  terminal with 24-bit colour, as most are. In one without, drop it.

For weave's screens to match, make mauve the
[selected colour](settings.md#theme), in
`~/.config/weaver/settings.yml`:

```yaml
theme:
  selected: "#cba6f7"
```

For more, such as rounded window names and other flavours, the
[Catppuccin tmux plugin](https://github.com/catppuccin/tmux) has them.
