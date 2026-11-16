# Projects are roots listed in the settings

A project used to be a `<name>.yml` in the config directory, holding its
`root:`. Now it is a root listed under the settings' `projects:`, the one
source of truth for what is registered. Its config is `weaver.yml` in
that root, or any file its entry names, anywhere on disk. A config can
then be versioned with the code it opens, and shared with a team, which
[left-out.md](../left-out.md) had turned away. The root moves from the
config to the settings, so a shared config holds nothing tied to one
machine.

## Considered options

- **Symlinks** into the config directory: they already worked, but
  nothing creates them, and the list of projects stays scattered links.
- **Pointer files** in the config directory naming a repository's
  config: two files per project.
- **Scanning directories** (`~/dev/*/weaver.yml`): registers configs
  nobody read, which then run commands.
- **Finding `weaver.yml` from the current directory**: what is
  registered would depend on where weave runs, and the browser could not
  list it.

## Consequences

- A project is identified by its root, not its name: two projects may
  share a name, and several roots a config. Names differing only in case
  are no longer one project.
- Sessions are named `w/<name>`, so a session counts for a project only
  by that name and its root, or, when weaver started it, by the root it
  records. Sessions started before the change are not recognised.
- Project names are no longer reserved (ADR 0003 kept them so):
  `weave open <project>` reaches a project named after a command.
- The config directory is no longer read; weave warns while it still
  holds configs, and a config holding `root:` is refused.
- Registering a config is the only time it is trusted: commands a
  teammate commits later run without asking.
- The browser lists running projects first, by when tmux last attached
  them, then the rest in the settings' order, instead of a record of
  opens (#24).
