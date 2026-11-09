# Development

```sh
go build ./...               # build everything
go run ./cmd/weave show X    # run without building
go test ./...                # run tests (the tmux adapter's need tmux)
golangci-lint run ./...      # format check, static analysis, layer boundaries
```

The tmux adapter's tests start private tmux servers on their own
sockets; they never touch your sessions.

## Architecture

The `Session` bounded context is organized by layer. `domain` imports
only the standard library, and `app` only that and `domain`, which
depguard allow-lists in `.golangci.yml` enforce. Use cases depend on ports; `infra`
implements them and `cmd/weave` wires everything together.

```mermaid
graph TD
    subgraph cmd["cmd/weave (Cobra CLI)"]
        CLI["weave, freeze, edit, kill, list, show, completion"]
    end
    subgraph app["internal/session/app"]
        UC["BrowseProjects, OpenProject, FreezeProject,<br/>EditProject, KillProject, ListProjects"]
        AP["ProjectBrowser, ProjectForm, Confirmer,<br/>ConfigEditor, ProjectDocuments (ports)"]
    end
    subgraph domain["internal/session/domain"]
        M["Project, Window, Pane, Split, Command, Env<br/>SessionPlan, WindowPlacement"]
        DP["ProjectRegistry, SessionFinder, SessionLauncher,<br/>SessionLister, SessionKiller, SessionCapturer,<br/>SessionLocator, Workspace (ports)"]
    end
    subgraph infra["internal/session/infra"]
        A["FileProjectRegistry + schema, Tmux,<br/>LocalWorkspace, Editor,<br/>TeaConfirmer, TeaBrowser, TeaConfigForm"]
    end

    CLI --> UC
    CLI --> A
    UC --> M
    UC --> DP
    UC --> AP
    A -.implements.-> DP
    A -.implements.-> AP
```

Agent workflows, such as testing weave in a real terminal, are in
[docs/agents/](agents/).
