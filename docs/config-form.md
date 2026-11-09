# The config form

`weave freeze` and `weave edit` (keys `f` and `e` in the
[browser](browser.md)) open the config form, which changes a project in
place.

It shows a project as cards: the project's, then the
window list and the selected window's card. Freezing starts it on a
name, the root, and the session's windows if you named one; the name is
typed there, and stays when editing. The selected window's panes are
drawn as they open, the selected pane in the
[selected colour](settings.md), heavy when the cursor is
on the drawing; below them are the pane list, then the selected pane's
card, starting with where the pane is placed. Click a window in the
window list or a pane in the pane list to select it, or a `starts`, `clear` or `focus` option
to pick it; the selected ones are blocks of the selected colour. Each field is
checked as you type it, and a root or dir is marked ✓ when it exists; a
command run that leaves a quote open is marked ⚠, as it would leave the
shell waiting, but kept. The
project's card lists its setup and teardown commands, edited like its
before commands. A window lists the project's variables dimmed, and a
pane the project's and its window's before commands and variables, with
the level they come from; enter on such a variable sets it for the
window or pane instead. A window or pane among several has a `focus`:
whether opening lands in it, or its window selects it. A pane's tasks come last: each
its command, then its name, left blank to show the task by its command.

A window laid out by a `grid` shows its `layout` in its card: pick
`splits` to turn the grid into the splits it makes, the panes then
split, removed and resized on the drawing like any other (a column or
row in cells becoming its share of the window as the drawing shows it,
so the drawing doesn't change); `u`
undoes it. Below are its `areas`, its rows
separated by ` / `, the panes' names in a row by commas
(`app shell, flutter run / app shell, emulator`), so that adding a column
changes every row at once, then
its `columns` and `rows` sizes, separated by spaces (`30 1fr`), blank for
equal ones. The drawing follows as you type. A name new to the window
adds a pane, and a pane no longer named is removed with what it runs:
rename a pane in its card instead, which renames it in the areas too.
Changing how many columns or rows there are makes them equal. When a
pane's name holds a comma or a `/`, the areas can't be typed there:
use `weave edit --raw`. Panes are
saved in the order the config lists them, new ones last.

Saving rewrites the config, so its comments go: `weave edit` asks first
when a config has some. To keep them, use `weave edit --raw` instead.

## Keys

| Key | Does |
| --- | --- |
| ↑ ↓ or k j | select a field |
| enter | type into it, or pick the next choice; on the window list or the pane list, type the window's or pane's name; on `+ add` or `+ window`, add one |
| ← → | on the window list or the pane list, select a window or a pane; on a pane's `starts`, whether it starts a shell, runs a command or prefills it, also while the command typed is empty; on `clear`, whether the `before` commands are cleared, or as the level above says; on `focus`, whether the window or pane is focused |
| o | add an item to the list selected or a task to the pane's tasks, split the pane selected right, or add a window |
| d | remove the item or task selected at once; on the pane list or a pane's other fields, remove the pane, and on a window's own fields or the window list, the window, after you confirm |
| J K | move the item, task or window selected down or up |
| u, ctrl+r | undo the last change, or redo the last change undone |
| ctrl+s | show what saving changes in the config (↑ ↓ scroll), then y, enter or ctrl+s saves and n or esc goes back; the config is rewritten, without its comments |
| esc | cancel typing, or leave, asking first when there are changes |

## Keys on the drawing

On the drawing, the keys are tmux's unless
[set otherwise](settings.md#keys). A window laid out by a `grid` only
selects its panes there: its areas place them.

| Key | Does |
| --- | --- |
| ← → ↑ ↓ or h l k j | select the pane next to the selected one; ↑ ↓ leave the drawing at its edge |
| % or " | split the selected pane right or below, in half |
| x | remove the selected pane, after you confirm; the pane it split takes its space |
| H L | make the selected pane smaller or larger, by 5%, or by 5 cells when its size is in cells |
| enter | go to the selected pane's card |
