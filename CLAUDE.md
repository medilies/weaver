# Project rules

## Design
- SOLID and DDD: `internal/session/{domain,app,infra}` plus a thin `cmd/weaver` Cobra layer. `domain` and `app` never import Cobra or `infra`; enforced by the `depguard` rules in `.golangci.yml`.
- Rich domain model, immutable value objects, expressive named constructors, fluent intention-revealing APIs. Unexported fields with getters, no setters, explicit error returns.
- Minimalism: add only what is needed now. No speculative abstractions, unused dependencies or placeholder files.

## Environment
- No Docker: weaver drives the host's real tmux server, not a container's. Build and run natively with `go build ./...` / `go run ./cmd/weaver`.
- Go version is `1.26` everywhere. `go.mod`'s `go` directive is the source of truth; CI reads it via `go-version-file`. Change it in one place.
- No database: project configs are plain JSON files on disk, read through the `domain.ProjectRepository` port and the `infra.JSONProjectRepository` implementation.

## Tests
- Meaningful tests over coverage: test behaviour through public APIs, one reason to fail per test, descriptive names. `domain`/`app` tests use in-memory fakes; `infra` tests may touch disk (`t.TempDir()`) since they test the disk-backed adapter itself. No coverage targets.

## Workflow
- At the end of each change, ask: review then commit / commit then review / commit without review.
- Reviews always run in a fresh subagent that has not seen the work, never in the main context.
- Commits follow Conventional Commits v1.0.0. Never add Co-Authored-By trailers or any mention of Claude/AI to commits or PRs.
- Use a Mermaid diagram (or an inline visual) when an explanation would otherwise be long.

## Commands
- `go build ./...` — build everything
- `go run ./cmd/weaver <args>` — run without building
- `go test ./...` — run tests
- `golangci-lint run ./...` — format check, static analysis, layer boundaries
