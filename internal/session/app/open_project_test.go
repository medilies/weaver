package app_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeSessions is an in-memory domain.SessionLauncher recording what it
// was asked to do.
type fakeSessions struct {
	running  map[string]domain.RunningSession
	started  []string
	restored []string
	attached []string
}

func (s *fakeSessions) Find(name domain.ProjectName) (domain.RunningSession, bool, error) {
	session, ok := s.running[name.String()]
	return session, ok, nil
}

func (s *fakeSessions) Restore(_ domain.ProjectName, placements []domain.WindowPlacement) error {
	for _, placement := range placements {
		s.restored = append(s.restored, placement.Window().Name().String())
	}
	return nil
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
// directories, and the programs that can be run.
type fakeWorkspace struct {
	dirs     map[string]bool
	programs map[string]bool
	links    map[string]string // paths reached through a symlink, and where they lead
}

func (w *fakeWorkspace) RealPath(path string) string {
	if target, ok := w.links[path]; ok {
		return target
	}
	return path
}

func (w *fakeWorkspace) Executable(program, _ string) bool {
	return w.programs[program]
}

func (w *fakeWorkspace) Exists(dir string) (bool, error) {
	return w.dirs[dir], nil
}

// shop is a project rooted at /srv/shop with a back and a front
// directory, each opened in a pane of the first window, and a logs
// window.
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
		domain.NewPane().Named(backName).In(backDir),
		domain.NewPane().Named(frontName).In(frontDir).Splitting(right),
	)
	must(err)
	logsName, err := domain.NewWindowName("logs")
	must(err)
	logsDir, err := domain.NewPaneDir("logs")
	must(err)
	logs, err := domain.NewWindow(logsName, domain.NewPane().In(logsDir))
	must(err)
	project, err := domain.NewProject(name, root).WithWindows(window, logs)
	must(err)
	return project
}

// openShop runs OpenProject on the shop project with the given state.
func openShop(t *testing.T, sessions *fakeSessions, workspace *fakeWorkspace) error {
	t.Helper()
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t)}}
	return app.NewOpenProject(configs, sessions, workspace).Handle("shop")
}

// owned is a running session weaver started, which still has windows.
func owned(t *testing.T, windows ...string) domain.RunningSession {
	t.Helper()
	names := make([]domain.WindowName, 0, len(windows))
	for _, raw := range windows {
		name, err := domain.NewWindowName(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		names = append(names, name)
	}
	return domain.NewRunningSession(true, names...)
}

func everything() *fakeWorkspace {
	return &fakeWorkspace{dirs: map[string]bool{
		"/srv/shop": true, "/srv/shop/back": true, "/srv/shop/front": true, "/srv/shop/logs": true, "/srv/blog": true,
	}}
}

func TestOpenProject(t *testing.T) {
	t.Run("starts the session when it isn't running, then attaches", func(t *testing.T) {
		sessions := &fakeSessions{}
		if err := openShop(t, sessions, everything()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.started, []string{"shop"}) || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got started %q, attached %q; want shop started then attached", sessions.started, sessions.attached)
		}
	})

	t.Run("only attaches when weaver's session has every window", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop", "logs")}}
		if err := openShop(t, sessions, everything()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions.started)+len(sessions.restored) != 0 || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got started %q, restored %q, attached %q; want shop attached only",
				sessions.started, sessions.restored, sessions.attached)
		}
	})

	t.Run("re-creates the windows weaver's session lacks, then attaches", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop")}}
		if err := openShop(t, sessions, everything()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.restored, []string{"logs"}) || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got restored %q, attached %q; want logs restored, then shop attached", sessions.restored, sessions.attached)
		}
	})

	t.Run("leaves a session weaver didn't start as it is", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}
		if err := openShop(t, sessions, everything()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions.restored) != 0 || !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got restored %q, attached %q; want shop attached only", sessions.restored, sessions.attached)
		}
	})

	t.Run("fails on a missing directory before restoring a window", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop")}}
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		err := openShop(t, sessions, workspace)
		if !errors.Is(err, app.ErrMissingDirectory) || len(sessions.restored)+len(sessions.attached) != 0 {
			t.Fatalf("got %v, restored %q, attached %q; want ErrMissingDirectory and nothing done",
				err, sessions.restored, sessions.attached)
		}
	})

	t.Run("fails on a missing pane directory before starting", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		err := openShop(t, sessions, workspace)
		if !errors.Is(err, app.ErrMissingDirectory) {
			t.Fatalf("got %v, want ErrMissingDirectory", err)
		}
		if len(sessions.started) != 0 || len(sessions.attached) != 0 {
			t.Fatalf("got %q started and %q attached, want neither", sessions.started, sessions.attached)
		}
	})

	t.Run("says how to fix a missing directory", func(t *testing.T) {
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		err := openShop(t, &fakeSessions{}, workspace)
		if err == nil || !strings.HasSuffix(err.Error(), "/srv/shop/logs (create it, or change it with weave edit shop)") {
			t.Fatalf("got %v, want a hint to create it or edit shop", err)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		configs := inMemoryRegistry{projects: map[string]domain.Project{}}
		err := app.NewOpenProject(configs, &fakeSessions{}, everything()).Handle("shop")
		if !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
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

// withBrokenConfig is a registry that also lists the project "broken",
// whose stored config can't be read.
type withBrokenConfig struct {
	inMemoryRegistry
}

func (r withBrokenConfig) Names() ([]domain.ProjectName, error) {
	names, err := r.inMemoryRegistry.Names()
	broken, _ := domain.NewProjectName("broken")
	return append([]domain.ProjectName{broken}, names...), err
}

func (r withBrokenConfig) FindByName(name domain.ProjectName) (domain.Project, error) {
	if name.String() == "broken" {
		return domain.Project{}, fmt.Errorf("broken.yml: %w: root: missing", domain.ErrInvalidConfig)
	}
	return r.inMemoryRegistry.FindByName(name)
}

func TestOpenProjectHandleDir(t *testing.T) {
	registry := func(projects ...domain.Project) inMemoryRegistry {
		configs := inMemoryRegistry{projects: map[string]domain.Project{}}
		for _, p := range projects {
			configs.projects[p.Name().String()] = p
		}
		return configs
	}
	open := func(t *testing.T, sessions *fakeSessions, dir string, projects ...domain.Project) error {
		t.Helper()
		return app.NewOpenProject(registry(projects...), sessions, everything()).HandleDir(dir)
	}
	openPastBroken := func(t *testing.T, sessions *fakeSessions, dir string, projects ...domain.Project) error {
		t.Helper()
		configs := withBrokenConfig{registry(projects...)}
		return app.NewOpenProject(configs, sessions, everything()).HandleDir(dir)
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
		// shop-back sorts after shop, so the first root found isn't it.
		if err := open(t, sessions, "/srv/shop/back/app", shop(t), rootedAt(t, "shop-back", "/srv/shop/back")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"shop-back"}) {
			t.Fatalf("got %q attached, want shop-back", sessions.attached)
		}
	})

	t.Run("opens the project whose root a symlinked directory leads into", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		workspace.links = map[string]string{"/home/me/shop/front": "/srv/shop/front"}
		if err := app.NewOpenProject(registry(shop(t)), sessions, workspace).HandleDir("/home/me/shop/front"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got %q attached, want shop", sessions.attached)
		}
	})

	t.Run("opens the project whose root is written through a symlink", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		workspace.links = map[string]string{"/home/me/blog": "/srv/blog"}
		workspace.dirs["/home/me/blog"] = true
		if err := app.NewOpenProject(registry(rootedAt(t, "blog", "/home/me/blog")), sessions, workspace).HandleDir("/srv/blog/posts"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"blog"}) {
			t.Fatalf("got %q attached, want blog", sessions.attached)
		}
	})

	t.Run("prefers the innermost root when only one is written through a symlink", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		workspace.links = map[string]string{"/home/me/projects/shop": "/srv/shop", "/home/me/projects/shop/front": "/srv/shop/front"}
		workspace.dirs["/home/me/projects/shop"] = true
		configs := registry(rootedAt(t, "outer", "/home/me/projects/shop"), rootedAt(t, "inner", "/srv/shop/front"))
		if err := app.NewOpenProject(configs, sessions, workspace).HandleDir("/home/me/projects/shop/front"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"inner"}) {
			t.Fatalf("got %q attached, want inner", sessions.attached)
		}
	})

	t.Run("opens the project a symlink inside it leads out of", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		workspace.links = map[string]string{"/srv/shop/vendor": "/opt/vendor"}
		if err := app.NewOpenProject(registry(shop(t)), sessions, workspace).HandleDir("/srv/shop/vendor"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got %q attached, want shop", sessions.attached)
		}
	})

	t.Run("opens the project a symlink leads into over the one it is in", func(t *testing.T) {
		sessions := &fakeSessions{}
		workspace := everything()
		workspace.links = map[string]string{"/srv/shop/data": "/srv/big/data"}
		workspace.dirs["/srv/big"] = true
		configs := registry(shop(t), rootedAt(t, "big", "/srv/big"))
		if err := app.NewOpenProject(configs, sessions, workspace).HandleDir("/srv/shop/data"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"big"}) {
			t.Fatalf("got %q attached, want big", sessions.attached)
		}
	})

	t.Run("reports a directory no project contains", func(t *testing.T) {
		err := open(t, &fakeSessions{}, "/home/me", shop(t))
		if !errors.Is(err, app.ErrNoProjectAt) {
			t.Fatalf("got %v, want ErrNoProjectAt", err)
		}
	})

	t.Run("opens the matching project past an invalid config", func(t *testing.T) {
		sessions := &fakeSessions{}
		if err := openPastBroken(t, sessions, "/srv/shop/front", shop(t)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(sessions.attached, []string{"shop"}) {
			t.Fatalf("got %q attached, want shop", sessions.attached)
		}
	})

	t.Run("names the invalid configs when no project contains the directory", func(t *testing.T) {
		err := openPastBroken(t, &fakeSessions{}, "/home/me", shop(t))
		if !errors.Is(err, app.ErrNoProjectAt) || !errors.Is(err, domain.ErrInvalidConfig) || !strings.Contains(err.Error(), "broken.yml") {
			t.Fatalf("got %v, want ErrNoProjectAt naming broken.yml", err)
		}
	})

	t.Run("names each root as written when projects share one through a symlink", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/home/me/blog": "/srv/blog"}
		configs := registry(rootedAt(t, "blog", "/home/me/blog"), rootedAt(t, "notes", "/srv/blog"))
		err := app.NewOpenProject(configs, &fakeSessions{}, workspace).HandleDir("/srv/blog")
		if !errors.Is(err, app.ErrAmbiguousProject) || !strings.Contains(err.Error(), "blog, rooted at /home/me/blog, and notes, rooted at /srv/blog") {
			t.Fatalf("got %v, want each project named with its own root", err)
		}
	})

	t.Run("refuses to guess between projects sharing a root", func(t *testing.T) {
		sessions := &fakeSessions{}
		err := open(t, sessions, "/srv/blog", rootedAt(t, "blog", "/srv/blog"), rootedAt(t, "notes", "/srv/blog"))
		if !errors.Is(err, app.ErrAmbiguousProject) || len(sessions.attached) != 0 {
			t.Fatalf("got %v with %q attached, want ErrAmbiguousProject and nothing attached", err, sessions.attached)
		}
	})
}
