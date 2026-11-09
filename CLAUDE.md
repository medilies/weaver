# Project rules

## Design

- SOLID and DDD: `internal/session/{domain,app,infra}` plus a thin `cmd/weave` Cobra layer. `domain` imports only the standard library, and `app` only that and `domain`; enforced by the `depguard` allow-lists in `.golangci.yml`.
- Ports about domain concepts (projects, tmux sessions, the workspace) live in `domain`; ports for talking to the user (forms, confirmation, editor, raw config documents, the project browser) live in `app`. `infra` holds every adapter: YAML configs and their schema, tmux, the local filesystem, the editor, the Bubble Tea confirmation dialog, the Bubble Tea config form and project browser. Keep logic out of the forms and the browser: they only collect answers checked by domain constructors, or a choice for a use case to carry out.
- Rich domain model, immutable value objects, expressive named constructors, fluent intention-revealing APIs. Unexported fields with getters, no setters, explicit error returns.
- Minimalism: add only what is needed now. No speculative abstractions, unused dependencies or placeholder files.
- Minimal and explicit in what weaver executes: it runs only what the user wrote in a config, and does nothing on their behalf. It knows nothing of git: no cloning, no repository status. Users add commands for that themselves.

## Ubiquitous language

Code, configs, CLI, browser text, docs and commits use these words, and only these. The config format and CLI are released, so when code and user-facing words disagree, the code is renamed. A new concept gets an entry here before it gets code.

| Term | Meaning | Config / CLI / browser | Code |
| --- | --- | --- | --- |
| project | A named dev environment weaver can open | `<name>.yml` | `Project`, `ProjectName` |
| settings | Weaver-wide preferences, the same for every project, in one file beside the configs; never part of a project's config | `settings.yml` | `Settings` |
| selected colour | The colour marking what is selected in the browser and the config form; cyan unless set | `selected-colour:` (settings) | `Settings` |
| config | A project's YAML file | `weave show`, `weave edit` | `ProjectRegistry`, `ConfigDocument` |
| registered | Having a config; freeze registers, delete unregisters | `weave list` | `ProjectRegistry.Names()` |
| root | The absolute directory panes are relative to | `root:` | `ProjectRoot` |
| workspace | The filesystem projects live in: their directories, and the programs on PATH | | `Workspace` |
| window, pane | tmux windows and panes, panes laid out by splits or a grid | `windows:`, `panes:`, `split:` | `Window`, `Pane`, `Split` |
| grid | A window's panes placed by naming them in rows and columns, sized as shares of the window or in cells. A window has splits or a grid, never both | `grid:` (`areas:`, `columns:`, `rows:`) | `Grid`, `Track`, `NewGridWindow` |
| layout cell | How tmux tiles a window: a pane, or cells side by side or stacked. Capture reads one and a grid is cut into one; both become splits | | `LayoutCell`, `CarvedPane` |
| pane name | A pane's name, shown in its top border | `name:` (pane) | `PaneName`, the `@label` pane option |
| dir | A window's or pane's directory, relative to the root; a pane's replaces its window's, `.` going back to the root | `dir:` | `PaneDir`, `Window.In`, `Pane.In` |
| env | Environment variables, set on the project, a window or a pane, the lowest winning | `env:` | `Env`, `WithEnv` |
| before commands | Commands typed and run in a pane before its own | `before:` | `Project`, `Window` and `Pane.Before()`, `RunningFirst` |
| command / prefill | What a pane runs, or types without running; with neither, it starts a plain shell | `command:` / `prefill:`; `starts` (shell, command, prefill) in the config form | `Startup`, `Pane.Running`, `Pane.Prefilling`, `Pane.WithoutCommand` |
| task | A command a pane offers to run on request, shown by its name or else its command: typed into the pane when its shell is idle, otherwise, if you agree, run in a throwaway window; you are then put where it runs. Only in an owned session | `tasks:` (pane) | `Task`, `Pane.Offering`, `RunTask`, `ActionRunTask`, `TaskRunner` |
| throwaway window | A window a task runs in when its pane is busy, starting in the pane's dir with its env and before commands, kept open after the task ends until you close it. Never captured or restored | | `TaskRunner.RunApart`, the `@weaver_throwaway` and `@weaver_task_of` window options |
| focus | The window open lands you in, and the pane each window selects; the first ones unless set. At most one per project and one per window | `focus:` (window, pane) | `WithFocus`, `Focusing`, `FocusedWindow()`, `FocusedPane()` |
| clear | Hiding a pane's before commands once they ran: `clear` is typed after them, then the note printed. Set on the project, a window or a pane, the lowest winning; off unless set | `clear:` | `Clearing`, `WithClearing`, `PanePlan.Clears()` |
| note | Lines printed in a pane before its shell starts, or after its before commands when it clears | `note:` | `Pane.WithNote` |
| session | A tmux session; a project's session is named after it | `weave freeze [session]` | `RunningSession`, `Session*` ports |
| running / not running | Whether a project's session exists | ● in the browser | `ProjectStatus.Running()` |
| owned session | A session weaver started; only those are restored | | `RunningSession.Owned()` |
| capture | Reading a running session's windows and panes, for freeze to start from | `weave freeze [session]` | `SessionCapturer.Capture` |
| plan | The windows and panes opening a project builds | | `SessionPlan`, `WindowPlan`, `PanePlan` |
| restoration | Putting back the configured windows an owned session lost | | `Project.Restoration`, `WindowPlacement` |
| freeze | Register a project through the config form, optionally from a running session | `weave freeze`, key `f` | `FreezeProject`, `ActionFreeze` |
| draft | What a form starts from: a name, a root, and the captured windows if any when freezing; a registered project and its config when editing, whose name stays; a freeze's name is typed | | `ProjectDraft`, `NameStays` |
| open | Start a project's session if not running, then attach or switch | `weave <project>`, enter | `OpenProject`, `ActionOpen` |
| setup commands | Commands run once in the root, in order, before opening builds a project's session; a failing one stops the open. Not "start", which is too close to a pane's startup | `setup:` | `Project.Setup()`, `SettingUp`, `CommandRunner`, `ShellRunner` |
| edit | Change a config in the config form, or raw in the editor, keeping it only if valid | `weave edit`, key `e`; `weave edit --raw`, key `E` | `EditProject`, `ActionEdit`, `ActionEditRaw` |
| config form | A full-screen form changing a project in place, a draft when freezing: a card for the project, then the window list and a card for the selected window, with its drawing, the pane list and the selected pane's card, each field checked as it is typed. Saving rewrites the config, so its comments go | `weave edit`, key `e`; `weave freeze`, key `f` | `TeaConfigForm`, `app.ProjectForm` |
| undo, redo | Going back to the project as it was before the config form's last change, or forward again | keys `u`, `ctrl+r` in the config form | `undone`, `redone` in `tea_config_form.go` |
| diff | What saving changes in the stored config, shown before the config form saves: comments the user wrote show as removed. Freezing under a registered name, it is what saving changes in that project's config | `ctrl+s` in the config form | `configDiff`, `review` in `tea_config_form.go` |
| card | The config form's box for the project, the windows or the selected pane: the one with the cursor, or with the cursor on its window list or pane list above it, has a heavy border, as does the selected pane in the drawing when the cursor is on it | config form | `card` in `tea_config_form.go` |
| trail | The config form's top line: the project, window and pane the cursor is in, and ● unsaved when there are changes | config form | `trail` in `tea_config_form.go` |
| kill | End a project's session and what runs in it. Not "stop" | `weave kill`, key `x` | `KillProject`, `ActionKill` |
| teardown commands | Commands run in the root, in order, before a kill ends a project's session; the session is killed even when one fails. Not "stop" | `teardown:` | `Project.Teardown()`, `TearingDown`, `CommandRunner`, `ShellRunner` |
| delete | Remove a config; a running session keeps running | key `d` | `ProjectRegistry.Delete`, `ActionDelete` |
| browser | The project list `weave` shows without arguments | `weave` | `BrowseProjects`, `ProjectBrowser`, `ProjectOverview` |
| summary | Where the selected project stands, at the top of the browser's details | browser | `summary` in `tea_browser.go` |
| drawing | A window's panes drawn as boxes in their proportions, each as it looks the moment it opens, the selected one in the selected colour unless a warning colours it. In the config form, panes are split, removed and resized on it with tmux's keys | browser details; config form, `%`, `"`, `x`, `H/L` | `drawing` in `tea_drawing.go`, `pressedOnDrawing` in `tea_config_form.go` |
| window list | The names of a project's windows, the selected one in a block of the selected colour: in the browser above the drawing, where ○ marks one an owned session lacks, which open restores; in the config form above the windows card, with `+ window`, clicked or picked with ←/→. Never "tabs" | `←/→` and click in the browser and the config form | `windowList` in `tea_browser.go`, `formWindowList`, `fieldWindowList` in `tea_config_form.go`, `listed`, `hotspot` in `tea_hotspot.go` |
| pane list | The names of the selected window's panes, or their numbers for those without, below the drawing, in the order it places them, the selected one in a block of the selected colour, clicked or picked: with Tab in the browser, with ←/→ in the config form, above the selected pane's card | Tab and click in the browser; `←/→` and click in the config form | `paneList`, `fieldPaneList` in `tea_config_form.go` |
| task list | The browser's second column, between the project list and the details: the selected project's tasks under `window › pane`, in config order, ● before one still running in its pane or throwaway window, clicked or picked to run one | `t` and click in the browser | `taskList` in `tea_tasks.go`, `ProjectOverview.TaskRuns`, `RunningCommands` |
| level | Where a pane's dir, before commands, env or clear is set: the project, the pane's window or the pane. The Tab key selects a pane in the drawing, and below it the browser lists what that pane gets, each with the level it comes from and, when it replaces a higher level's, what it is over. A pane's card in the config form shows the same, the higher levels' dimmed, and a window's card the project's variables | Tab key in the browser; config form | `levels`, `higherBefore`, `envLevels` in `tea_levels.go`, `fieldHigherEnv`, `envRows`, `overHigher` |
| filter | Text narrowing the browser's list to the projects whose names contain it, ignoring case | `/` in the browser | `filter` in `tea_browser.go` |
| notice | The browser's status line: what the last action did, or why it failed | browser | the `notice` passed to `ProjectBrowser.Browse` |
| warning | Something opening would likely fail on, or an exited command, in the panes it concerns | browser summary and drawing | `Warning`, `ProjectOverview.Warnings()` |
| rerun | Typing an exited command into its pane again, as a task is typed; you are then put in that pane. Only on a pane with the exited-command warning | key `r` in the browser | `RerunCommand`, `ActionRerun` |
| exited command | A pane's command, in an owned session, whose shell is back at its prompt: it crashed, was interrupted or finished, or the pane is still starting. A hint, not an error. Never a prefill, a plain shell or a task | browser summary and drawing | `ErrCommandExited` |
| program | The program a command starts, `npm` in `NODE_ENV=dev npm run dev`, checked to warn when missing | browser summary | `Command.Program`, `ErrMissingProgram` |

## Environment

- No Docker: weaver drives the host's real tmux server, not a container's. Build and run natively with `go build ./...` / `go run ./cmd/weave`.
- Go version is `1.26` everywhere, and `go.mod` is the source of truth: its `go` directive (`1.26.0`, which the `go` command insists on) sets the language version, and its `toolchain` directive the exact Go that CI and releases build with, read by setup-go's `go-version-file`. Bump `toolchain` when a 1.26.x patch ships.
- No database: project configs are plain YAML files (`<name>.yml`, one per project) under `os.UserConfigDir()/weaver/projects`, with `weaver.schema.json` beside them, read and written through the `domain.ProjectRegistry` port and the `infra.FileProjectRegistry` implementation. Keep `infra/weaver.schema.json` in step with the config format; a test checks saved configs only use declared fields.

## Tests

- Meaningful tests over coverage: test behaviour through public APIs, one reason to fail per test, descriptive names. `domain`/`app` tests use in-memory fakes; `infra` tests may touch disk (`t.TempDir()`) since they test the disk-backed adapter itself. No coverage targets.
- tmux adapter tests start a private server (own `-S` socket, `-f /dev/null`, `/bin/sh` panes) and never touch the user's sessions. tmux tests skip when tmux is missing, except in CI, which installs it.
- The confirmation dialog, the config form and the browser aren't unit-tested; check them by running `weave freeze`, `weave edit` and `weave` in a pane of a throwaway tmux server (`TMUX_TMPDIR`) with a temporary `XDG_CONFIG_HOME`. Before a release, and after one on the published binary, run the scenarios in `docs/agents/terminal-testing.md` in a real terminal, which also says how never to reach the user's own tmux server. A case those scenarios miss, once found, is added to them, kept free of anything from the user's machine.

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

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `medilies/weaver`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: the glossary is the Ubiquitous language table above, and ADRs live in `docs/adr/`. See `docs/agents/domain.md`.
