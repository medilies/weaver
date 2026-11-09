# Testing in a real terminal

How to check weave end to end before or after a release: the browser, the config form and the confirmation dialog aren't unit-tested, and neither is how they behave together in a real terminal. Run every scenario below in a visible kitty window attached to a throwaway tmux server, driving it with `tmux send-keys` and reading it with `tmux capture-pane`.

## Never touch the user's tmux

The agent's shell usually runs inside the user's tmux, with `$TMUX` set, and `tmux` then talks to the user's server whatever `TMUX_TMPDIR` says. So:

- Start every command with `unset TMUX`.
- Always pass the private socket: define `T() { tmux -S /tmp/wvt/tmux/tmux-$(id -u)/default "$@"; }` and use `T`, never a bare `tmux`.
- Before relying on an empty variable, such as a pane ID read with `awk`, check it: `send-keys -t ""` goes to whatever tmux picks.

## Setup

1. **The binary under test.** For a release, test what users get:

   ```sh
   mkdir -p /tmp/wvt/bin
   GOBIN=/tmp/wvt/bin go install github.com/medilies/weaver/cmd/weave@vX.Y.Z
   cd /tmp/wvt && gh release download vX.Y.Z -R medilies/weaver -p '*inux*amd64*' -p checksums.txt
   sha256sum -c --ignore-missing checksums.txt
   mkdir asset && tar -xzf *inux*amd64*.tar.gz -C asset
   /tmp/wvt/bin/weave --version; /tmp/wvt/asset/weave --version   # both vX.Y.Z
   ```

   Before a release, `go build -o /tmp/wvt/asset/weave ./cmd/weave` instead.

2. **A config and a socket directory of its own.** tmux refuses a socket directory it didn't create unless it is the user's and mode 700:

   ```sh
   mkdir -p /tmp/wvt/tmux/tmux-$(id -u) /tmp/wvt/cfg/weaver/projects /tmp/wvt/shop/back /tmp/wvt/blog /tmp/wvt/notes
   chmod 700 /tmp/wvt/tmux/tmux-$(id -u)
   ```

   Write the three projects under [Sample projects](#sample-projects) into `/tmp/wvt/cfg/weaver/projects/`.

3. **A visible terminal.** A terminal emulator window, kitty here, attached to a session `term` of the throwaway server, so that attaching and switching clients run for real and the user can watch. Find the emulator with `command -v`; don't write its path here:

   ```sh
   cd /tmp && setsid kitty --title weaver-test \
     -o initial_window_width=160c -o initial_window_height=45c \
     env -u TMUX TMUX_TMPDIR=/tmp/wvt/tmux XDG_CONFIG_HOME=/tmp/wvt/cfg \
     PATH=/tmp/wvt/asset:/usr/bin:/bin tmux -f /dev/null new-session -s term bash --norc \
     >/dev/null 2>&1 &
   T list-clients -F '#{client_session} #{client_width}x#{client_height}'   # one client, on term
   ```

   Without a display, leave out the emulator and run `T -f /dev/null new-session -d -s term -x 160 -y 45 …` instead: everything below works except watching it.

4. **Clean up** once done: `T kill-server; rm -rf /tmp/wvt`. The terminal window closes with the server.

## Driving it

- Type a command into the shell with `T send-keys -t term 'clear; weave' Enter`. Send keys to a pane by its target (`=shop:shell`), or by the ID of a labelled pane: ``T list-panes -t '=shop:api' -F '#{pane_id} #{@label}'``.
- Wait after every key that changes the screen (`sleep 0.3` to `0.5`, `1.5` after starting weave or a session), then read it with `T capture-pane -p -t term`. Keys sent while weave is still busy are lost or reach a different mode. In a form, a word typed while not typing is read as keys: `o` adds a window, then the rest becomes its name.
- Check where the cursor is before acting on it. The browser lists projects by name, so a project added during the run shifts the others: count the lines, or read the selected one from the details' first line.
- Check what tmux holds, not only the screen: `T list-clients -F '#{client_session}'` for where the user is, `T display -p -t '=shop:' '#{window_name} #{@label}'` for the current window and pane, and the options weaver sets (`@weaver_task`, `@weaver_task_of`, `@weaver_throwaway`).
- Before each scenario that starts from the shell, `T switch-client -t term`.

## Scenarios

Each with what must hold. A failure is a bug in weave only once the keys sent are known to be the right ones.

| # | Do | Expect |
| --- | --- | --- |
| 1 | `weave` in `term` | The project list, task list, drawing and key line drawn, nothing cut or misaligned. |
| 2 | Select `blog`, Enter | The client switches to `blog`. The grid is about 60/40 and 70/30, and `preview`, focused, is the active pane. |
| 3 | `weave shop` in `term` | The setup command's output shows. The client lands in window `api`, pane `back`, both focused. `echo "$APP_ENV $LOG"` prints `staging info` in `logs` and `dev info` in `back`. |
| 4 | `weave`, select `shop`, → to `api` | `⚠` on `logs`, whose `sleep 3` exited. The levels show `APP_ENV=staging` as `window api, over project's local`. `r rerun` is in the key line on `logs`, not on `back`. |
| 5 | `t`, Enter on `wait` | The command is typed into `logs` and the client moves there. A new `weave` shows `● wait`. |
| 6 | `t`, `echo hi`, Enter, `y` | The dialog says `logs is busy`. A throwaway window `echo` opens with `@weaver_throwaway` 1 and `@weaver_task_of` set to the ID of `logs`, showing `hi`. |
| 7 | Ctrl-C in `logs`, then `weave`, select `shop`, → | `wait` has no ● any more. |
| 8 | `y`, then `y` again | The YAML shows as stored, then the drawing comes back. |
| 9 | `r` on `logs` | `sleep 3` is typed into `logs` again and the client moves there. |
| 10 | `weave edit shop`, the `shell` window's `focus` row, Enter, Ctrl-S, `y` | The diff removes `focus: true` from `api` and keeps the pane's. `Saved shop.` The file has a single `focus: true`, on `back`. |
| 11 | `weave freeze blog`, Ctrl-S, `y`, then `n`, Ctrl-U, type a new name, Enter, Ctrl-S, `y` | Saving under `blog` asks `blog is already registered. Replace it?`; `n` opens the form again typing the name. The saved config has `focus: true` on `preview`, and no window but `write`. |
| 12 | In `shop`'s `shell` window, `weave`, select `shop`, `x`, `y` | `tearing down`, then `teardown: false: command failed`, `shop's session will be killed anyway, with this pane.` and `Press Enter to go on.` One Enter kills `shop`. |
| 13 | Killing `shop` ended the terminal's client: attach it to `term` again (`tmux attach -t term` in the terminal command of [Setup](#setup)). `weave notes` in `term` | The client switches to `notes`, window `code`. `files` is exactly 30 columns wide and `edit` takes the rest (`#{pane_width}`), the window being the client's size less the status line. |
| 14 | `T switch-client -t term`, `weave`, select `notes` | Under the window list, `code` and `shell` are drawn small, each under its name, `code`'s with its narrow `files` column. In a terminal too short for them, only the names show. |
| 15 | Esc. Write `settings.yml` beside `projects` with `selected-colour: 214` and `keys: { split-right: "\|", split-below: "-" }`. `weave edit notes`, go to `shell`'s drawing, `\|` | `settings.schema.json` is beside it. The key line reads `\| split right · - split below`, `\|` splits the pane right, and `T capture-pane -e` shows the selected pane in colour 214 (`38;5;214`). `u` then Esc leave with `Nothing saved.` |
| 16 | Set `keys: { remove: u }` in `settings.yml`, `weave list` | `Error: …/settings.yml: keys.remove: key taken: "u" already does undo`, and nothing else runs. Remove `settings.yml` afterwards. |
| 17 | `weave edit notes`, `code`'s `areas` row, Enter, Ctrl-U, type `files, edit / files, logs`, Enter, Ctrl-S, `y` | The drawing adds `logs` under `edit` as it is typed. The diff adds the row and a pane `logs`, last. The saved config keeps `columns: [30, 1fr]` and lists `files`, `edit`, `logs`. |
| 18 | `weave edit notes`, `code`'s `layout` row, →, Ctrl-S, `y` | The `areas`, `columns` and `rows` rows go, and the drawing is unchanged: `files` keeps its width, `edit`'s split being its share of the terminal (about 86% at 213 columns), not of tmux's default 80. The diff removes the grid and adds a `split` to `edit` and `logs`. |

## Sample projects

`shop.yml`: window `env`, `focus`, setup and teardown commands (one of them failing), tasks, and a command that exits.

```yaml
root: /tmp/wvt/shop
env: { APP_ENV: local, LOG: info }
setup: [echo setting up shop]
teardown: [echo tearing down, "false"]
windows:
  - name: shell
  - name: api
    focus: true
    env: { APP_ENV: staging }
    panes:
      - name: logs
        command: sleep 3
        tasks: [{name: wait, command: sleep 300}, {command: echo hi}]
      - name: back
        focus: true
        dir: back
        env: { APP_ENV: dev }
        split: { direction: right, size: 60% }
```

`blog.yml`: a grid window whose focused pane isn't the first.

```yaml
root: /tmp/wvt/blog
windows:
  - name: write
    grid:
      columns: [60%, 40%]
      rows: [70%, 30%]
      areas:
        - [editor, preview]
        - [editor, shell]
    panes:
      - name: editor
      - name: preview
        focus: true
      - name: shell
```

`notes.yml`: a grid with a column in cells, beside a second window, to
check cell sizes, miniatures, settings, and editing a grid in the config
form.

```yaml
root: /tmp/wvt/notes
windows:
  - name: code
    grid:
      columns: [30, 1fr]
      areas:
        - [files, edit]
    panes:
      - name: files
      - name: edit
  - name: shell
```

## Adding scenarios

The list grows. Add a row whenever a case it misses turns up: a new feature or a changed behaviour, a bug found by hand, by a review or by a user, or anything a run shows that no row checks. Add what its sample needs to the projects above, and keep the rows in the order they run, each starting where the one before leaves off.

This file is public, in the repository. Keep everything in it generic:

- Paths only under `/tmp/wvt`, or as `~`, `$(id -u)` and tools found on `PATH`. Never a real home directory, user name, user ID, host name or install path.
- Sample projects and commands made up for the test (`shop`, `blog`, `sleep`, `echo`), never the user's own projects, configs, sessions or their names.
- Expected output as weave prints it for the samples, never copied from the user's terminal with their prompt, paths or other sessions in it.
