# Freeze is renamed bind, the one Silksong word in the core

Registering a project through the config form, optionally from a running
session, was called freeze until v0.6.0. It is now bind: `weave bind`,
key `b` in the browser. weaver is named after the Weavers of Hollow
Knight: Silksong, where binding is drawing silk together, and binding a
loose session into a config fits it better than freezing one.

This is a deliberate exception to the backlog's Silksong plugin, which
keeps the core in the glossary's plain words and leaves the theme to a
plugin. The other candidates (unbind for delete, weave for open, mend
for restoration, cut for kill, loom for the browser…) stay plugin candidates: they replace words that are already
standard, like tmux's kill, or that nobody would guess.

`weave freeze` is removed outright, with no deprecated alias: weaver is
before 1.0, and earlier renames (add to freeze) did the same. `bind`
becomes a reserved project name in place of `freeze`, so a project named
`bind` must be renamed.

To tmux users, bind means a key binding (`bind-key`). As a weave
subcommand, with help that says it binds a session or directory into a
config, it is accepted: the name is worth the ambiguity.
