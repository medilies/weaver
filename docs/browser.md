# The browser

`weave` alone opens the browser: the projects, their tasks, and the
selected project's windows drawn as they open.

## Summary

The browser lists the projects, a ● and a green name marking those whose session runs,
the selected one bold after a › in the [selected colour](settings.md),
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
neither (`$ █`). A pane with a warning has ✗ in its top border and a red border when its
directory is missing, or ⚠ and a yellow border when its program isn't
found or its command exited. In a small terminal the panes are listed instead.
The selected pane is in the [selected colour](settings.md), cyan unless
set, its name in brackets. Below the drawing, the
pane list names the window's panes as the drawing places them, top to
bottom then left to right, each after the ✗ or ⚠ of its warning, the
selected one underlined, in the selected colour while the cursor is in
the project list, and
below it are the selected pane's directory, its `before` commands in the order they
run, whether they are cleared, and its variables, each with the level it comes from (the project,
the window or the pane) and what it is over when it replaces the
window's `dir`, or a project or window variable.

With several windows and the lines to spare, each window is drawn small
under its name, a miniature, the selected one in the selected colour;
click one to select its window.

## Preview

When the selected project's session runs and weaver started it, the
selected window shows as it is now instead of as it opens: its panes
where tmux has them, each showing the bottom of its screen in its
colours, updated every second. The details' title ends in `· preview`.
Its panes are matched to the config's as tasks find them, so their
warnings, the levels below and `r` stay; a pane split by hand shows by
its number, as "not in the config". The pane the browser runs in reads
"this browser": captured, it would show the preview inside itself. A
zoomed window shows its zoomed pane alone, as tmux draws it. `p` switches to the drawing and
back, for as long as the browser stays open. The drawing comes back,
the status line saying why, when the session ends or the window closes,
and for a window the session lacks, which enter restores. Only the
preview and the task list's ● marks are live: the summary and the project
list's ● marks are as they were when the browser last showed the list.

## Tasks

Beside the project list, the task list shows the selected project's tasks under
`window › pane`, in the order its panes are created, ● before one still
running in its pane or its throwaway window (weaver can't tell it from
another job started there after it ended, and never in the pane the
browser runs in, which it keeps busy itself), checked again every second.
Running one needs the project's
session running, started by weaver: when the pane's shell is idle at its
prompt, the task's command is typed into it and run, after cutting
whatever is at the prompt, such as a `prefill` (C-y pastes it back);
when something
runs there, or the pane is in copy mode, weaver asks before running the task in a throwaway window
instead, named after the task (or its command's program), starting in
the pane's directory with its `env` and `before` commands, and kept open
once the task ends until you close it. `w` runs a task in a throwaway
window without asking, whether its pane is busy or not. `weave bind` leaves throwaway
windows out, and opening the project doesn't restore them. With enter,
weaver then puts you where the task runs, as opening the project does;
with `r`, `w` or a click, you stay in the browser, and the session's
current window and pane stay as they were. A task of the pane the browser
runs in is never typed there, which would type it into the browser: the
status line says so, and `w` runs it in a throwaway window.
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
| p | switch between the preview of the selected window and its drawing, for as long as the browser stays open. Only in a session weaver started |
| y | show the selected project's config, as stored, instead of its drawing, and back; it stays shown as you select other projects, and esc hides it too |
| t | move the cursor into the task list: ↑ ↓ or k j select a task, its window and pane selected in the drawing, enter runs it and puts you where it runs, r runs it and keeps you in the browser, saying where it runs, w runs it in a throwaway window, keeping you in the browser, esc goes back |
| click | select the project clicked in the list, the window clicked in the window list, or the pane clicked in the pane list; run the task clicked in the task list, as r does; on a key shown at the bottom, or on ← or → beside it, do what the key does; on the kept filter, edit it again as / does |
| / | filter the list by name as you type, ignoring case; while typing, ↑ ↓ select a project, ← → a window and the Tab key a pane, enter keeps the filter for the keys below and esc clears it |
| enter | open it: start it, then attach or switch, like `weave <project>` |
| s | start it, like `weave -d <project>`: the browser stays, saying what was started |
| b | bind the current directory, like `weave bind` |
| e | change its config in the [config form](config-form.md) |
| E | edit its config raw in $VISUAL, else $EDITOR, else vi (with VS Code, `VISUAL="code --wait"`), keeping its comments; only a valid config is saved: on a mistake, weaver asks whether to discard your changes, and otherwise reopens the editor on what you wrote |
| x | kill its session, after you confirm |
| d | delete its config, after you confirm; a running session keeps running |
| q or esc | quit (esc hides the config, then clears the filter, first) |

After starting, binding, editing, killing, deleting or running a task, the list
comes back with what happened in the status line, a failure in red after
✗, the filter kept while it shows the project acted on. The browser
stays on screen while the action runs, asking there what it asks
(`y` or enter answers yes, any other key no), unless the action needs
the terminal: the config form, the editor, `setup` or `teardown`
commands, or going into a session. Since the browser and the config form take
the mouse, most terminals select text with shift held, and the wheel
doesn't scroll them.
