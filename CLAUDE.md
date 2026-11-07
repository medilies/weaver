# Project rules

## Design

- SOLID and DDD: `internal/session/{domain,app,infra}` plus a thin `cmd/weave` Cobra layer. `domain` imports only the standard library, and `app` only that and `domain`; enforced by the `depguard` allow-lists in `.golangci.yml`.
- Ports about domain concepts (projects, tmux sessions, the workspace) live in `domain`; ports for talking to the user (forms, confirmation, editor, raw config documents, the project browser) live in `app`. `infra` holds every adapter: YAML configs and their schema, tmux, the local filesystem, the editor, the huh forms and the Bubble Tea project browser. Keep logic out of the forms and the browser: they only collect answers checked by domain constructors, or a choice for a use case to carry out.
- Rich domain model, immutable value objects, expressive named constructors, fluent intention-revealing APIs. Unexported fields with getters, no setters, explicit error returns.
- Minimalism: add only what is needed now. No speculative abstractions, unused dependencies or placeholder files.
- Minimal and explicit in what weaver executes: it runs only what the user wrote in a config, and does nothing on their behalf. It knows nothing of git: no cloning, no repository status. Users add commands for that themselves.

## Ubiquitous language

Code, configs, CLI, browser text, docs and commits use these words, and only these. The config format and CLI are released, so when code and user-facing words disagree, the code is renamed. A new concept gets an entry here before it gets code.

| Term | Meaning | Config / CLI / browser | Code |
| --- | --- | --- | --- |
| project | A named dev environment weaver can open | `<name>.yml` | `Project`, `ProjectName` |
| config | A project's YAML file | `weave show`, `weave edit` | `ProjectRegistry`, `ConfigDocument` |
| registered | Having a config; freeze registers, delete unregisters | `weave list` | `ProjectRegistry.Names()` |
| root | The absolute directory panes are relative to | `root:` | `ProjectRoot` |
| window, pane | tmux windows and panes, panes laid out by splits | `windows:`, `panes:`, `split:` | `Window`, `Pane`, `Split` |
| pane name | A pane's name, shown in its top border | `name:` (pane) | `PaneName`, the `@label` pane option |
| dir | A pane's directory, relative to the root | `dir:` | `PaneDir` |
| env | Environment variables, a pane's overriding the project's | `env:` | `Env`, `WithEnv` |
| before commands | Commands typed and run in a pane before its own | `before:` | `Project`, `Window` and `Pane.Before()`, `RunningFirst` |
| command / prefill | What a pane runs, or types without running | `command:` / `prefill:` | `Startup`, `Pane.Running`, `Pane.Prefilling` |
| note | Lines printed in a pane before its shell starts | `note:` | `Pane.WithNote` |
| session | A tmux session; a project's session is named after it | `weave freeze [session]` | `RunningSession`, `Session*` ports |
| running / not running | Whether a project's session exists | ● in the browser | `ProjectStatus.Running()` |
| owned session | A session weaver started; only those are restored | | `RunningSession.Owned()` |
| plan | The windows and panes opening a project builds | | `SessionPlan`, `WindowPlan`, `PanePlan` |
| restoration | Putting back the configured windows an owned session lost | | `Project.Restoration`, `WindowPlacement` |
| freeze | Register a project through forms, optionally from a running session | `weave freeze`, key `f` | `FreezeProject`, `ActionFreeze` |
| open | Start a project's session if not running, then attach or switch | `weave <project>`, enter | `OpenProject`, `ActionOpen` |
| edit | Change a config in the editor, keeping it only if valid | `weave edit`, key `e` | `EditProject`, `ActionEdit` |
| kill | End a project's session and what runs in it. Not "stop" | `weave kill`, key `x` | `KillProject`, `ActionKill` |
| delete | Remove a config; a running session keeps running | key `d` | `ProjectRegistry.Delete`, `ActionDelete` |
| browser | The project list `weave` shows without arguments | `weave` | `BrowseProjects`, `ProjectBrowser`, `ProjectOverview` |
| warning | Something opening would likely fail on | browser summary | `ProjectOverview.Warnings()` |

## Environment

- No Docker: weaver drives the host's real tmux server, not a container's. Build and run natively with `go build ./...` / `go run ./cmd/weave`.
- Go version is `1.26` everywhere, and `go.mod` is the source of truth: its `go` directive (`1.26.0`, which the `go` command insists on) sets the language version, and its `toolchain` directive the exact Go that CI and releases build with, read by setup-go's `go-version-file`. Bump `toolchain` when a 1.26.x patch ships.
- No database: project configs are plain YAML files (`<name>.yml`, one per project) under `os.UserConfigDir()/weaver/projects`, with `weaver.schema.json` beside them, read and written through the `domain.ProjectRegistry` port and the `infra.FileProjectRegistry` implementation. Keep `infra/weaver.schema.json` in step with the config format; a test checks saved configs only use declared fields.

## Tests

- Meaningful tests over coverage: test behaviour through public APIs, one reason to fail per test, descriptive names. `domain`/`app` tests use in-memory fakes; `infra` tests may touch disk (`t.TempDir()`) since they test the disk-backed adapter itself. No coverage targets.
- tmux adapter tests start a private server (own `-S` socket, `-f /dev/null`, `/bin/sh` panes) and never touch the user's sessions. tmux tests skip when tmux is missing, except in CI, which installs it.
- The huh forms and the browser aren't unit-tested; check them by running `weave freeze` and `weave` in a pane of a throwaway tmux server (`TMUX_TMPDIR`) with a temporary `XDG_CONFIG_HOME`.

## Workflow

- At the end of each change, ask: review then commit / commit then review / commit without review.
- Reviews always run in a fresh subagent that has not seen the work, never in the main context.
- Commits follow Conventional Commits v1.0.0. Never add Co-Authored-By trailers or any mention of Claude/AI to commits or PRs.
- Use a Mermaid diagram (or an inline visual) when an explanation would otherwise be long.

## Commands

- `go build ./...` — build everything
- `go run ./cmd/weave <args>` — run without building
- `go test ./...` — run tests
- `golangci-lint run ./...` — format check, static analysis, layer boundaries
