# Settings

Settings are weaver-wide preferences, the same for every project, in
`~/.config/weaver/settings.yml` (`$XDG_CONFIG_HOME/weaver` when that is
set), beside the `projects` folder. They are never part of a project's
config. Without the file, every setting has its default.

```yaml
# yaml-language-server: $schema=./settings.schema.json
selected-colour: "#5fafd7"
keys:
  split-right: "|"
  split-below: "-"
```

weave saves `settings.schema.json` beside it once that folder exists, so
an editor with a YAML language server completes and checks the file.
weave refuses to start while the file is invalid, naming the file and
the field.

## `selected-colour`

The colour marking what is selected in the [browser](browser.md) and
the [config form](config-form.md): the selected pane in the drawing, the
selected window, pane or option in a list, and the chosen answer of a
question. An ANSI colour number, `0` to `255`, or `"#rrggbb"` in
quotes. Cyan (`6`) unless set. Warnings keep their own colours.

## `keys`

The config form's keys on the drawing, tmux's unless set. Write a key
as the terminal sends it: a character (`|`, `-`), or a name such as
`ctrl+x` or `alt+s`.

| Setting | Does | Default |
| --- | --- | --- |
| `split-right` | split the selected pane, the new one on the right | `%` |
| `split-below` | split the selected pane, the new one below | `"` |
| `remove` | remove the selected pane | `x` |
| `shrink` | make the selected pane smaller | `H` |
| `grow` | make the selected pane larger | `L` |

A key the config form already uses (the arrows, `h` `j` `k` `l`,
`enter`, `o`, `d`, `J`, `K`, `u`, `ctrl+r`, `ctrl+s`, `esc`, `ctrl+c`,
`tab`, `shift+tab`) is refused, as are two settings on the same key.
weave reads only this file, never your tmux bindings: see
[ADR 0001](adr/0001-drawing-keys-are-a-setting-not-read-from-tmux.md).
