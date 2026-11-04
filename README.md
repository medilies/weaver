# weaver

Weaver drives tmux sessions, windows and panes from a per-project JSON
config, so starting a dev environment is one command instead of a
bespoke script per project.

## Status

Early bootstrap. weaver reads the full project config below, and
`weaver show <project>` prints a registered project; building the tmux
session from it is the next piece of work.

## Commands

```sh
go build ./...              # build everything
go run ./cmd/weaver show X  # run without building
go test ./...                # run tests
golangci-lint run ./...      # format check, static analysis, layer boundaries
```

## Project config

A project is a JSON file at `~/.config/weaver/projects/<name>.json`. The
file name is the project's name and its tmux session's name: letters,
digits, `-` and `_`.

The smallest config opens one window with one shell in the root:

```json
{ "root": "~/dev/weaver" }
```

A fuller one, with three windows:

```json
{
  "root": "~/dev/bicyclette",
  "repositories": { ".": "https://github.com/medilies/bicyclette.git" },
  "windows": [
    { "name": "api", "panes": [
      { "name": "api shell", "dir": "api" },
      { "name": "task dev", "dir": "api", "split": { "direction": "right", "size": "40%" }, "command": "task dev" },
      { "name": "psql", "split": { "direction": "below" }, "command": "bicyclette psql" } ] },
    { "name": "app", "panes": [
      { "name": "app shell", "dir": "app" },
      { "name": "flutter run", "dir": "app", "split": { "direction": "right", "size": "40%" }, "command": "bicyclette app" },
      { "name": "emulator", "split": { "direction": "below", "size": "30%" }, "prefill": "bicyclette emulator" } ] },
    { "name": "repo" }
  ]
}
```

A back end and a front end, each its own repository, on different
toolchain versions:

```json
{
  "root": "~/dev/shop",
  "repositories": {
    "back": "git@bitbucket.org:acme/shop-back.git",
    "front": "git@bitbucket.org:acme/shop-front.git"
  },
  "windows": [
    { "name": "shop", "panes": [
      { "name": "back", "dir": "back", "runtimes": { "php": "8.4" } },
      { "name": "front", "dir": "front", "split": { "direction": "right", "size": "30%" },
        "runtimes": { "node": "20" },
        "note": ["Usual commands:", "  npm i", "  npm run dev"] } ] }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `root` | Directory the session is rooted at: absolute, or starting with `~/`. Required. |
| `repositories` | Directory (relative to `root`; `.` is `root`) → git remote. |
| `env` | Variables for every pane. Values are literal. `PATH` is managed by `runtimes`. |
| `runtimes` | Versions for every pane: `php` (e.g. `8.4`) and `node` (e.g. `20`). |
| `windows` | Windows in order, with unique `name`s. None: one window named after the project. |
| `windows[].panes` | Panes in creation order. None: one shell in `root`. |
| `name` (pane) | Shown in the pane's top border. Required and unique when a window has several panes. |
| `dir` | Where the pane's shell starts, relative to `root` (or absolute). |
| `split` | How every pane but the first is made: `direction` (`right` or `below`), `size` (`40%`, or `20` cells; default half of the split pane), `of` (the pane it splits; default the previous one). |
| `command` | Typed into the shell and run, so it stays in the shell history. |
| `prefill` | Typed at the prompt but not run: press Enter to run it. |
| `note` | Lines printed before the shell starts. |
| `env`, `runtimes` (pane) | Override the project's, per key. |

Unknown fields are errors: they are usually typos.

## Architecture

The `Session` bounded context is organized by layer. `domain` and `app`
never import Cobra or `infra` — enforced by a `depguard` rule in
`.golangci.yml`.

```mermaid
graph TD
    subgraph cmd["cmd/weaver (Cobra CLI)"]
        CLI["weaver show"]
    end
    subgraph app["internal/session/app"]
        LP["LoadProject use case"]
    end
    subgraph domain["internal/session/domain"]
        P["Project, ProjectName, ProjectRoot"]
        PR["ProjectRepository (port)"]
    end
    subgraph infra["internal/session/infra"]
        JSON["JSONProjectRepository"]
    end

    CLI --> LP
    CLI --> JSON
    LP --> PR
    JSON -.implements.-> PR
```
