package app_test

import (
	"slices"
	"testing"
	"time"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// runningSessions is a fake domain.SessionLister.
type runningSessions []domain.ListedSession

func (r runningSessions) Running() ([]domain.ListedSession, error) {
	return r, nil
}

func TestListProjects(t *testing.T) {
	configs := registry(project(t, "nimbus"), shop(t), project(t, "blog"), project(t, "api"))
	at := func(minute int) time.Time { return time.Date(2026, 11, 16, 9, minute, 0, 0, time.UTC) }
	sessions := runningSessions{
		domain.NewListedSession("w/shop", "/srv/shop").ActiveAt(at(1)),
		domain.NewListedSession("w/blog", "/tmp").StartedFor("/srv/blog").ActiveAt(at(5)),
		domain.NewListedSession("w/api", "/srv/elsewhere").ActiveAt(at(9)),
		domain.NewListedSession("scratch", "/srv/nimbus"),
	}

	statuses, err := app.NewListProjects(configs, sessions).Handle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []string
	for _, s := range statuses {
		state := "not running"
		if s.Running() {
			state = "running"
		}
		got = append(got, s.Name().String()+" "+state)
	}
	// Running first, the last attached at the top, then the settings' order.
	if want := []string{"blog running", "shop running", "nimbus not running", "api not running"}; !slices.Equal(got, want) {
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
