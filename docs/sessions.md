# Opening, killing and freezing

## Open

`weave <project>` attaches to the project's tmux session (inside tmux,
it switches to it, from a `display-popup` too). When the session isn't
running, weaver first:

1. runs the config's `setup` commands in the root, one after another,
   and stops if one fails or you press Ctrl-C, starting no session;
2. checks every pane's directory exists, and stops if one is missing,
   saying to create it or change it with `weave edit`: weaver creates
   none;
3. builds the session, sized like your terminal so percentage splits
   come out right, labels its panes in their top border, and types each
   pane's `before` commands, then `clear` and its note when it clears
   them, then its command, into its shell.

weaver doesn't know git and clones nothing: clone what a project needs
yourself before opening it.

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

## Kill

`weave kill <project>` runs the config's `teardown` commands in the root
before killing an owned session, and kills it even when one fails,
reporting the failure. Anything a pane started that left it, such as
containers, keeps running unless teardown stops it.

## Freeze

`weave freeze <session>` starts the [config form](config-form.md) from the session as it runs:
its windows and panes, laid out as they are, each in its directory,
written relative to the root even when you reached the root through a
symlink. The command in a pane's foreground is typed at its prompt but
not run, from the directory of the shell that started it. Check it
before you run it: pipes, quotes and variables don't survive. A pane
tmux started with a command, as `tmux new-window 'npm run dev'`, gets
that command typed at its prompt too, from the directory it started in.
A window laid out by a `grid` is frozen as splits.
