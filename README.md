# weaver

Weaver drives tmux sessions, windows and panes from a per-project YAML
config, so starting a dev environment is one command, `weave`,
instead of a bespoke script per project.

```yaml
# ~/.config/weaver/projects/nimbus.yml
root: ~/dev/nimbus
windows:
  - name: api
    dir: api
    panes:
      - name: api shell
      - name: task dev
        split: { direction: right, size: 40% }
        command: task dev
        tasks:
          - {name: migrate, command: task migrate}
      - name: psql
        dir: .
        split: { direction: below }
        command: nimbus psql
  - name: app
    dir: app
    panes:
      - name: app shell
      - name: flutter run
        split: { direction: right, size: 40% }
        command: nimbus app
      - name: emulator
        dir: .
        split: { direction: below, size: 30% }
        prefill: nimbus emulator
  - name: repo
```

`weave nimbus` opens it from anywhere. `weave` alone browses your
projects, drawing each window as it will open and warning about what
would likely fail:

```text
nimbus ~/dev/nimbus | tasks          | nimbus.yml
weaver ~/dev/weaver | api › task dev |
                    |   migrate      | ~/dev/nimbus
                    |                |
                    |                |  api    app    repo                         2/3
                    |                | +-[ app shell ]----------+- flutter run ------+
                    |                | | app/                   | app/               |
                    |                | | $                      | $ nimbus app       |
                    |                | |                        |                    |
                    |                | |                        |                    |
                    |                | |                        +- emulator ---------+
                    |                | |                        | $ nimbus emulator  |
                    |                | |                        |                    |
                    |                | +------------------------+--------------------+
                    |                |  app shell   flutter run   emulator
                    |                | app shell  ~/dev/nimbus/app  window app

enter open  s start  b bind  e edit  E raw  x kill  d delete
tab pane  t tasks  y config  / filter  q quit
```

## Install

Download a binary for Linux or macOS from the
[releases](https://github.com/medilies/weaver/releases), or build it with
Go 1.26 or newer:

```sh
go install github.com/medilies/weaver/cmd/weave@latest   # puts weave in ~/go/bin
```

weaver needs tmux, and is tested only with tmux 3.4. Other versions
may work; if one doesn't, please open an issue.

To complete project names with Tab, load weave's completion in your
`~/.bashrc` (zsh, fish and PowerShell work too, see
`weave completion --help`):

```sh
source <(weave completion bash)
```

## Usage

```sh
weave bind            # register the project in the current directory, through the config form
weave bind scratch    # same, starting from the windows, panes and commands of the running session scratch
weave bind .          # same, from the session you're in
weave nimbus          # open the nimbus session, from anywhere
weave .               # same, for the project whose root holds the current directory
weave -d nimbus       # start it, or restore its windows, without attaching: you stay where you are
weave                 # browse the projects, to open, bind, edit, kill or delete them, or run their tasks
weave kill nimbus     # kill the session and everything running in it
weave kill            # same, for the session you're in, if weave started it
```

weaver runs only what your config says: it knows no git and clones
nothing, and creates no directories.

## Documentation

- [Project config](docs/config.md): every field, `before` commands for toolchains, grids, the schema
- [Opening, killing and binding](docs/sessions.md): `setup` and `teardown`, restoring closed windows, capturing a session
- [The browser](docs/browser.md): summary and warnings, the drawing, tasks, keys
- [The config form](docs/config-form.md): cards, editing on the drawing, keys
- [Settings](docs/settings.md): the selected colour and the drawing's keys
- [Development](docs/development.md): building, testing, architecture
