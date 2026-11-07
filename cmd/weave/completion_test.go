package main

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// registered is a fake domain.ProjectRepository that only knows names.
type registered []string

func (r registered) FindByName(name domain.ProjectName) (domain.Project, error) {
	return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
}

func (r registered) Save(domain.Project) error {
	return nil
}

func (r registered) Delete(domain.ProjectName) error {
	return nil
}

func (r registered) Names() ([]domain.ProjectName, error) {
	var names []domain.ProjectName
	for _, raw := range r {
		name, err := domain.NewProjectName(raw)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

// running is a fake domain.SessionLister.
type running []string

func (r running) Running() ([]string, error) {
	return r, nil
}

// complete runs shell completion for args, the last one being the word
// being typed, and returns the suggestions.
func complete(t *testing.T, args ...string) []string {
	t.Helper()
	weaver := application{listProjects: app.NewListProjects(registered{"nimbus", "shop"}, running{"shop"})}
	root := newRootCommand(weaver)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs(append([]string{"__complete"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("completing %q: %v", args, err)
	}
	var suggestions []string
	for line := range strings.Lines(out.String()) {
		if line = strings.TrimSuffix(line, "\n"); !strings.HasPrefix(line, ":") {
			suggestions = append(suggestions, line) // ":4" is the directive
		}
	}
	return suggestions
}

func TestProjectNameCompletion(t *testing.T) {
	t.Run("weave suggests projects, noting running ones, besides commands", func(t *testing.T) {
		got := complete(t, "")
		for _, want := range []string{"nimbus", "shop\trunning", "list\tList registered projects and whether their session runs"} {
			if !slices.Contains(got, want) {
				t.Fatalf("got %q, want it to contain %q", got, want)
			}
		}
	})

	for _, command := range []string{"kill", "show"} {
		t.Run(command+" suggests projects only", func(t *testing.T) {
			if got, want := complete(t, command, ""), []string{"nimbus", "shop\trunning"}; !slices.Equal(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}

	t.Run("suggests nothing after the project", func(t *testing.T) {
		if got := complete(t, "kill", "shop", ""); len(got) != 0 {
			t.Fatalf("got %q, want no suggestions", got)
		}
	})
}

func TestListCommand(t *testing.T) {
	weaver := application{listProjects: app.NewListProjects(registered{"nimbus", "shop"}, running{"shop"})}
	root := newRootCommand(weaver)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := out.String(), "nimbus  -\nshop        running\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
