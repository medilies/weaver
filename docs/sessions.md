# Opening, killing and binding

## Open

`weave <project>` attaches to the project's tmux session (inside tmux,
it switches to it, from a `display-popup` too). When the session isn't
running, weaver first:

1. runs the config's `setup` commands in the root, one after another,
   and stops if one fails or you press Ctrl-C, starting no session;
2. checks every pane's directory exists, and stops if one is missing,
   saying to create it or change it with `e` in the browser: weaver creates
   none;
3. builds the session, labels its panes in their top border, and types
   each pane's `before` commands, then `clear` and its note when it
   clears them, then its `command`, or its `prefill` without running
   it, into its shell.

If a step fails, nothing is left half-built. What the `setup` commands
did stays done, though: with no session, there is nothing to kill, so
no `teardown` runs, and the error says so.

## Restore

When weaver started the running session (an owned session) and you
closed some of its windows, `weave <project>` puts them back where the
config lists them before attaching, once it has checked their panes'
directories exist; a window that fails to build is removed again.
Windows you opened yourself stay as they are, and a session weaver
didn't start is only attached. Neither runs the `setup` commands.

## Start

Opening is starting, then attaching: `weave -d <project>`
(`--detached`; `.` for the current directory's project) does all of the
above but attach, so the
session starts, or its missing windows come back, and you
stay where you are, told what was done: `Started shop.`, `Restored
"logs" in shop.` or `shop is already running.` In the browser, `s`
does the same and keeps the browser open, the notice saying it.

## Kill

`weave kill <project>` runs the config's `teardown` commands in the root
before killing an owned session, and kills it even when one fails,
reporting the failure. Anything a pane started that left it, such as
containers, keeps running unless teardown stops it. Killed from inside
that session, whose pane goes with it, weave shows a failed teardown and
waits for Enter first.

## Bind

`weave bind <session>` starts the [config form](config-form.md) from
the session as it runs: its windows and panes, laid out as they are, each in its directory,
written relative to the root even when you reached the root through a
symlink. The command in a pane's foreground is typed at its prompt but
not run, from the directory of the shell that started it. Check it
before you run it: pipes, quotes and variables don't survive. A pane
tmux started with a command, as `tmux new-window 'npm run dev'`, gets
that command typed at its prompt too, from the directory it started in.
A window laid out by a `grid` is captured as splits, and the window
and panes that were active get `focus`.

The name is the session's, else the root's base name, and the root the
current directory; `--name` and `--root` set them. Binding under a
registered name keeps its `setup` and `teardown`, and the diff before
saving shows what it replaces.

## When tmux doesn't answer

weave gives up on a tmux command that doesn't answer within 5 seconds,
naming it. The [settings](settings.md#tmux-timeout) can change how long.
