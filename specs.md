# Weaver

> Historical: the first brief for weaver, kept as written. The README and the glossary in CLAUDE.md describe weaver as released.

Look how my tmux scripts work with globex and nimbus

I should be able to call `weaver globex` or `weaver nimbus` from anywhere and start or attach to their tmux session like the current behavior

I should be to config json per project. The JSON will contain:

- Project location
- optional repository
- tmux Windows
- Panes with sizes
- Default text per pane
- default commands per pane
- env changes like default php or versions

there also be an interractive command to add all these per project. use current pwd if I do not provide project location

Think outside the box and propose ideas and implement them after I approve

---

## Post M5 feedback

- weave command
- weave opens tui to show all prjects list with pane for details and other control options
- one json per project in a configs folder
- support yml
