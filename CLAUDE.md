# Project rules

## Design

- SOLID and DDD: `internal/session/{domain,app,infra}` plus a thin `cmd/weave` Cobra layer. `domain` imports only the standard library, and `app` only that and `domain`; enforced by the `depguard` allow-lists in `.golangci.yml`.
- Ports about domain concepts (projects, tmux sessions, runtimes, the workspace) live in `domain`; ports for talking to the user (forms, confirmation, editor, raw config documents, the project browser) live in `app`. `infra` holds every adapter: YAML configs and their schema, tmux, git, installed runtimes, the editor, the huh forms and the Bubble Tea project browser. Keep logic out of the forms and the browser: they only collect answers checked by domain constructors, or a choice for a use case to carry out.
- Rich domain model, immutable value objects, expressive named constructors, fluent intention-revealing APIs. Unexported fields with getters, no setters, explicit error returns.
- Minimalism: add only what is needed now. No speculative abstractions, unused dependencies or placeholder files.

## Environment

- No Docker: weaver drives the host's real tmux server, not a container's. Build and run natively with `go build ./...` / `go run ./cmd/weave`.
- Go version is `1.26` everywhere. `go.mod`'s `go` directive is the source of truth; CI reads it via `go-version-file`. Change it in one place.
- No database: project configs are plain YAML files (`<name>.yml`, one per project) under `os.UserConfigDir()/weaver/projects`, with `weaver.schema.json` beside them, read and written through the `domain.ProjectRepository` port and the `infra.FileProjectRepository` implementation. Keep `infra/weaver.schema.json` in step with the config format; a test checks saved configs only use declared fields.
- Pinned PHP versions are linked under `os.UserCacheDir()/weaver/php`; Node versions come from nvm's install directory.

## Tests

- Meaningful tests over coverage: test behaviour through public APIs, one reason to fail per test, descriptive names. `domain`/`app` tests use in-memory fakes; `infra` tests may touch disk (`t.TempDir()`) since they test the disk-backed adapter itself. No coverage targets.
- tmux adapter tests start a private server (own `-S` socket, `-f /dev/null`, `/bin/sh` panes) and never touch the user's sessions. tmux and git tests skip when the tool is missing, except in CI, which installs tmux.
- The huh forms and the browser aren't unit-tested; check them by running `weave add` and `weave` in a pane of a throwaway tmux server (`TMUX_TMPDIR`) with a temporary `XDG_CONFIG_HOME`.

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
