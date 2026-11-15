# Project rules

## Design

- SOLID and DDD: `internal/session/{domain,app,infra}` plus a thin `cmd/weave` Cobra layer. `domain` imports only the standard library, and `app` only that and `domain`; enforced by the `depguard` allow-lists in `.golangci.yml`.
- Ports about domain concepts (projects, tmux sessions, the workspace) live in `domain`; ports for talking to the user (forms, confirmation, editor, raw config documents, the project browser) live in `app`. `infra` holds every adapter, such as the YAML configs and settings with their schemas, tmux, and the Bubble Tea screens. Keep logic out of the forms and the browser: they only collect answers checked by domain constructors, or a choice for a use case to carry out.
- Rich domain model, immutable value objects, expressive named constructors, fluent intention-revealing APIs. Unexported fields with getters, no setters, explicit error returns.
- Minimalism: add only what is needed now. No speculative abstractions, unused dependencies or placeholder files.
- Minimal and explicit in what weaver executes: it runs only what the user wrote in a config, and does nothing on their behalf. It knows nothing of git: no cloning, no repository status. Users add commands for that themselves.

## Ubiquitous language

Code, configs, CLI, browser text, docs and commits use these words, and only these. The config format and CLI are released, so when code and user-facing words disagree, the code is renamed. A new concept gets an entry here before it gets code.

| Term | Meaning | Config / CLI / browser | Code |
| --- | --- | --- | --- |
| project | A named dev environment weaver can open | `<name>.yml` or `<name>.yaml` | `Project`, `ProjectName` |
| settings | Weaver-wide preferences, the same for every project, in one file beside the configs; never part of a project's config | `settings.yml` | `Settings` |
| popup | A tmux popup the size of the window, which the browser and the config form re-run themselves in when started in a tmux pane, if the settings ask for it; off unless set | `popup:` (settings) | `Settings.Popup`, `Tmux.Popup` |
| selected colour | The colour marking what is selected, and where the cursor is, in the browser and the config form; cyan unless set. In a window list or pane list, the selected item is underlined, and in this colour while the cursor is on the list, or in the browser while it is in the project list | `selected-colour:` (settings) | `Settings`, `theme` |
| theme | How weaver's screens are styled: the selected colour, the shades (bright, plain, muted) and the state colours (green, yellow, red) | | `theme` in `theme.go` |
| keys | The settings choosing the browser's and the config form's action keys, one key each, the drawing keys among them; moving, enter, esc, Tab and ctrl+c are fixed, as are the config form's h, l, o, d, y and n. Two actions available at once never share a key | `keys:` (settings) | `keymap`, `browserActions`, `formActions` in `keymap.go` |
| drawing keys | The config form's keys splitting, removing and resizing panes on the drawing; tmux's unless set | `keys:` (settings) | `DrawingKeys` |
| config | A project's YAML file, shown in the browser's details | browser, key `y` | `ProjectRegistry`, `ConfigDocument` |
| registered | Having a config; bind registers, delete unregisters | the browser's project list | `ProjectRegistry.Names()` |
| root | The absolute directory panes are relative to | `root:` | `ProjectRoot` |
| workspace | The filesystem projects live in: their directories, and the programs on PATH | | `Workspace` |
| window, pane | tmux windows and panes, panes laid out by splits or a grid | `windows:`, `panes:`, `split:` | `Window`, `Pane`, `Split` |
| grid | A window's panes placed by naming them in rows and columns, sized as shares of the window or in terminal cells; not to be confused with the slots of its areas, which hold names. A window has splits or a grid, never both | `grid:` (`areas:`, `columns:`, `rows:`) | `Grid`, `Track`, `NewGridWindow`, `WindowPlan.SizedTo`, `Window.LaidOutBy`, `Grid.WithAreas`, `Window.Listing` |
| layout | Whether a window's panes are placed by splits or by a grid. The config form turns a grid into the splits it makes, never back | `layout` row (grid, splits) in the config form | `Window.InSplits`, `fieldLayout` in `tea_config_form.go` |
| layout cell | How tmux tiles a window: a pane, or cells side by side or stacked. Capture reads one and a grid is cut into one; both become splits | | `LayoutCell`, `CarvedPane` |
| pane name | A pane's name, shown in its top border | `name:` (pane) | `PaneName`, the `@label` pane option |
| dir | A window's or pane's directory, relative to the root; a pane's replaces its window's, `.` going back to the root | `dir:` | `PaneDir`, `Window.In`, `Pane.In` |
| env | Environment variables, set on the project, a window or a pane, the lowest winning | `env:` | `Env`, `WithEnv` |
| before commands | Commands typed and run in a pane before its own | `before:` | `Project`, `Window` and `Pane.Before()`, `RunningFirst` |
| command / prefill | What a pane runs, or types without running; with neither, it starts a plain shell | `command:` / `prefill:`; `starts` (shell, command, prefill) in the config form | `Startup`, `Pane.Running`, `Pane.Prefilling`, `Pane.WithoutCommand` |
| task | A command a pane offers to run on request, shown by its name or else its command: typed into the pane when its shell is idle, otherwise, if you agree, run in a throwaway window. Enter on a ● task goes where it runs, rather than running it again: its pane when it runs there, else its most recent copy. Never typed into the pane the browser runs in. Only in an owned session. Keys in `docs/browser.md` | `tasks:` (pane) | `Task`, `Pane.Offering`, `RunTask`, `ActionRunTask`, `RunTask.HandleStaying`, `ActionRunTaskStaying`, `TaskRunner`, `TaskRunner.Reach`, `RunningCommand`, `ErrPaneRunsWeaver` |
| throwaway window | A window a task runs in when its pane is busy and you agree, or when you ask for one with `w`, starting in the pane's dir with its env and before commands, kept open after the task ends until you close it. Never captured or restored | | `TaskRunner.RunApart`, `RunTask.HandleApart`, `ActionRunTaskApart`, the `@weaver_throwaway` and `@weaver_task_of` window options |
| copy | A task running in its pane or in one of its throwaway windows; one task can run as several copies at once, the most recent being the last run | ● in the task list | `RunningCommand`, `RunningCommand.Apart`, `RunningCommand.Since` |
| focus | The window open lands you in, and the pane each window selects; the first ones unless set. At most one per project and one per window | `focus:` (window, pane) | `WithFocus`, `Focusing`, `FocusedWindow()`, `FocusedPane()` |
| clear | Hiding a pane's before commands once they ran: `clear` is typed after them, then the note printed. Set on the project, a window or a pane, the lowest winning; off unless set | `clear:` | `Clearing`, `WithClearing`, `PanePlan.Clears()` |
| note | Lines printed in a pane before its shell starts, or after its before commands when it clears | `note:` | `Pane.WithNote` |
| session | A tmux session; a project's session is named after it | `weave bind [session]` | `RunningSession`, `Session*` ports |
| running / not running | Whether a project's session exists, kept up to date while the browser is open | ● and a green name in the browser | `ProjectStatus.Running()` |
| owned session | A session weaver started; only those are restored | | `RunningSession.Owned()` |
| capture | Reading a running session's windows and panes, for bind to start from | `weave bind [session]` | `SessionCapturer.Capture` |
| plan | The windows and panes opening a project builds | | `SessionPlan`, `WindowPlan`, `PanePlan` |
| restoration | Putting back the configured windows an owned session lost | | `Project.Restoration`, `WindowPlacement` |
| bind | Register a project through the config form, optionally from a running session; binding, bound. Not "freeze" (ADR 0002), nor "bind" as tmux's key bindings | `weave bind`, key `b` | `BindProject`, `ActionBind` |
| draft | What a form starts from: a name, a root, and the captured windows if any when binding; a registered project and its config when editing, whose name stays; when binding, the name is typed | | `ProjectDraft`, `NameStays` |
| open | Start a project, then attach or switch to its session | `weave <project>`, enter | `OpenProject`, `ActionOpen` |
| ready | A pane once its before commands ran, `clear` typed if set, its note printed, and its command started or its prefill typed; at once with none of these. Open puts you in a session once the panes it created are ready | | |
| start | Create a project's session if not running, or restore the windows its owned session lacks, without attaching or switching: you stay where you are, told what was started or restored, or that it was already running. Opening starts first. Only for projects: a task is run. "Detached" only names its flag, as in tmux | `weave <project> --detached` (`-d`), key `s` in the browser (start) | `OpenProject.HandleStart`, `ActionStart` |
| setup commands | Commands run once in the root, in order, before starting builds a project's session; a failing one stops the start. Not "start", which builds the session | `setup:` | `Project.Setup()`, `SettingUp`, `CommandRunner`, `ShellRunner` |
| edit | Change a config in the config form, or raw in the editor, keeping it only if valid. Not a command, like list and show (ADR 0003) | key `e`; key `E` | `EditProject`, `ActionEdit`, `ActionEditRaw` |
| config form | A full-screen form changing a project in place, a draft when binding: a card for the project, then the window list and a card for the selected window, with its drawing, the pane list and the selected pane's card, each field checked as it is typed | key `e`; `weave bind`, key `b` | `TeaConfigForm`, `app.ProjectForm` |
| undo, redo | Going back to the project as it was before the config form's last change, or forward again | keys `u`, `ctrl+r` in the config form, unless the settings move them | `undone`, `redone` in `tea_config_form.go` |
| diff | What saving changes in the stored config, shown before the config form saves: comments the user wrote show as removed. Binding under a registered name, it is what saving changes in that project's config | `ctrl+s` in the config form | `configDiff` in `config_diff.go`, `review` in `tea_config_form.go` |
| card | A rounded box around a part of the screen, titled in its top border: in the config form, the project, the windows or the selected pane; in the browser, the project list, the task list or the details. The innermost one holding the cursor, or with the cursor on its window list or pane list above it, has its border in the selected colour and its title bright, the others muted; the browser's details never hold it | config form, browser | `card` in `tea_config_form.go` |
| trail | The config form's top line: the project, window and pane the cursor is in, and ● unsaved when there are changes | config form | `trail` in `tea_config_form.go` |
| kill | End a project's session and what runs in it. Not "stop" | `weave kill`, key `x` | `KillProject`, `ActionKill` |
| teardown commands | Commands run in the root, in order, before a kill ends a project's session; the session is killed even when one fails. Not "stop" | `teardown:` | `Project.Teardown()`, `TearingDown`, `CommandRunner`, `ShellRunner` |
| delete | Remove a config; a running session keeps running | key `d` | `ProjectRegistry.Delete`, `ActionDelete` |
| browser | The project list `weave` shows without arguments | `weave` | `BrowseProjects`, `ProjectBrowser`, `ProjectOverview` |
| summary | Where the selected project stands, at the top of the browser's details, kept up to date while shown | browser | `summary` in `tea_browser.go`, `LiveSessions.Live` |
| drawing | A window's panes drawn as boxes in their proportions, each as it looks the moment it opens, the selected one in the selected colour unless a warning colours it, and with a heavy border when the cursor is on the drawing. In the config form, panes are split, removed and resized on it with tmux's keys unless the settings say otherwise | browser details; config form, `%`, `"`, `x`, `H/L` | `drawing` in `tea_drawing.go`, `pressedOnDrawing` in `tea_config_form.go` |
| window list | The names of a project's windows: in the browser above the drawing, where ○ marks one an owned session lacks, which open restores; in the config form above the windows card, with `+ window`, clicked or picked with ←/→. Never "tabs" | `←/→` and click in the browser and the config form | `windowList` in `tea_browser.go`, `formWindowList`, `fieldWindowList` in `tea_config_form.go`, `listed`, `hotspot` in `tea_hotspot.go` |
| miniature | A window drawn small under its name in the browser's window list, when the project has several windows and the lines to spare, unless the settings turn them off | browser; `miniatures:` (settings) | `miniatures` in `tea_browser.go` |
| preview | The selected window of a running owned session as it is now, in the browser in place of its drawing: its panes in tmux's layout, named as in the session, each showing its last lines in their colours, kept up to date while shown. Shown unless switched off. Not the drawing, which shows a window as it opens | key `p` in the browser switches between it and the drawing; `preview:` (settings) | `WindowPreview`, `PanePreview`, `LiveSessions`, `ProjectOverview.Previewable`, `previewDrawing` in `tea_drawing.go`, `tea_preview.go` |
| pane list | The names of the selected window's panes, or their numbers for those without, below the drawing, in the order it places them, clicked or picked: with Tab in the browser, with ←/→ in the config form, above the selected pane's card | Tab and click in the browser; `←/→` and click in the config form | `paneList`, `fieldPaneList` in `tea_config_form.go` |
| task list | The browser's second column, between the project list and the details: the selected project's tasks under `window › pane`, in config order, ● before one still running in its pane or throwaway window, kept up to date while shown, clicked or picked to run one | `t` and click in the browser | `taskList` in `tea_tasks.go`, `ProjectOverview.TaskRuns`, `LiveSessions.Live`, `RunningCommands` |
| level | Where a pane's dir, before commands, env or clear is set: the project, the pane's window or the pane. The Tab key selects a pane in the drawing, and below it the browser lists what that pane gets, each with the level it comes from and, when it replaces a higher level's, what it is over. A pane's card in the config form shows the same, the higher levels' muted, and a window's card the project's variables | Tab key in the browser; config form | `levels`, `higherBefore`, `envLevels` in `tea_levels.go`, `fieldHigherEnv`, `envRows`, `overHigher` |
| filter | Text narrowing the browser's list to the projects whose names contain it, ignoring case | `/` in the browser | `filter` in `tea_browser.go` |
| notice | The browser's status line: what the last action did, or why it failed, in red after ✗, the preview's hints staying plain; a question the browser asks, answered there with y or enter, anything else declining, the keys line below saying so; or, while an action runs, a spinner and what it is doing, the keys line saying a second ctrl+c leaves the browser | browser | `Notice`, `Telling`, `FailingWith`, passed to `ProjectBrowser.Browse`; `ProjectBrowser.Confirm`, `ProjectBrowser.Working` |
| warning | Something opening would likely fail on, or an exited command, in the panes it concerns | browser summary and drawing | `Warning`, `ProjectOverview.Warnings()` |
| reload | Rebuilding a pane, or a whole window, of an owned session as its config says now, ending what runs in it: the pane restarted in place, the window replaced where it is. Not rerun, which ends nothing, nor restoration, which only puts back missing windows | key `R` in the browser, asking pane or window | `Reload`, `ActionReloadPane`, `ActionReloadWindow`, `SessionReloader` |
| interrupt | Typing ctrl+c into a pane of an owned session, as if you pressed it there: a ● task in its pane when it runs there, else its most recent copy, one copy per press; or what runs in the pane selected. Never the pane the browser runs in. Not "kill", which ends a session, nor "stop" | key `i` in the browser | `Interrupt`, `ActionInterruptTask`, `ActionInterruptPane`, `TaskRunner.Interrupt`, `TaskRunner.InterruptRun`, `ErrNothingToInterrupt` |
| rerun | Typing an exited command into its pane again, as a task is typed; you are then put in that pane. Only on a pane with the exited-command warning | key `r` in the browser | `RerunCommand`, `ActionRerun` |
| exited command | A pane's command, in an owned session, whose shell is back at its prompt: it crashed, was interrupted or finished, or the pane is still starting. A hint, not an error, kept up to date while the browser shows its project. Never a prefill, a plain shell or a task | browser summary and drawing | `ErrCommandExited`, `LiveSessions.Live` |
| program | The program a command starts, `npm` in `NODE_ENV=dev npm run dev`, checked to warn when missing | browser summary | `Command.Program`, `ErrMissingProgram` |

## Environment

- No Docker: weaver drives the host's real tmux server, not a container's.
- Go version is `1.26` everywhere, and `go.mod` is the source of truth: its `go` directive (`1.26.0`, which the `go` command insists on) sets the language version, and its `toolchain` directive the exact Go that CI and releases build with, read by setup-go's `go-version-file`. Bump `toolchain` when a 1.26.x patch ships.
- No database: settings are one YAML file, `os.UserConfigDir()/weaver/settings.yml`, read by `infra.LoadSettings` with `settings.schema.json` beside it; project configs are plain YAML files (`<name>.yml` or `<name>.yaml`, one per project, new ones `.yml`) under `os.UserConfigDir()/weaver/projects`, with `weaver.schema.json` beside them, read and written through the `domain.ProjectRegistry` port and the `infra.FileProjectRegistry` implementation. Keep `infra/weaver.schema.json` in step with the config format; a test checks saved configs only use declared fields.

## Tests

- Meaningful tests over coverage: test behaviour through public APIs, one reason to fail per test, descriptive names. `domain`/`app` tests use in-memory fakes; `infra` tests may touch disk (`t.TempDir()`) since they test the disk-backed adapter itself. No coverage targets.
- Never run `go test` with `-v`: its output floods the context (about 136 KB for the unit tests). A failure prints without it; narrow one down with `-run`, and the terminal tests' `-keep` or `-traces`.
- tmux adapter tests start a private server (own `-S` socket, `-f /dev/null`, `/bin/sh` panes) and never touch the user's sessions. tmux tests skip when tmux is missing, except in CI, which installs it.
- The config form's model is unit-tested in `tea_config_form_test.go`; the browser's and the confirmation dialog's models aren't. What needs tmux or a real terminal is checked by the terminal tests in `test/terminal`: see `docs/agents/terminal-testing.md`, including how never to reach the user's tmux server. A case they miss, once found, is added there.

## Docs

- A change updates, in the same commit, every doc it makes wrong: the README and `docs/*.md`, the schemas' descriptions, this file's glossary (its Code column included), `docs/development.md`, `docs/agents/` and `docs/left-out.md`, and closes the issue it finishes (`Closes #<n>`). Before committing, grep the docs for each key, flag, command, file and identifier the change renames or removes.
- Each fact lives in one doc, and the others link to it. Users: the README (what weaver is, install, usage), `config.md` (fields), `sessions.md` (open, start, kill, bind), `browser.md`, `config-form.md`, `settings.md`, `tmux.md` (tmux for newcomers, the demo's theme). Contributors: `development.md` (commands, architecture, issues, releasing), `left-out.md` (what weaver leaves out, and why). Agents: this file and `docs/agents/`.
- Never list in full what the code lists (use cases, ports, adapters, keys a screen already shows): name a few as examples, so the list can't go stale.
- User docs say what weave does, not how: no internals, no reasons behind workarounds, no history. History is in git, decisions in ADRs, which are records and aren't updated.
- A screen shown in a doc is copied from a real run (a terminal-test snapshot, or weave on a private tmux server with its own `TMUX_TMPDIR`), never drawn by hand.
- Short sentences, and lists over long paragraphs.
- A review checks the docs a change touches against these rules.

## Workflow

- At the end of each iteration of work, run the tests, the terminal tests and both lints (commands in `docs/development.md`), and fix what fails before going on.
- At the end of each change, ask: review then commit / commit then review / commit without review.
- Reviews always run in a fresh subagent that has not seen the work, never in the main context.
- Commits follow Conventional Commits v1.0.0. Never add Co-Authored-By trailers or any mention of Claude/AI to commits or PRs.
- Use a Mermaid diagram (or an inline visual) when an explanation would otherwise be long.

## Commands

Build, run, test and lint commands, the terminal tests' included, are in `docs/development.md`.

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `medilies/weaver`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: the glossary is the Ubiquitous language table above, and ADRs live in `docs/adr/`. See `docs/agents/domain.md`.
