# weaver quality report

Review of 2026-11-12, of the whole codebase on `main` at `3ac4ac3`, tagged v0.6.0. Numbers continue from the review of 2026-11-06, whose findings were fixed or moved to `docs/backlog.md` (`git log -- docs/quality-report.md` has them). What the backlog already lists isn't repeated here. This review's findings, 17 to 20 and two consistency items, are fixed: `git log -- docs/quality-report.md` has them.

`go build ./...`, `go vet ./...`, `go test ./...` and `golangci-lint run ./...` all pass with 0 issues, `go test -race` passes for `domain`, `app` and `cmd/weave`, and `go mod tidy -diff` changes nothing.

Line numbers refer to `3ac4ac3`. Findings marked **Tested** were reproduced on a copy of the repo: with scratch tests against private tmux 3.4 servers, or with the built `weave` and a throwaway `XDG_CONFIG_HOME`. The rest were checked by reading the code.

## Read closely, nothing found

- Quoting: tmux arguments (`argument`, `formatArgument`), pane names in borders (`#{q/h:@label}`), values read back (`sized`, `verbatim`, `startArgs`), and the startup script's `shellQuote`.
- Configs: strict decoding (unknown fields, a second document, a bare `~`, a null grid), canonical encoding, atomic saves through a symlink.
- Layouts: grid cuts and sizes in cells, `#{window_layout}` parsing, creation order, `WithoutPane` handing a pane's space back.
- Sessions: `Start` cleaning up after a failed step, restoration placing windows, a kill whose teardown commands fail from inside the session, `weave .` with nested and symlinked roots.
- `weave edit --raw`: Esc answers No, which reopens the editor on what was typed, so edits aren't lost.

## Not covered

- The browser, the config form and the confirmation dialog weren't driven in a terminal: they were read, and one drawing was rendered in a test.
- Nothing ran on macOS or on a tmux other than 3.4.
