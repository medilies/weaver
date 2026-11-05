package app_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeSessions is an in-memory domain.SessionLauncher recording what it
// was asked to do.
type fakeSessions struct {
	running  map[string]bool
	started  []string
	attached []string
}

func (s *fakeSessions) IsRunning(name domain.ProjectName) (bool, error) {
	return s.running[name.String()], nil
}

func (s *fakeSessions) Start(plan domain.SessionPlan) error {
	s.started = append(s.started, plan.Name().String())
	return nil
}

func (s *fakeSessions) Attach(name domain.ProjectName) error {
	s.attached = append(s.attached, name.String())
	return nil
}

// fakeWorkspace is an in-memory domain.Workspace: a set of existing
// directories, which cloning adds to.
type fakeWorkspace struct {
	dirs   map[string]bool
	cloned []string
}

func (w *fakeWorkspace) Exists(dir string) (bool, error) {
	return w.dirs[dir], nil
}

func (w *fakeWorkspace) Clone(_ domain.Checkout, dir string) error {
	w.cloned = append(w.cloned, dir)
	w.dirs[dir] = true
	return nil
}

// fakeRuntimes is a fake domain.RuntimeResolver: "kind version" → dir.
type fakeRuntimes map[string]string

func (r fakeRuntimes) Resolve(runtime domain.Runtime) (string, error) {
	if dir, ok := r[runtime.String()]; ok {
		return dir, nil
	}
	return "", fmt.Errorf("%w: %s", domain.ErrRuntimeNotInstalled, runtime)
}

// shop is a project rooted at /srv/shop with a back and a front
// repository, each opened in a pane of the first window, and a logs
// window in a plain directory. The back pane pins PHP.
func shop(t *testing.T) domain.Project {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("building the shop project: %v", err)
		}
	}
	name, err := domain.NewProjectName("shop")
	must(err)
	root, err := domain.NewProjectRoot("/srv/shop")
	must(err)
	backRepo, err := domain.NewCheckout("back", "git@host:back.git")
	must(err)
	frontRepo, err := domain.NewCheckout("front", "git@host:front.git")
	must(err)
	checkouts, err := domain.NewCheckouts(backRepo, frontRepo)
	must(err)
	php, err := domain.NewRuntime("php", "8.4")
	must(err)
	pins, err := domain.NewRuntimes(php)
	must(err)
	backName, err := domain.NewPaneName("back")
	must(err)
	backDir, err := domain.NewPaneDir("back")
	must(err)
	frontName, err := domain.NewPaneName("front")
	must(err)
	frontDir, err := domain.NewPaneDir("front")
	must(err)
	right, err := domain.NewSplit(0, domain.Right, domain.DefaultSize())
	must(err)
	windowName, err := domain.NewWindowName("shop")
	must(err)
	window, err := domain.NewWindow(windowName,
		domain.NewPane().Named(backName).In(backDir).WithRuntimes(pins),
		domain.NewPane().Named(frontName).In(frontDir).Splitting(right),
	)
	must(err)
	logsName, err := domain.NewWindowName("logs")
	must(err)
	logsDir, err := domain.NewPaneDir("logs")
	must(err)
	logs, err := domain.NewWindow(logsName, domain.NewPane().In(logsDir))
	must(err)
	project, err := domain.NewProject(name, root).WithCheckouts(checkouts).WithWindows(window, logs)
	must(err)
	return project
}

// openShop runs OpenProject on the shop project with the given state.
func openShop(t *testing.T, sessions *fakeSessions, workspace *fakeWorkspace, runtimes fakeRuntimes) error {
	t.Helper()
	repo := inMemoryRepository{projects: map[string]domain.Project{"shop": shop(t)}}
	return app.NewOpenProject(repo, sessions, workspace, runtimes).Handle("shop")
}

func everything() *fakeWorkspace {
	return &fakeWorkspace{dirs: map[string]bool{
		"/srv/shop": true, "/srv/shop/back": true, "/srv/shop/front": true, "/srv/shop/logs": true, "/srv/blog": true,
	}}
}

var php84 = fakeRuntimes{"php 8.4": "/cache/php/8.4"}

func TestOpenProject(t *testing.T) {
	t.Run("starts the session when it isn't running, then attaches", func(t *testing.T) {
		sessions := &fakeSessions{}
		if err := openShop(t, sessions, everything(), php84); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.started, []string{"shop"}) || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got started %q, attached %q; want shop started then attached", sessions.started, sessions.attached)
		}
	})

	t.Run("only attaches when the session is running", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]bool{"shop": true}}
		if err := openShop(t, sessions, everything(), php84); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions.started) != 0 || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got started %q, attached %q; want shop attached only", sessions.started, sessions.attached)
		}
	})

	t.Run("clones missing repositories before starting", func(t *testing.T) {
		workspace := &fakeWorkspace{dirs: map[string]bool{"/srv/shop": true, "/srv/shop/logs": true}}
		if err := openShop(t, &fakeSessions{}, workspace, php84); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"/srv/shop/back", "/srv/shop/front"}; !slices.Equal(workspace.cloned, want) {
			t.Fatalf("got %q cloned, want %q", workspace.cloned, want)
		}
	})

	t.Run("leaves existing repositories alone", func(t *testing.T) {
		workspace := &fakeWorkspace{dirs: map[string]bool{"/srv/shop": true, "/srv/shop/back": true, "/srv/shop/logs": true}}
		if err := openShop(t, &fakeSessions{}, workspace, php84); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"/srv/shop/front"}; !slices.Equal(workspace.cloned, want) {
			t.Fatalf("got %q cloned, want %q", workspace.cloned, want)
		}
	})

	t.Run("fails on a missing version before cloning or starting", func(t *testing.T) {
		sessions, workspace := &fakeSessions{}, &fakeWorkspace{dirs: map[string]bool{}}
		err := openShop(t, sessions, workspace, fakeRuntimes{})
		if !errors.Is(err, domain.ErrRuntimeNotInstalled) {
			t.Fatalf("got %v, want ErrRuntimeNotInstalled", err)
		}
		if len(workspace.cloned) != 0 || len(sessions.started) != 0 {
			t.Fatalf("got %q cloned and %q started, want nothing changed", workspace.cloned, sessions.started)
		}
	})

	t.Run("fails on a missing pane directory before starting", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		err := openShop(t, sessions, workspace, php84)
		if !errors.Is(err, domain.ErrMissingDirectory) {
			t.Fatalf("got %v, want ErrMissingDirectory", err)
		}
		if len(sessions.started) != 0 || len(sessions.attached) != 0 {
			t.Fatalf("got %q started and %q attached, want neither", sessions.started, sessions.attached)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		repo := inMemoryRepository{projects: map[string]domain.Project{}}
		err := app.NewOpenProject(repo, &fakeSessions{}, everything(), php84).Handle("shop")
		if _, ok := errors.AsType[domain.ProjectNotFound](err); !ok {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})
}

// rootedAt is a project with its default window, rooted at root.
func rootedAt(t *testing.T, raw, root string) domain.Project {
	t.Helper()
	name, err := domain.NewProjectName(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	projectRoot, err := domain.NewProjectRoot(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return domain.NewProject(name, projectRoot)
}

func TestOpenProjectHandleDir(t *testing.T) {
	open := func(t *testing.T, sessions *fakeSessions, dir string, projects ...domain.Project) error {
		t.Helper()
		repo := inMemoryRepository{projects: map[string]domain.Project{}}
		for _, p := range projects {
			repo.projects[p.Name().String()] = p
		}
		return app.NewOpenProject(repo, sessions, everything(), php84).HandleDir(dir)
	}

	t.Run("opens the project whose root contains the directory", func(t *testing.T) {
		sessions := &fakeSessions{}
		if err := open(t, sessions, "/srv/shop/front/src", shop(t), rootedAt(t, "blog", "/srv/blog")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got %q attached, want shop", sessions.attached)
		}
	})

	t.Run("prefers the innermost root when roots nest", func(t *testing.T) {
		sessions := &fakeSessions{}
		if err := open(t, sessions, "/srv/shop/back/app", shop(t), rootedAt(t, "back", "/srv/shop/back")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"back"}) {
			t.Fatalf("got %q attached, want back", sessions.attached)
		}
	})

	t.Run("reports a directory no project contains", func(t *testing.T) {
		err := open(t, &fakeSessions{}, "/home/me", shop(t))
		if !errors.Is(err, domain.ErrNoProjectAt) {
			t.Fatalf("got %v, want ErrNoProjectAt", err)
		}
	})

	t.Run("refuses to guess between projects sharing a root", func(t *testing.T) {
		sessions := &fakeSessions{}
		err := open(t, sessions, "/srv/blog", rootedAt(t, "blog", "/srv/blog"), rootedAt(t, "notes", "/srv/blog"))
		if !errors.Is(err, domain.ErrAmbiguousProject) || len(sessions.attached) != 0 {
			t.Fatalf("got %v with %q attached, want ErrAmbiguousProject and nothing attached", err, sessions.attached)
		}
	})
}
