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

The browser (`weave`) and the config form (`weave bind`) read the
settings, and refuse to start while the file is invalid, naming the file
and the field; the other commands, the help, the version and completion
run either way. Each saves `settings.schema.json` beside the file once
that folder exists, so an editor with a YAML language server completes
and checks it; failing to save it only warns.

## `selected-colour`

The colour marking what is selected, and the cursor, in the
[browser](browser.md) and the [config form](config-form.md): the
selected pane, window, option or answer, the › mark, the border of the
card holding the cursor, and the keys at the bottom. An ANSI colour
number, `0` to `255`, or `"#rrggbb"` in quotes. Cyan (`6`) unless set.
Warnings keep their own colours.

## `preview`

Whether the [browser](browser.md) shows the preview of a running
session's window, rather than its drawing. `p` still switches. `true`
unless set.

## `preview-every`

How often the browser reads again what it shows: the preview, which
projects run, the selected project's summary and its task list's ●
marks. A duration in quotes from `"100ms"` to `"1m"`, such as `"500ms"`
or `"2s"`. `"1s"` unless set.

## `miniatures`

Whether the browser's window list draws each window small under its
name, when the project has several windows and there are lines to spare.
`true` unless set.

## `mouse`

Whether the browser, the config form and the questions weave asks take
clicks. With `false`, your terminal selects text with the mouse again.
`true` unless set.

## `editor`

The editor `E` in the browser opens a config in, written as `$VISUAL`
is, such as `code --wait`: the config's path is added after it.
`$VISUAL`, else `$EDITOR`, else `vi` unless set.

## `keys`

The config form's keys on the drawing, tmux's unless set. Write a key
as the terminal sends it: a character (`|`, `-`), or a name such as
`space`, `f5`, `ctrl+x` or `alt+s`. A key written otherwise (`" "`,
`ctrl-x`) is refused.

| Setting | Does | Default |
| --- | --- | --- |
| `split-right` | split the selected pane, the new one on the right | `%` |
| `split-below` | split the selected pane, the new one below | `"` |
| `remove` | remove the selected pane | `x` |
| `shrink` | make the selected pane smaller | `H` |
| `grow` | make the selected pane larger | `L` |

A key the config form already uses (the arrows, `h` `j` `k` `l`,
`enter`, `o`, `d`, `J`, `K`, `u`, `ctrl+r`, `ctrl+s`, `esc`, `ctrl+c`)
is refused, as are two settings on the same key.
weave reads only this file, never your tmux bindings: see
[ADR 0001](adr/0001-drawing-keys-are-a-setting-not-read-from-tmux.md).
