# The browser

`weave` alone opens the browser: the projects, their tasks, and the
selected project's windows drawn as they open.

Inside tmux, it shows in the pane you typed it in. To have it fill the
window instead:

- set [`popup: true`](settings.md#popup): `weave` and `weave bind` then
  show in a tmux popup the size of the window;
- or bind a tmux key to it, in `~/.tmux.conf`:

  ```sh
  bind-key W display-popup -E -w 100% -h 100% weave
  ```

## Summary

The browser lists the projects, the selected one after a ›:

- first those whose session runs, after a ● and in green, the one last
  attached or started at the top; then the others in the order
  [`projects`](settings.md#projects) lists them. When the list reorders,
  the cursor stays on its project.
- a project sharing its name with another shows its root beside it.
- a project whose config is missing or can't be read shows after a red
  ✗, its details saying why.

A project added to or taken off the settings' list, by hand or by
another weave, joins or leaves the list as the browser reads again
([`preview-every`](settings.md#preview-every)); when the selected one
goes, the cursor moves to the next. The details' title is the project's
name and its config file. Beside the list, a summary of the selected
one:

- how many of the configured windows a running session still has, and
  which ones enter restores (a session weaver didn't start is left as it
  is);
- the root, or a warning when it is gone;
- what would likely fail in the panes enter creates: a directory
  missing; a command whose program isn't on your `PATH` (unchecked in
  panes with `before` commands or an `env` setting `PATH`, which may
  provide it; aliases and shell functions show as not found); or a
  command, `before` ones included, that leaves a quote open, so the
  shell would wait for the rest of it;
- in a session weaver started, each pane whose `command` isn't running,
  its shell back at its prompt: it crashed, was interrupted (Ctrl-C warns
  too) or finished, or the pane is still starting; checked again every
  second;
- the `before` commands and variables every pane starts with.

## Drawing

Below the summary, the window list names the project's windows, ○ marking those enter restores, and the
selected window is drawn as it opens: each pane a box in its
proportions, with its name when the window has several, its `dir`
unless it is the root, its `note`, and its prompt, showing a `command`
as run (`$ task dev`), a `prefill` as typed (`$ nimbus emulator█`) or
neither (`$ █`). A pane with a warning has ✗ in its top border and a red border when its
directory is missing, or ⚠ and a yellow border when its program isn't
found or its command exited. In a small terminal the panes are listed instead.
The selected pane is in the [selected colour](settings.md), its name in
brackets. Below the drawing, the
pane list names the window's panes as the drawing places them, top to
bottom then left to right, each after the ✗ or ⚠ of its warning, the
selected one underlined, and below it are the selected pane's directory, its `before` commands in the order they
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
colours, kept up to date. The details' title ends in `· preview`.
Its panes keep their config names, warnings and levels, and `r` works;
a pane split by hand shows its number, as "not in the config". The pane
the browser runs in reads "this browser". A zoomed window shows its
zoomed pane alone, as tmux draws it. The drawing comes back, the status
line saying why, when the session ends or the window closes, and for a
window the session lacks, which enter restores; these hints aren't
failures and stay plain.

The browser stays up to date while it is open, checking every second
unless the [settings](settings.md#preview-every) say otherwise:
the project list's ● marks and green names, and the selected project's
summary, drawing and task list, its config read again.

## Tasks

Beside the project list, the task list shows the selected project's
tasks under `window › pane`, in the order its panes are created, ● before
one still running in its pane or its throwaway window, kept up to
date. weaver can't tell a task from another job started there
after it ended, and never marks one in the pane the browser runs in,
which it keeps busy itself.

Running one needs the project's session running, started by weaver:

- When the pane's shell is idle at its prompt, the task's command is
  typed into it and run, after cutting whatever is at the prompt, such
  as a `prefill` (C-y pastes it back).
- When something runs there, or the pane is in copy mode, weaver asks
  before running the task in a throwaway window instead: named after the
  task (or its command's program), starting in the pane's directory with
  its `env` and `before` commands, and kept open once the task ends until
  you close it. `weave bind` leaves throwaway windows out, and opening
  the project doesn't restore them.
- `w` runs a task in a throwaway window without asking.
- With enter, weaver then puts you where the task runs, as opening the
  project does; with `r`, `w` or a click, you stay in the browser, and
  the session's current window and pane stay as they were.
- Enter on a ● task doesn't run it again: it puts you where it runs,
  its pane when it runs there, otherwise its most recent throwaway
  window. `r` and `w` still run another copy.
- `i` on a ● task interrupts it, typing ctrl+c where it runs: in its
  pane when it runs there, otherwise in its most recent throwaway
  window. Each `i` interrupts one copy; the status line says how many
  more run. The browser stays.
- A task of the pane the browser runs in is never typed there, which
  would type it into the browser: the status line says so, and `w` runs
  it in a throwaway window.

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

The keys below are the defaults: the [settings](settings.md#keys) can
move the browser's actions to other keys.

| Key | Does |
| --- | --- |
| ↑ ↓ or k j | select a project |
| ← → | select a window to draw |
| tab or shift+tab | select the next or previous pane of the selected window, in the pane list's order |
| r | rerun the selected pane's command once it exited: typed into the pane again, as a task is, then you are put in that pane. Only in a session weaver started, on a pane run with `command`, not `prefill` |
| i | interrupt what runs in the selected pane: ctrl+c typed into it, as if you pressed it there; the browser stays. Only in a session weaver started, never the pane the browser runs in. A pane idle at its prompt has nothing to interrupt |
| R | reload the selected pane or its whole window, as its config says now, ending what runs there; the status line asks which, `p` or `w`, any other key cancelling. A pane restarts in place; a window is rebuilt where it is, and one the session lacks is restored. The browser stays. Only in a session weaver started, never the pane the browser runs in or its window |
| p | switch between the preview of the selected window and its drawing, for as long as the browser stays open; it starts on the preview unless the [settings](settings.md#preview) say otherwise. Only in a session weaver started |
| y | show the selected project's config, as stored, instead of its drawing, and back; it stays shown as you select other projects, and esc hides it too |
| t | move the cursor into the task list: ↑ ↓ or k j select a task, its window and pane selected in the drawing; enter, r or w run it, enter going to a ● task instead, and i interrupts a ● task (see [Tasks](#tasks)); esc goes back |
| click | unless the [settings](settings.md#mouse) turn the mouse off: select the project clicked in the list, the window clicked in the window list, or the pane clicked in the pane list; run the task clicked in the task list, as r does; on a key shown at the bottom, or on ← or → beside it, do what the key does; on the kept filter, edit it again as / does |
| / | filter the list by name as you type, ignoring case; while typing, ↑ ↓ select a project, ← → a window and the Tab key a pane, enter keeps the filter for the keys below and esc clears it |
| enter | open it: start it, then attach or switch, like `weave <project>` |
| s | start it, like `weave -d <project>`: the browser stays, saying what was started |
| b | bind the current directory, like `weave bind` |
| e | change its config in the [config form](config-form.md) |
| E | edit its config raw in the [settings](settings.md#editor)' editor, else $VISUAL, else $EDITOR, else vi (with VS Code, `VISUAL="code --wait"`), keeping its comments; only a valid config is saved: on a mistake, weaver asks whether to discard your changes, and otherwise reopens the editor on what you wrote |
| x | kill its session, after you confirm |
| d | unregister it, after you confirm: its config, and a running session, stay |
| q or esc | quit (esc hides the config, then clears the filter, first) |

After starting, binding, editing, killing, deleting, running a task,
interrupting or reloading, the list
comes back with what happened in the status line, a failure in red after
✗, the filter kept while it shows the project acted on. The browser
stays on screen while the action runs, unless the action needs the
terminal: the config form, the editor, `setup` or `teardown` commands
that run, or going into a session.

- A question shows in the status line. `y` or enter answers yes, any
  other key no, as the keys line below it says.
- While the action runs, the status line shows a spinner and what it is
  doing, such as `Killing shop…`. Keys wait meanwhile. Pressing ctrl+c
  twice leaves the browser, and the action finishes on its own.

The browser and the config form take the mouse, so most terminals
select text with shift held, and the wheel doesn't scroll them.
