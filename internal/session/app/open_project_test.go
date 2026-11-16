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

// fakeSessions is an in-memory domain.SessionLauncher recording what it
// was asked to do.
type fakeSessions struct {
	running  map[string]domain.RunningSession
	started  []string
	restored []string
	attached []string
	idle     map[string]map[domain.WindowName][]int // by project, the panes whose shell idles
	commands map[string][]domain.RunningCommand     // by project, the commands weaver typed still running
	previews map[string]domain.WindowPreview        // by project and window, "shop/api"
	onAttach func()                                 // called as each attach happens
	unlisted error                                  // why listing the sessions fails, if it does
}

func (s *fakeSessions) Preview(name domain.ProjectRef, window domain.WindowPlan) (domain.WindowPreview, error) {
	if _, ok := s.running[name.String()]; !ok {
		return domain.WindowPreview{}, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	preview, ok := s.previews[name.String()+"/"+window.Name().String()]
	if !ok {
		return domain.WindowPreview{}, fmt.Errorf("%w: %s", domain.ErrWindowNotRunning, window.Name())
	}
	return preview, nil
}

func (s *fakeSessions) RunningCommands(name domain.ProjectRef, _ []domain.WindowPlan) ([]domain.RunningCommand, error) {
	if _, ok := s.running[name.String()]; !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	return s.commands[name.String()], nil
}

func (s *fakeSessions) Idle(name domain.ProjectRef, _ []domain.WindowPlan) (map[domain.WindowName][]int, error) {
	if _, ok := s.running[name.String()]; !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	return s.idle[name.String()], nil
}

func (s *fakeSessions) Find(name domain.ProjectRef) (domain.RunningSession, bool, error) {
	session, ok := s.running[name.String()]
	return session, ok, nil
}

func (s *fakeSessions) Restore(_ domain.ProjectRef, placements []domain.WindowPlacement) error {
	for _, placement := range placements {
		s.restored = append(s.restored, placement.Window().Name().String())
	}
	return nil
}

func (s *fakeSessions) Start(plan domain.SessionPlan) error {
	s.started = append(s.started, plan.Project().String())
	return nil
}

func (s *fakeSessions) Attach(name domain.ProjectRef) error {
	s.attached = append(s.attached, name.String())
	if s.onAttach != nil {
		s.onAttach()
	}
	return nil
}

// listed is running as tmux lists it: each session started for, or in,
// the root of the project it is keyed by, /srv/ and its name, as
// project builds them.
func listed(running map[string]domain.RunningSession) []domain.ListedSession {
	var sessions []domain.ListedSession
	for _, name := range slices.Sorted(maps.Keys(running)) {
		session := domain.NewListedSession("w/"+name, "/srv/"+name)
		if running[name].Owned() {
			session = session.StartedFor("/srv/" + name)
		}
		sessions = append(sessions, session)
	}
	return sessions
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
	return app.NewOpenProject(configs, sessions, workspace, &fakeCommands{}).Handle(rootOf(t, "shop"))
}

// openSetUpShop runs OpenProject on the shop project, with APP_ENV=local
// and two setup commands, running them with commands.
func openSetUpShop(t *testing.T, sessions *fakeSessions, workspace *fakeWorkspace, commands *fakeCommands) error {
	t.Helper()
	env, err := domain.NewEnv(map[string]string{"APP_ENV": "local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setup := shop(t).WithEnv(env).SettingUp(command(t, "docker compose up -d"), command(t, "php artisan migrate"))
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": setup}}
	return app.NewOpenProject(configs, sessions, workspace, commands).Handle(rootOf(t, "shop"))
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
		if err == nil || !strings.HasSuffix(err.Error(), "/srv/shop/logs (create it, or edit shop with e in weave)") {
			t.Fatalf("got %v, want a hint to create it or edit shop", err)
		}
	})

	t.Run("runs the setup commands in the root, in order, with the project's env, then starts", func(t *testing.T) {
		sessions, commands := &fakeSessions{}, &fakeCommands{}
		if err := openSetUpShop(t, sessions, everything(), commands); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := commands.commandsRun(); !slices.Equal(got, []string{"docker compose up -d", "php artisan migrate"}) {
			t.Fatalf("got %q run, want both setup commands in order", got)
		}
		for _, ran := range commands.ran {
			if vars := ran.env.Vars(); ran.dir != "/srv/shop" || len(vars) != 1 || vars[0].Name() != "APP_ENV" {
				t.Fatalf("got %q run in %s with %v, want it in /srv/shop with APP_ENV", ran.cmd, ran.dir, vars)
			}
		}
		if !slices.Equal(sessions.started, []string{"shop"}) {
			t.Fatalf("got %q started, want shop", sessions.started)
		}
	})

	t.Run("stops at a failing setup command, starting nothing", func(t *testing.T) {
		sessions := &fakeSessions{}
		commands := &fakeCommands{fails: map[string]bool{"docker compose up -d": true}}
		err := openSetUpShop(t, sessions, everything(), commands)
		if !errors.Is(err, domain.ErrCommandFailed) || !strings.Contains(err.Error(), "setup: docker compose up -d: ") {
			t.Fatalf("got %v, want the failing setup command named", err)
		}
		if len(commands.ran) != 1 || len(sessions.started)+len(sessions.attached) != 0 {
			t.Fatalf("got %q run, %q started, %q attached; want nothing after the failure",
				commands.commandsRun(), sessions.started, sessions.attached)
		}
	})

	t.Run("runs the setup commands before checking directories, which they may create", func(t *testing.T) {
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		commands := &fakeCommands{}
		err := openSetUpShop(t, &fakeSessions{}, workspace, commands)
		if !errors.Is(err, app.ErrMissingDirectory) || len(commands.ran) != 2 {
			t.Fatalf("got %v with %q run, want both run before the missing directory", err, commands.commandsRun())
		}
	})

	t.Run("says the setup commands ran when opening fails after them", func(t *testing.T) {
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		err := openSetUpShop(t, &fakeSessions{}, workspace, &fakeCommands{})
		if err == nil || !strings.Contains(err.Error(), "the setup commands already ran") {
			t.Fatalf("got %v, want it to say the setup commands already ran", err)
		}
	})

	t.Run("runs no setup command in a missing root", func(t *testing.T) {
		workspace := everything()
		delete(workspace.dirs, "/srv/shop")
		commands := &fakeCommands{}
		err := openSetUpShop(t, &fakeSessions{}, workspace, commands)
		if !errors.Is(err, app.ErrMissingDirectory) || len(commands.ran) != 0 {
			t.Fatalf("got %v with %q run, want the missing root and nothing run", err, commands.commandsRun())
		}
	})

	t.Run("doesn't run the setup commands for a running session", func(t *testing.T) {
		for _, session := range []domain.RunningSession{owned(t, "shop"), domain.NewRunningSession(false)} {
			sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": session}}
			commands := &fakeCommands{}
			if err := openSetUpShop(t, sessions, everything(), commands); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(commands.ran) != 0 {
				t.Fatalf("got %q run, want none", commands.commandsRun())
			}
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		configs := inMemoryRegistry{projects: map[string]domain.Project{}}
		err := app.NewOpenProject(configs, &fakeSessions{}, everything(), &fakeCommands{}).Handle(rootOf(t, "shop"))
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

func TestOpenProjectStart(t *testing.T) {
	// started starts shop against sessions, returning what it said.
	started := func(t *testing.T, sessions *fakeSessions) string {
		t.Helper()
		configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t)}}
		said, err := app.NewOpenProject(configs, sessions, everything(), &fakeCommands{}).HandleStart(rootOf(t, "shop"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions.attached) != 0 {
			t.Fatalf("got %q attached, want none", sessions.attached)
		}
		return said
	}

	t.Run("starts the session without attaching", func(t *testing.T) {
		sessions := &fakeSessions{}
		if said := started(t, sessions); said != "Started shop." || !slices.Equal(sessions.started, []string{"shop"}) {
			t.Fatalf("got %q, started %q; want shop started", said, sessions.started)
		}
	})

	t.Run("restores the windows weaver's session lacks without attaching", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop")}}
		if said := started(t, sessions); said != `Restored "logs" in shop.` || !slices.Equal(sessions.restored, []string{"logs"}) {
			t.Fatalf("got %q, restored %q; want logs restored", said, sessions.restored)
		}
	})

	t.Run("says a session with every window is already running", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop", "logs")}}
		if said := started(t, sessions); said != "shop is already running." {
			t.Fatalf("got %q, want shop already running", said)
		}
	})

	t.Run("fails, restoring nothing, on a missing directory of a window to restore", func(t *testing.T) {
		sessions := &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop")}}
		workspace := everything()
		delete(workspace.dirs, "/srv/shop/logs")
		configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t)}}
		said, err := app.NewOpenProject(configs, sessions, workspace, &fakeCommands{}).HandleStart(rootOf(t, "shop"))
		if !errors.Is(err, app.ErrMissingDirectory) || said != "" || len(sessions.restored) != 0 {
			t.Fatalf("got %q, %v, restored %q; want ErrMissingDirectory and nothing restored", said, err, sessions.restored)
		}
	})
}

// rootOf is the root of the project named raw, as project builds it.
func rootOf(t *testing.T, raw string) domain.ProjectRoot {
	t.Helper()
	return project(t, raw).Root()
}
