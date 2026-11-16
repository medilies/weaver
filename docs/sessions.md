# Opening, killing and binding

## Naming a project

A command names a project by its name, or by a directory its root holds:

- `weave shop` opens the project named `shop`. A name two projects
  share is refused: the error names their roots.
- `weave ~/dev/b/api`, or anything holding a `/`, opens the project
  whose root holds that directory, the innermost when roots nest; `.`
  is the current directory.
- `weave open <project>` does the same as `weave <project>`, and reaches
  a project named after a command, such as `weave open kill`.

`weave kill` names one the same way.

## Sessions and their projects

A project's session is named `w/` and its name, as `w/shop`, or `w/`
and its root, as `w/home/you/dev/b/api`, when another session has that
name. `.` and `:` in it become `_`, as tmux writes them.

- A session weave started is its project's, by its root, even once
  renamed or the project renamed.
- A session you started is a project's when it has its session name
  and started in its root: `tmux new -s w/shop -c ~/dev/shop`.

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
(`--detached`) does all of the above but attach, so the
session starts, or its missing windows come back, and you
stay where you are, told what was done: `Started shop.`, `Restored
"logs" in shop.` or `shop is already running.` In the browser, `s`
does the same and keeps the browser open, the notice saying it.

## Kill

`weave kill <project>` ends a session. An owned one is ended in steps:

1. Ctrl-C is typed into each pane where something runs, throwaway
   windows included, but the one weave runs in. weave waits, up to 5
   seconds, for them to be back at their prompts, so a dev server can
   clean up after itself.
2. The config's `teardown` commands run in the root.
3. The session is killed, even when a pane still runs or a teardown
   command fails. weave names the panes that still ran, and reports the
   failure.

A session weave didn't start is killed at once. Anything a pane started
that left it, such as containers, keeps running unless teardown stops
it. Killed from inside that session, whose pane goes with it, weave
shows a failed teardown and waits for Enter first.

## Bind

`weave bind` registers the directory it runs in as a project's root,
through the [config form](config-form.md):

- When the directory has a config, `weaver.yml` or another
  [config name](config.md#registering), the form starts from it, and
  the diff before saving shows what changes. Otherwise it starts from a
  new project named after the directory.
- `--config` names another file, relative to the directory or
  absolute, such as `--config ~/.config/weaver/api.yml` to keep the
  config out of a team's repository; `--name` names the project.
- Saving writes the config and lists the directory first under
  `projects:` in `settings.yml`, unless it is listed already.

A teammate who cloned a repository holding `weaver.yml` registers it
with `weave bind` there, then `ctrl+s`.

`weave bind <session>` starts the form's windows from the session as it
runs: its windows and panes, laid out as they are, each in its directory,
written relative to the root even when you reached the root through a
symlink. The command in a pane's foreground is typed at its prompt but
not run, from the directory of the shell that started it. Check it
before you run it: pipes, quotes and variables don't survive. A pane
tmux started with a command, as `tmux new-window 'npm run dev'`, gets
that command typed at its prompt too, from the directory it started in.
A window laid out by a `grid` is captured as splits, and the window
and panes that were active get `focus`.

The config's `setup`, `teardown` and the rest are kept. Once saved, a
session started in the root is renamed after the project, so it is the
project's. Any other keeps its name: one started elsewhere, or one weave
started for another project.

## When tmux doesn't answer

weave gives up on a tmux command that doesn't answer within 5 seconds,
naming it. The [settings](settings.md#tmux-timeout) can change how long.
