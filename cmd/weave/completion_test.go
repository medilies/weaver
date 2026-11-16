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

// registered is a fake domain.ProjectRegistry: projects by name, each
// rooted at /srv/ and its name, with their default window.
type registered []string

func (r registered) Find(root domain.ProjectRoot) (domain.Project, error) {
	for _, ref := range r.refs() {
		if ref.Root() == root {
			return domain.NewProject(ref.Name(), root), nil
		}
	}
	return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, root)
}

func (r registered) Save(domain.Project, string) error {
	return nil
}

func (r registered) Delete(domain.ProjectRoot) error {
	return nil
}

func (r registered) Registered() ([]domain.ProjectRef, error) {
	return r.refs(), nil
}

func (r registered) refs() []domain.ProjectRef {
	var refs []domain.ProjectRef
	for _, raw := range r {
		name, _ := domain.NewProjectName(raw)
		root, _ := domain.NewProjectRoot("/srv/" + raw)
		refs = append(refs, domain.NewProjectRef(root, name))
	}
	return refs
}

// running is a fake domain.SessionLister: the sessions of projects by
// name, as if started by hand in their roots.
type running []string

func (r running) Running() ([]domain.ListedSession, error) {
	var sessions []domain.ListedSession
	for _, name := range r {
		sessions = append(sessions, domain.NewListedSession("w/"+name, "/srv/"+name))
	}
	return sessions, nil
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
		for _, want := range []string{"nimbus", "shop\trunning", "kill\tKill a project's tmux session and everything running in it"} {
			if !slices.Contains(got, want) {
				t.Fatalf("got %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("kill suggests projects only", func(t *testing.T) {
		if got, want := complete(t, "kill", ""), []string{"shop\trunning", "nimbus"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("suggests nothing after the project", func(t *testing.T) {
		if got := complete(t, "kill", "shop", ""); len(got) != 0 {
			t.Fatalf("got %q, want no suggestions", got)
		}
	})
}
