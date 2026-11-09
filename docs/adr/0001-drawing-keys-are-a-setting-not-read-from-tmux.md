# The drawing's keys are a setting, not read from tmux

The config form splits, removes and resizes panes on its drawing with
tmux's default keys (`%`, `"`, `x`, `H`, `L`). Users who rebound them in
tmux want their own, so the keys become settings in `settings.yml`, and
weave never reads the user's tmux bindings. Finding them would mean
parsing `list-keys` output: telling `split-window -h` from `-v` among
arbitrary commands, key tables and prefixes, and binding the form's keys
to whatever a user's `tmux.conf` happens to contain. That is guesswork,
like telling shells apart when freezing, and weaver does only what the
user wrote down.
