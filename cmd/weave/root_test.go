package main

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

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
