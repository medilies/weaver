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
		s.browser, confirm, s.repo, s.documents, s.workspace,
		app.NewListProjects(s.repo, server),
		app.NewOpenProject(s.repo, server, s.workspace),
		app.NewAddProject(s.repo, foundCheckouts{}, capturedSessions{}, s.form, confirm),
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
	t.Run("shows each project's config", func(t *testing.T) {
		s := newBrowseSetup(t)
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if got := shop.Config(); got.File != "shop.yml" || string(got.Content) != "root: /srv/shop\n" {
			t.Fatalf("got %q %q, want shop.yml as stored", got.File, got.Content)
		}
	})

	t.Run("shows why a project's config can't be read", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.documents, "blog")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); !isProjectNotFound(blog.Problem()) {
			t.Fatalf("got problem %v, want ErrProjectNotFound", blog.Problem())
		}
	})

	t.Run("shows why a readable config isn't valid, beside the config", func(t *testing.T) {
		s := newBrowseSetup(t)
		err := app.NewBrowseProjects(s.browser, answer(true), invalidConfig{s.repo, "shop"}, s.documents, s.workspace,
			app.NewListProjects(s.repo, tmuxServer{s.sessions}), app.OpenProject{}, app.AddProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if !errors.Is(shop.Problem(), domain.ErrInvalidConfig) || shop.Config().File != "shop.yml" {
			t.Fatalf("got problem %v and file %q, want ErrInvalidConfig beside shop.yml", shop.Problem(), shop.Config().File)
		}
	})

	t.Run("shows whether each of a project's repositories is cloned", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.workspace.dirs, "/srv/shop/front")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		var got []string
		for _, status := range shop.Checkouts() {
			got = append(got, fmt.Sprintf("%s %v", status.Checkout().Dir(), status.Cloned()))
		}
		if want := []string{"back true", "front false"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows when a project's root is gone", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.workspace.dirs, "/srv/blog")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); blog.RootExists() {
			t.Fatal("got blog's root existing, want it gone")
		}
	})

	t.Run("shows the working tree a project's root is in", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.workspace.trees = map[string]domain.WorkingTree{"/srv/shop": domain.OnBranch("main").WithChanges()}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if tree, ok := shop.WorkingTree(); !ok || tree.Branch() != "main" || !tree.Dirty() {
			t.Fatalf("got %+v (%v), want main with changes", tree, ok)
		}
	})

	t.Run("shows no working tree for a root outside git", func(t *testing.T) {
		s := newBrowseSetup(t)
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		blog, _ := s.browser.last().overview("blog")
		if _, ok := blog.WorkingTree(); ok {
			t.Fatal("got a working tree for blog, want none")
		}
	})

	t.Run("ends without error when the user quits", func(t *testing.T) {
		s := newBrowseSetup(t)
		if err := s.browse(answer(true)); err != nil || len(s.browser.shown) != 1 {
			t.Fatalf("got %v after %d views, want nil after one", err, len(s.browser.shown))
		}
	})

	t.Run("opens the chosen project", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionOpen, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(s.sessions.attached, []string{"shop"}) {
			t.Fatalf("got %q attached, want shop", s.sessions.attached)
		}
	})

	t.Run("stops browsing once a project opens", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionOpen, "shop"), choose(t, app.ActionOpen, "blog"))
		if err := s.browse(answer(true)); err != nil || len(s.browser.shown) != 1 {
			t.Fatalf("got %v after %d views, want nil after one", err, len(s.browser.shown))
		}
	})

	t.Run("shows why a project didn't open and keeps browsing", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionOpen, "blog"))
		s.workspace = &fakeWorkspace{dirs: map[string]bool{}}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; !strings.Contains(notice, app.ErrMissingDirectory.Error()) {
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

	t.Run("shows why a session couldn't be stopped", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "blog"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; !strings.Contains(notice, domain.ErrSessionNotRunning.Error()) {
			t.Fatalf("got notice %q, want blog's session not running", notice)
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

	t.Run("roots an added project at the working directory", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionAdd, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/new" {
			t.Fatalf("got root %q, want /srv/new", s.form.draft.Root)
		}
	})

	t.Run("selects an added project", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionAdd, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.browser.last().selected.String(); got != "new" {
			t.Fatalf("got %q selected, want new", got)
		}
	})

	t.Run("comes back to the highlighted project when adding is abandoned", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionAdd, "shop"))
		s.form.err = app.ErrAborted
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if last := s.browser.last(); last.selected.String() != "shop" || last.notice != "Nothing done." {
			t.Fatalf("got %q selected and notice %q, want shop and nothing done", last.selected, last.notice)
		}
	})

	t.Run("says when an edit saved the config", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionEdit, "shop"))
		s.editor.saves = []string{"root: /srv/store\n"}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; notice != "Saved shop." {
			t.Fatalf("got notice %q, want %q", notice, "Saved shop.")
		}
	})

	t.Run("says when an edit changed nothing", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionEdit, "shop"))
		s.editor.saves = []string{"root: /srv/shop\n"}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; notice != "No changes." {
			t.Fatalf("got notice %q, want %q", notice, "No changes.")
		}
	})

	t.Run("reports a browser that fails", func(t *testing.T) {
		s := newBrowseSetup(t)
		failing := errors.New("no terminal")
		err := app.NewBrowseProjects(failingBrowser{failing}, answer(true), s.repo, s.documents, s.workspace,
			app.NewListProjects(s.repo, tmuxServer{s.sessions}), app.OpenProject{}, app.AddProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if !errors.Is(err, failing) {
			t.Fatalf("got %v, want the browser's error", err)
		}
	})
}

func isProjectNotFound(err error) bool {
	return errors.Is(err, domain.ErrProjectNotFound)
}

// failingBrowser is an app.ProjectBrowser that can't show anything.
type failingBrowser struct{ err error }

func (b failingBrowser) Browse([]app.ProjectOverview, domain.ProjectName, string) (app.Choice, error) {
	return app.Choice{}, b.err
}

// invalidConfig is a repository whose config for the project named
// invalid can't be loaded.
type invalidConfig struct {
	inMemoryRepository
	invalid string
}

func (r invalidConfig) FindByName(name domain.ProjectName) (domain.Project, error) {
	if name.String() == r.invalid {
		return domain.Project{}, fmt.Errorf("%s: %w", name, domain.ErrInvalidConfig)
	}
	return r.inMemoryRepository.FindByName(name)
}
