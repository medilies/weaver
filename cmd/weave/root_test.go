package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// `weave .` opens the project holding the current directory: "." is no
// project's name.
func TestDotOpensTheProjectHoldingTheCurrentDirectory(t *testing.T) {
	root := newRootCommand(application{openProject: app.NewOpenProject(registered{}, nil, infra.NewLocalWorkspace(), nil)})
	root.SetArgs([]string{"."})
	root.SetErr(io.Discard)
	if err := root.Execute(); !errors.Is(err, app.ErrNoProjectAt) {
		t.Fatalf("got %v, want ErrNoProjectAt from looking up the current directory", err)
	}
}

// `weave bind --root=~bob/x` would otherwise bind <cwd>/~bob/x: only
// ~/ is expanded, as in a config.
func TestBindRejectsARootUnderAnotherUsersHome(t *testing.T) {
	root := newRootCommand(application{})
	root.SetArgs([]string{"bind", "--root=~bob/x"})
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "only a leading ~/") {
		t.Fatalf("got %v, want the ~bob root rejected", err)
	}
}

// `weave <name>` runs the command called name, if there is one, so no
// project may take a command's name.
func TestEveryCommandNameIsAReservedProjectName(t *testing.T) {
	root := newRootCommand(application{})
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	for _, cmd := range root.Commands() {
		for _, name := range append([]string{cmd.Name()}, cmd.Aliases...) {
			t.Run(name, func(t *testing.T) {
				if _, err := domain.NewProjectName(name); !errors.Is(err, domain.ErrReservedProjectName) {
					t.Fatalf("got %v, want %q reserved in domain.NewProjectName", err, name)
				}
			})
		}
	}
}

// `weave --start` alone has no project to open: it doesn't fall back
// to the browser.
func TestStartNeedsAProject(t *testing.T) {
	root := newRootCommand(application{})
	root.SetArgs([]string{"--start"})
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "needs a project") {
		t.Fatalf("got %v, want --start refused without a project", err)
	}
}
