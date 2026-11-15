# Development

```sh
go build ./...               # build everything
go run ./cmd/weave X         # run without building
go test ./...                # run tests (the tmux adapter's need tmux)
golangci-lint run ./...      # format check, static analysis, layer boundaries

cd test/terminal
go test -tags terminal .                    # weave in a real terminal emulator
golangci-lint run --build-tags terminal ./...
```

Tests that need tmux start private tmux servers on their own sockets;
they never touch your sessions. The terminal tests are described in
[docs/agents/terminal-testing.md](agents/terminal-testing.md).

## Architecture

Use cases depend on ports; `infra` implements them and `cmd/weave`
wires everything together. The layer rules are in
[CLAUDE.md](../CLAUDE.md#design). The types below are examples, not a
full list.

```mermaid
graph TD
    subgraph cmd["cmd/weave (Cobra CLI)"]
        CLI["weave, bind, kill, completion"]
    end
    subgraph app["internal/session/app"]
        UC["use cases, e.g. OpenProject, BindProject, RunTask"]
        AP["ports for the user, e.g. ProjectBrowser, ProjectForm"]
    end
    subgraph domain["internal/session/domain"]
        M["model, e.g. Project, Window, Pane, SessionPlan"]
        DP["ports, e.g. ProjectRegistry, SessionLauncher, Workspace"]
    end
    subgraph infra["internal/session/infra"]
        A["adapters, e.g. FileProjectRegistry, Tmux, TeaBrowser"]
    end

    CLI --> UC
    CLI --> A
    UC --> M
    UC --> DP
    UC --> AP
    A -.implements.-> DP
    A -.implements.-> AP
```

## Releasing

Pushing a `vX.Y.Z` tag runs the tests, then GoReleaser builds the
binaries and a `.deb`, and the apt repository on GitHub Pages is rebuilt,
signed with the key in the `APT_SIGNING_KEY` secret. Then run the
terminal tests on the published binaries, as
[terminal-testing.md](agents/terminal-testing.md#before-and-after-a-release) says.
