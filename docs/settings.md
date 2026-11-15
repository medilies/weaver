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
run either way, with every setting at its default. Each saves `settings.schema.json` beside the file once
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

## `tmux-timeout`

How long weave waits on a tmux command before giving up, so a tmux
server that stops answering can't freeze it. A duration in quotes from
`"500ms"` to `"1m"`, such as `"10s"`. `"5s"` unless set.

## `popup`

Whether the browser (`weave`) and the config form (`weave bind`),
started in a tmux pane, show in a tmux popup the size of the window
rather than in that pane. The popup closes when they end, or stays open
to show why they failed. `false` unless set.

## `keys`

The keys of the browser's and the config form's actions, one key each.
Write a key as the terminal sends it: a character (`|`, `-`), or a name
such as `space`, `f5`, `ctrl+x` or `alt+s`. A key written otherwise
(`" "`, `ctrl-x`) is refused. Once moved, an action's own key does
nothing, and the keys line shows the new one.

Moving, `enter`, `esc`, Tab and `ctrl+c` can't be changed, nor, in the
config form, `h`, `l`, `o`, `d`, `y` and `n`. Two actions
available at once can't share a key: in the browser, those of the
project list, or those of the task list.

### The config form's drawing

tmux's keys unless set.

| Setting | Does | Default |
| --- | --- | --- |
| `split-right` | split the selected pane, the new one on the right | `%` |
| `split-below` | split the selected pane, the new one below | `"` |
| `remove` | remove the selected pane | `x` |
| `shrink` | make the selected pane smaller | `H` |
| `grow` | make the selected pane larger | `L` |

A key the config form already uses (the arrows, `h` `j` `k` `l`,
`enter`, `o`, `d`, `y`, `n`, `esc`, `ctrl+c`, or one of the keys below)
is refused, as are two drawing keys on the same key.
weave reads only this file, never your tmux bindings: see
[ADR 0001](adr/0001-drawing-keys-are-a-setting-not-read-from-tmux.md).

### The config form

| Setting | Does | Default |
| --- | --- | --- |
| `undo` | undo the last change | `u` |
| `redo` | redo what was undone | `ctrl+r` |
| `save` | save, after showing the diff | `ctrl+s` |
| `move-up` | move the item, task or window the cursor is on up | `K` |
| `move-down` | move it down | `J` |

### The browser

| Setting | Does | Default |
| --- | --- | --- |
| `start` | start the selected project | `s` |
| `bind` | bind the current directory | `b` |
| `edit` | change the config in the config form | `e` |
| `edit-raw` | edit the config raw in the editor | `E` |
| `kill` | kill the session | `x` |
| `delete` | delete the config | `d` |
| `rerun` | rerun the selected pane's exited command | `r` |
| `reload` | reload the selected pane or its window | `R` |
| `interrupt` | interrupt the selected pane, or a task in the task list | `i` |
| `preview` | switch between the preview and the drawing | `p` |
| `config` | show the config instead of the drawing | `y` |
| `tasks` | move the cursor into the task list | `t` |
| `filter` | filter the projects by name | `/` |
| `run` | in the task list, run the selected task, staying | `r` |
| `throwaway` | in the task list, run it in a throwaway window | `w` |
| `quit` | quit | `q` |
