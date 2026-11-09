# The browser

`weave` alone opens the browser: the projects, their tasks, and the
selected project's windows drawn as they open.

## Summary

The browser lists the projects and their roots, a ● marking those whose session runs,
beside a summary of the selected one: how many of the
configured windows a running session still has and which ones enter
restores (a session weaver didn't start is left as it is); the root, or a
warning when it is gone; what would likely fail in the panes enter
creates (a directory missing, a command whose program isn't on your `PATH`,
unchecked in panes with `before` commands or an `env` setting `PATH`,
which may provide it, aliases and shell functions showing as not found,
or a command run, `before` ones included, that leaves a quote open, so
the shell would wait for the rest of it instead of running it); in a
session weaver started, each pane whose `command` isn't running, its
shell back at its prompt: it crashed, was interrupted (Ctrl-C warns too)
or finished, or the pane is still starting, its `before` commands
included; and
the `before` commands and variables every pane starts with.

## Drawing

Below the summary, the window list names the project's windows, ○ marking those enter restores, and the
selected window is drawn as it opens: each pane a box in its
proportions, with its name when the window has several, its `dir`
unless it is the root, its `note`, and its prompt, showing a `command`
as run (`$ task dev`), a `prefill` as typed (`$ nimbus emulator█`) or
neither (`$ █`). A pane with a warning has ⚠ in its top border, and
the border is red when its directory is missing, yellow when its
program isn't found or its command exited. In a small terminal the panes are listed instead.
The selected pane is cyan, its name in brackets. Below the drawing, the
pane list names the window's panes as the drawing places them, top to
bottom then left to right, the selected one in a cyan block, and
below it are the selected pane's directory, its `before` commands in the order they
run, whether they are cleared, and its variables, each with the level it comes from (the project,
the window or the pane) and what it is over when it replaces the
window's `dir`, or a project or window variable.

## Tasks

Beside the project list, the task list shows the selected project's tasks under
`window › pane`, in the order its panes are created, ● before one still
running in its pane or its throwaway window (weaver can't tell it from
another job started there after it ended). Running one needs the project's
session running, started by weaver: when the pane's shell is idle at its
prompt, the task's command is typed into it and run, after cutting
whatever is at the prompt, such as a `prefill` (C-y pastes it back);
when something
runs there, or the pane is in copy mode, weaver asks before running the task in a throwaway window
instead, named after the task (or its command's program), starting in
the pane's directory with its `env` and `before` commands, and kept open
once the task ends until you close it. `weave freeze` leaves throwaway
windows out, and opening the project doesn't restore them. Either way
weaver then puts you where the task runs, as opening the project does.
A pane offers tasks under `tasks:`:

```yaml
      - name: api
        dir: back
        command: php artisan serve
        tasks:
          - {name: migrate, command: php artisan migrate}
          - {command: 'php artisan db:seed'}
```

The warnings cover tasks too: a task whose program isn't on your `PATH`,
or that leaves a quote open, marks its pane.

## Keys

| Key | Does |
| --- | --- |
| ↑ ↓ or k j | select a project |
| ← → | select a window to draw |
| tab or shift+tab | select the next or previous pane of the selected window, in the pane list's order |
| r | rerun the selected pane's command once it exited: typed into the pane again, as a task is, then you are put in that pane. Only in a session weaver started, on a pane run with `command`, not `prefill` |
| y | show the selected project's config, as stored, instead of its drawing, and back; it stays shown as you select other projects, and esc hides it too |
| t | move the cursor into the task list: ↑ ↓ or k j select a task, its window and pane selected in the drawing, enter runs it, esc goes back |
| click | select the project clicked in the list, the window clicked in the window list, or the pane clicked in the pane list; run the task clicked in the task list; on a key shown at the bottom, or on ← or → beside it, do what the key does; on the kept filter, edit it again as / does |
| / | filter the list by name as you type, ignoring case; while typing, ↑ ↓ select a project, ← → a window and the Tab key a pane, enter keeps the filter for the keys below and esc clears it |
| enter | open it, like `weave <project>` |
| f | freeze the current directory, like `weave freeze` |
| e | change its config in the [config form](config-form.md), like `weave edit <project>` |
| E | edit its config in your editor, like `weave edit --raw <project>` |
| x | kill its session, after you confirm |
| d | delete its config, after you confirm; a running session keeps running |
| q or esc | quit (esc hides the config, then clears the filter, first) |

After freezing, editing, killing, deleting or running a task, the list
comes back, unfiltered, with what happened. Since the browser and the config form take
the mouse, most terminals select text with shift held, and the wheel
doesn't scroll them.
