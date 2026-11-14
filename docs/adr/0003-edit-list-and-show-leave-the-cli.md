# edit, list and show leave the CLI for the browser

`weave edit [--raw]`, `weave list` and `weave show` are removed in
v0.7.0, with no deprecated alias. The browser already does each of them:
`e` and `E` edit a config, even one that fails to load, the project list
shows what is registered and running, and the details show the config.
Keeping a second way in for each costs a command, its help and its
tests, for no use the browser lacks. weaver is before 1.0, and earlier
removals (`weave freeze`, ADR 0002) dropped commands the same way.

Their names stay reserved project names, so that bringing one back, if a
user asks for it, breaks no config. The backlog keeps them as a
candidate. Messages that pointed to them now point to the browser.
