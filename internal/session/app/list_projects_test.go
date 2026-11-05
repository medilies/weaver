package app_test

import (
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// runningSessions is a fake domain.SessionLister.
type runningSessions []string

func (r runningSessions) Running() ([]string, error) {
	return r, nil
}

func TestListProjects(t *testing.T) {
	repo := inMemoryRepository{projects: map[string]domain.Project{
		"shop":       shop(t),
		"bicyclette": project(t, "bicyclette"),
	}}

	statuses, err := app.NewListProjects(repo, runningSessions{"shop", "scratch"}).Handle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []string
	for _, s := range statuses {
		state := "stopped"
		if s.Running() {
			state = "running"
		}
		got = append(got, s.Name().String()+" "+state)
	}
	if want := []string{"bicyclette stopped", "shop running"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// project is a project rooted in /srv/<name> with its default window.
func project(t *testing.T, raw string) domain.Project {
	t.Helper()
	name, err := domain.NewProjectName(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root, err := domain.NewProjectRoot("/srv/" + raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return domain.NewProject(name, root)
}
