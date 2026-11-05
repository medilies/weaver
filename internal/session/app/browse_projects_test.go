package app_test

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// scriptedBrowser is an app.ProjectBrowser making the next scripted choice
// on each call, then quitting, and recording what each call showed.
type scriptedBrowser struct {
	choices []app.Choice
	shown   []browserView
}

// browserView is what one call to Browse showed.
type browserView struct {
	projects []app.ProjectOverview
	selected domain.ProjectName
	notice   string
}

func (b *scriptedBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice string) (app.Choice, error) {
	b.shown = append(b.shown, browserView{projects: projects, selected: selected, notice: notice})
	if len(b.choices) == 0 {
		return app.Choice{}, app.ErrAborted
	}
	choice := b.choices[0]
	b.choices = b.choices[1:]
	return choice, nil
}

// last is what the browser showed after the scripted choices.
func (b *scriptedBrowser) last() browserView {
	return b.shown[len(b.shown)-1]
}

// overview is the project named name in v, if shown.
func (v browserView) overview(name string) (app.ProjectOverview, bool) {
	for _, overview := range v.projects {
		if overview.Name().String() == name {
			return overview, true
		}
	}
	return app.ProjectOverview{}, false
}

// tmuxServer is fakeSessions that also lists and kills its sessions, so a
// stopped session shows as stopped.
type tmuxServer struct{ *fakeSessions }

func (s tmuxServer) Running() ([]string, error) {
	return slices.Sorted(maps.Keys(s.running)), nil
}

func (s tmuxServer) Kill(name domain.ProjectName) error {
	if _, ok := s.running[name.String()]; !ok {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	delete(s.running, name.String())
	return nil
}

// browseSetup is blog, stopped, and shop, running in a session weaver
// didn't start, with their configs stored as YAML.
type browseSetup struct {
	repo      inMemoryRepository
	documents storedDocuments
	sessions  *fakeSessions
	workspace *fakeWorkspace
	form      *fakeForm
	editor    *scriptedEditor
	browser   *scriptedBrowser
}

func newBrowseSetup(t *testing.T, choices ...app.Choice) browseSetup {
	t.Helper()
	return browseSetup{
		repo:      inMemoryRepository{projects: map[string]domain.Project{"blog": project(t, "blog"), "shop": shop(t)}},
		documents: storedDocuments{"blog": "root: /srv/blog\n", "shop": "root: /srv/shop\n"},
		sessions:  &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}},
		workspace: everything(),
		form:      &fakeForm{project: project(t, "new")},
		editor:    &scriptedEditor{},
		browser:   &scriptedBrowser{choices: choices},
	}
}

// browse runs the browser from /srv/new until the scripted choices are
// made, confirming with confirm.
func (s browseSetup) browse(confirm app.Confirmer) error {
	server := tmuxServer{s.sessions}
	return app.NewBrowseProjects(
		s.browser, confirm, s.repo, s.documents,
		app.NewListProjects(s.repo, server),
		app.NewOpenProject(s.repo, server, s.workspace, fakeRuntimes{}),
		app.NewAddProject(s.repo, foundCheckouts{}, installedRuntimes{}, capturedSessions{}, s.form, confirm),
		app.NewEditProject(s.documents, s.editor, confirm),
		app.NewKillProject(server),
	).Handle("/srv/new")
}

func choose(t *testing.T, action app.Action, raw string) app.Choice {
	t.Helper()
	name, err := domain.NewProjectName(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return app.Choice{Action: action, Project: name}
}

func TestBrowseProjects(t *testing.T) {
	t.Run("shows each project's config, or why it can't be read", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.documents, "blog")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		blog, _ := s.browser.last().overview("blog")
		if shop.Config().File != "shop.yml" || string(shop.Config().Content) != "root: /srv/shop\n" || blog.Problem() == nil {
			t.Fatalf("got shop %+v and blog problem %v, want shop's config and blog's problem", shop.Config(), blog.Problem())
		}
	})

	t.Run("ends without error when the user quits", func(t *testing.T) {
		s := newBrowseSetup(t)
		if err := s.browse(answer(true)); err != nil || len(s.browser.shown) != 1 {
			t.Fatalf("got %v after %d views, want nil after one", err, len(s.browser.shown))
		}
	})

	t.Run("opens the chosen project and stops browsing", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionOpen, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(s.sessions.attached, []string{"shop"}) || len(s.browser.shown) != 1 {
			t.Fatalf("got %q attached after %d views, want shop after one", s.sessions.attached, len(s.browser.shown))
		}
	})

	t.Run("shows why a project didn't open and keeps browsing", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionOpen, "blog"))
		s.workspace = &fakeWorkspace{dirs: map[string]bool{}}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; !strings.Contains(notice, domain.ErrMissingDirectory.Error()) {
			t.Fatalf("got notice %q, want the missing directory", notice)
		}
	})

	t.Run("stops a session once confirmed, then shows it stopped", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if shop.Running() || s.browser.last().notice != "Stopped shop." {
			t.Fatalf("got running=%v and notice %q, want shop stopped", shop.Running(), s.browser.last().notice)
		}
	})

	t.Run("keeps a session running when the user declines", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "shop"))
		if err := s.browse(answer(false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if !shop.Running() || s.browser.last().notice != "Nothing done." {
			t.Fatalf("got running=%v and notice %q, want shop still running", shop.Running(), s.browser.last().notice)
		}
	})

	t.Run("deletes a project once confirmed", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionDelete, "blog"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, shown := s.browser.last().overview("blog"); shown || s.browser.last().notice != "Deleted blog." {
			t.Fatalf("got blog shown=%v and notice %q, want blog gone", shown, s.browser.last().notice)
		}
	})

	t.Run("keeps a project the user declines to delete", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionDelete, "blog"))
		if err := s.browse(answer(false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, shown := s.browser.last().overview("blog"); !shown {
			t.Fatal("got blog deleted, want it kept")
		}
	})

	t.Run("adds a project rooted at the working directory, then selects it", func(t *testing.T) {
		s := newBrowseSetup(t, app.Choice{Action: app.ActionAdd})
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/new" || s.browser.last().selected.String() != "new" {
			t.Fatalf("got root %q and %q selected, want /srv/new and new", s.form.draft.Root, s.browser.last().selected)
		}
	})

	t.Run("tells what an edit did", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionEdit, "shop"))
		s.editor.saves = []string{"root: /srv/store\n"}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; notice != "Saved shop." {
			t.Fatalf("got notice %q, want %q", notice, "Saved shop.")
		}
	})

	t.Run("reports a browser that fails", func(t *testing.T) {
		s := newBrowseSetup(t)
		failing := errors.New("no terminal")
		err := app.NewBrowseProjects(failingBrowser{failing}, answer(true), s.repo, s.documents,
			app.NewListProjects(s.repo, tmuxServer{s.sessions}), app.OpenProject{}, app.AddProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if !errors.Is(err, failing) {
			t.Fatalf("got %v, want the browser's error", err)
		}
	})
}

// failingBrowser is an app.ProjectBrowser that can't show anything.
type failingBrowser struct{ err error }

func (b failingBrowser) Browse([]app.ProjectOverview, domain.ProjectName, string) (app.Choice, error) {
	return app.Choice{}, b.err
}
