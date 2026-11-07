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
// killed session shows as not running.
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

// unfindableSessions is a domain.SessionFinder that fails.
type unfindableSessions struct{ err error }

func (s unfindableSessions) Find(domain.ProjectName) (domain.RunningSession, bool, error) {
	return domain.RunningSession{}, false, s.err
}

// browseSetup is blog, not running, and shop, running in a session weaver
// didn't start, with their configs stored as YAML.
type browseSetup struct {
	configs   inMemoryRepository
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
		configs:   inMemoryRepository{projects: map[string]domain.Project{"blog": project(t, "blog"), "shop": shop(t)}},
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
		s.browser, confirm, s.configs, s.documents, s.workspace, server,
		app.NewListProjects(s.configs, server),
		app.NewOpenProject(s.configs, server, s.workspace),
		app.NewFreezeProject(s.configs, capturedSessions{}, s.form, confirm),
		app.NewEditProject(s.documents, s.editor, confirm),
		app.NewKillProject(server, capturedSessions{}),
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
		err := app.NewBrowseProjects(s.browser, answer(true), invalidConfig{s.configs, "shop"}, s.documents, s.workspace, tmuxServer{s.sessions},
			app.NewListProjects(s.configs, tmuxServer{s.sessions}), app.OpenProject{}, app.FreezeProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if !errors.Is(shop.Problem(), domain.ErrInvalidConfig) || shop.Config().File != "shop.yml" {
			t.Fatalf("got problem %v and file %q, want ErrInvalidConfig beside shop.yml", shop.Problem(), shop.Config().File)
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

	t.Run("shows the windows a session weaver started lacks", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = owned(t, "shop")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		var got []string
		for _, name := range shop.MissingWindows() {
			got = append(got, name.String())
		}
		if want := []string{"logs"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("counts the configured windows a session weaver started still has", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = owned(t, "logs")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if present, configured := shop.WindowCount(); present != 1 || configured != 2 {
			t.Fatalf("got %d/%d windows, want 1/2", present, configured)
		}
	})

	t.Run("shows nothing missing from a session weaver started that has every window", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = owned(t, "shop", "logs")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if missing := shop.MissingWindows(); len(missing) > 0 {
			t.Fatalf("got %v missing, want none", missing)
		}
	})

	t.Run("shows no windows missing from a session weaver didn't start", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = domain.NewRunningSession(false)
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if session, ok := shop.Session(); !ok || session.Owned() || len(shop.MissingWindows()) > 0 {
			t.Fatalf("got session %+v (%v) missing %v, want one weaver didn't start, missing nothing",
				session, ok, shop.MissingWindows())
		}
	})

	t.Run("shows no session for a project that isn't running", func(t *testing.T) {
		s := newBrowseSetup(t)
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		blog, _ := s.browser.last().overview("blog")
		if _, ok := blog.Session(); ok {
			t.Fatal("got a session for blog, want none")
		}
	})

	t.Run("keeps the summary when tmux can't describe a running session", func(t *testing.T) {
		s := newBrowseSetup(t)
		err := app.NewBrowseProjects(s.browser, answer(true), s.configs, s.documents, s.workspace,
			unfindableSessions{errors.New("tmux gone")},
			app.NewListProjects(s.configs, tmuxServer{s.sessions}), app.OpenProject{}, app.FreezeProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if _, ok := shop.Session(); ok || shop.Problem() != nil || !shop.RootExists() {
			t.Fatalf("got session %v, problem %v, root existing %v; want no session and the summary intact",
				ok, shop.Problem(), shop.RootExists())
		}
	})

	t.Run("warns about a pane directory that is missing", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.configs.projects["blog"] = serving(t, "docs", "")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		blog, _ := s.browser.last().overview("blog")
		if got := blog.Warnings(); len(got) != 1 || !errors.Is(got[0], app.ErrMissingDirectory) ||
			!strings.Contains(got[0].Error(), "/srv/blog/docs") {
			t.Fatalf("got %v, want /srv/blog/docs missing", got)
		}
	})

	t.Run("doesn't warn about directories under a missing root", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.workspace.dirs, "/srv/blog")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); len(blog.Warnings()) > 0 {
			t.Fatalf("got %v, want the missing root alone", blog.Warnings())
		}
	})

	t.Run("warns about a command whose program can't be found", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.configs.projects["blog"] = serving(t, "", "hugo server")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		blog, _ := s.browser.last().overview("blog")
		if got := blog.Warnings(); len(got) != 1 || !errors.Is(got[0], app.ErrMissingProgram) ||
			!strings.Contains(got[0].Error(), "hugo") {
			t.Fatalf("got %v, want hugo not found", got)
		}
	})

	t.Run("doesn't warn about a program that can be run", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.configs.projects["blog"] = serving(t, "", "hugo server")
		s.workspace.programs = map[string]bool{"hugo": true}
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); len(blog.Warnings()) > 0 {
			t.Fatalf("got %v, want no warnings", blog.Warnings())
		}
	})

	t.Run("doesn't look for programs in panes whose before commands may provide them", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.configs.projects["blog"] = serving(t, "", "npm run dev", "nvm use")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); len(blog.Warnings()) > 0 {
			t.Fatalf("got %v, want no warnings", blog.Warnings())
		}
	})

	t.Run("warns about the windows a session weaver started would get back", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = owned(t, "shop")
		delete(s.workspace.dirs, "/srv/shop/logs")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if got := shop.Warnings(); len(got) != 1 || !strings.Contains(got[0].Error(), "/srv/shop/logs") {
			t.Fatalf("got %v, want /srv/shop/logs missing", got)
		}
	})

	t.Run("doesn't warn about the windows a running session has", func(t *testing.T) {
		s := newBrowseSetup(t)
		s.sessions.running["shop"] = owned(t, "shop", "logs")
		delete(s.workspace.dirs, "/srv/shop/logs")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shop, _ := s.browser.last().overview("shop"); len(shop.Warnings()) > 0 {
			t.Fatalf("got %v, want no warnings", shop.Warnings())
		}
	})

	t.Run("doesn't look for programs in panes that set their PATH", func(t *testing.T) {
		s := newBrowseSetup(t)
		env, err := domain.NewEnv(map[string]string{"PATH": "/opt/hugo/bin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s.configs.projects["blog"] = serving(t, "", "hugo server").WithEnv(env)
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); len(blog.Warnings()) > 0 {
			t.Fatalf("got %v, want no warnings", blog.Warnings())
		}
	})

	t.Run("warns once about a directory several panes share", func(t *testing.T) {
		s := newBrowseSetup(t)
		dir, err := domain.NewPaneDir("docs")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var windows []domain.Window
		for _, raw := range []string{"one", "two"} {
			name, err := domain.NewWindowName(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			window, err := domain.NewWindow(name, domain.NewPane().In(dir))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			windows = append(windows, window)
		}
		blog, err := project(t, "blog").WithWindows(windows...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s.configs.projects["blog"] = blog
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blog, _ := s.browser.last().overview("blog"); len(blog.Warnings()) != 1 {
			t.Fatalf("got %v, want one warning", blog.Warnings())
		}
	})

	t.Run("doesn't warn about a session weaver didn't start", func(t *testing.T) {
		s := newBrowseSetup(t)
		delete(s.workspace.dirs, "/srv/shop/logs")
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shop, _ := s.browser.last().overview("shop"); len(shop.Warnings()) > 0 {
			t.Fatalf("got %v, want no warnings", shop.Warnings())
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

	t.Run("kills a session once confirmed, then shows it not running", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shop, _ := s.browser.last().overview("shop")
		if shop.Running() || s.browser.last().notice != "Killed shop." {
			t.Fatalf("got running=%v and notice %q, want shop killed", shop.Running(), s.browser.last().notice)
		}
	})

	t.Run("says a session isn't running without asking to kill it", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "blog"))
		confirm := &countingConfirmer{answer: true}
		if err := s.browse(confirm); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notice := s.browser.last().notice; !strings.Contains(notice, domain.ErrSessionNotRunning.Error()) || confirm.asked != 0 {
			t.Fatalf("got notice %q after %d questions, want blog's session not running and none", notice, confirm.asked)
		}
	})

	t.Run("still asks to kill a session it couldn't find", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionKill, "shop"))
		confirm := &countingConfirmer{answer: true}
		server := tmuxServer{s.sessions}
		err := app.NewBrowseProjects(
			s.browser, confirm, s.configs, s.documents, s.workspace, unfindableSessions{errors.New("tmux: not found")},
			app.NewListProjects(s.configs, server), app.OpenProject{}, app.FreezeProject{}, app.EditProject{},
			app.NewKillProject(server, capturedSessions{}),
		).Handle("/srv/new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if confirm.asked != 1 || s.browser.last().notice != "Killed shop." {
			t.Fatalf("got notice %q after %d questions, want shop killed once confirmed", s.browser.last().notice, confirm.asked)
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

	t.Run("roots a frozen project at the working directory", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionFreeze, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/new" {
			t.Fatalf("got root %q, want /srv/new", s.form.draft.Root)
		}
	})

	t.Run("selects a frozen project", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionFreeze, "shop"))
		if err := s.browse(answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.browser.last().selected.String(); got != "new" {
			t.Fatalf("got %q selected, want new", got)
		}
	})

	t.Run("comes back to the highlighted project when freezing is abandoned", func(t *testing.T) {
		s := newBrowseSetup(t, choose(t, app.ActionFreeze, "shop"))
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
		err := app.NewBrowseProjects(failingBrowser{failing}, answer(true), s.configs, s.documents, s.workspace, tmuxServer{s.sessions},
			app.NewListProjects(s.configs, tmuxServer{s.sessions}), app.OpenProject{}, app.FreezeProject{}, app.EditProject{}, app.KillProject{},
		).Handle("/srv/new")
		if !errors.Is(err, failing) {
			t.Fatalf("got %v, want the browser's error", err)
		}
	})
}

// serving is blog with one pane, in dir, running startup (none when
// blank) after the before commands.
func serving(t *testing.T, dir, startup string, before ...string) domain.Project {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("building the blog project: %v", err)
		}
	}
	paneDir, err := domain.NewPaneDir(dir)
	must(err)
	pane := domain.NewPane().In(paneDir)
	if startup != "" {
		cmd, err := domain.NewCommand(startup)
		must(err)
		pane = pane.Running(cmd)
	}
	var cmds []domain.Command
	for _, raw := range before {
		cmd, err := domain.NewCommand(raw)
		must(err)
		cmds = append(cmds, cmd)
	}
	name, err := domain.NewWindowName("blog")
	must(err)
	window, err := domain.NewWindow(name, pane.RunningFirst(cmds...))
	must(err)
	blog, err := project(t, "blog").WithWindows(window)
	must(err)
	return blog
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
