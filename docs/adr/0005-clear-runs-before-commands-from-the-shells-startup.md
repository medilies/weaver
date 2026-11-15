# clear runs before commands from the shell's startup

A pane that clears runs its before commands from its shell's own startup, after the user's startup files, rather than typing them into it. They never show as typed lines and never reach the shell's history. What they print is shown while they run, logged through tmux's `pipe-pane`, then cleared. The pane's command is then typed by the startup itself, through `tmux send-keys`, so it is in the history like a command the user typed.

Each shell is hooked its own way. bash gets an `--rcfile`, which reads what a login shell reads first. zsh gets a `ZDOTDIR` of weave's whose files read the user's. fish gets `--init-command`, which runs once its config ran. A POSIX sh gets `$ENV`, which reads the user's first. A temporary file, or folder for zsh, holds the hook, and the hook removes it.

Two other ways were weighed:

- Running the before commands in the start script, the `sh` that already exports a pane's env before it starts the user's shell. Only exported variables would survive into that shell. A virtualenv's prompt and `deactivate`, nvm's functions and aliases would be lost. The commands would also run in `sh`, not in the user's shell, so a fish user's syntax, or `nvm`, which is a shell function, would break.
- Typing them as before, with a leading space so that shells told to ignore such lines keep them out of the history. That depends on the user's shell settings, and they still show on screen as they are typed.

The cost is one hook per shell, and their limits:

- bash started with `--rcfile` isn't a login shell, though it reads a login shell's files. Debian's bash reads `/etc/bash.bashrc` on its own, and `/etc/profile` reads it again.
- A system file that sets `ZDOTDIR` before zsh reads weave's, or a `~/.profile` that sets `ENV` for a POSIX sh, takes the hook's place, and the before commands don't run.
- Ctrl-C in a before command would end the startup there. bash, zsh and sh trap it so the rest of the hook still runs, and the pane says a before command failed or was interrupted.

Another shell, or a tmux `default-command`, which weave can't see into, falls back to typing, the way `clear` worked before this.
