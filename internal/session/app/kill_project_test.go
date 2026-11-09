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

// fakeKiller is an in-memory domain.SessionKiller over running sessions.
type fakeKiller struct {
	running map[string]domain.RunningSession
	findErr error
	killed  []string
}

// runningOwned is the sessions named, each started by weaver.
func runningOwned(names ...string) map[string]domain.RunningSession {
	running := make(map[string]domain.RunningSession)
	for _, name := range names {
		running[name] = domain.NewRunningSession(true)
	}
	return running
}

func (k *fakeKiller) Find(name domain.ProjectName) (domain.RunningSession, bool, error) {
	session, ok := k.running[name.String()]
	return session, ok, k.findErr
}

func (k *fakeKiller) Kill(name domain.ProjectName) error {
	if _, ok := k.running[name.String()]; !ok {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	k.killed = append(k.killed, name.String())
	return nil
}

// noProjects is a registry without projects, so killing tears nothing
// down.
func noProjects() inMemoryRegistry {
	return inMemoryRegistry{projects: map[string]domain.Project{}}
}

// tornDownShop is a registry holding shop, with APP_ENV=local and two
// teardown commands.
func tornDownShop(t *testing.T) inMemoryRegistry {
	t.Helper()
	env, err := domain.NewEnv(map[string]string{"APP_ENV": "local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	torn := shop(t).WithEnv(env).TearingDown(command(t, "docker compose down"), command(t, "rm -f .lock"))
	return inMemoryRegistry{projects: map[string]domain.Project{"shop": torn}}
}

func TestKillProjectTearsDown(t *testing.T) {
	t.Run("runs the teardown commands in the root, in order, with the project's env, before killing", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{}
		commands.onRun = func() {
			if len(killer.killed) != 0 {
				t.Fatalf("got a teardown command run after the kill")
			}
		}
		if err := app.NewKillProject(killer, capturedSessions{}, tornDownShop(t), commands, &fakeAcks{}).Handle("shop"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := commands.commandsRun(); !slices.Equal(got, []string{"docker compose down", "rm -f .lock"}) {
			t.Fatalf("got %q run, want both teardown commands in order", got)
		}
		for _, ran := range commands.ran {
			if vars := ran.env.Vars(); ran.dir != "/srv/shop" || len(vars) != 1 || vars[0].Name() != "APP_ENV" {
				t.Fatalf("got %q run in %s with %v, want it in /srv/shop with APP_ENV", ran.cmd, ran.dir, vars)
			}
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("tears down the session weaver runs in", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{}
		if err := app.NewKillProject(killer, capturedSessions{current: "shop"}, tornDownShop(t), commands, &fakeAcks{}).HandleCurrent(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(commands.ran) != 2 || !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q run, %q killed; want shop torn down and killed", commands.commandsRun(), killer.killed)
		}
	})

	t.Run("doesn't tear down a session weaver didn't start", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}
		commands := &fakeCommands{}
		if err := app.NewKillProject(killer, capturedSessions{}, tornDownShop(t), commands, &fakeAcks{}).Handle("shop"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(commands.ran) != 0 || !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q run, %q killed; want shop killed only", commands.commandsRun(), killer.killed)
		}
	})

	t.Run("doesn't tear down a session that isn't running", func(t *testing.T) {
		commands := &fakeCommands{}
		err := app.NewKillProject(&fakeKiller{}, capturedSessions{}, tornDownShop(t), commands, &fakeAcks{}).Handle("shop")
		if !errors.Is(err, domain.ErrSessionNotRunning) || len(commands.ran) != 0 {
			t.Fatalf("got %v with %q run, want ErrSessionNotRunning and nothing run", err, commands.commandsRun())
		}
	})

	t.Run("kills the session when a teardown command fails, stopping at it, and reports it", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{fails: map[string]bool{"docker compose down": true}}
		err := app.NewKillProject(killer, capturedSessions{}, tornDownShop(t), commands, &fakeAcks{}).Handle("shop")
		if !errors.Is(err, domain.ErrCommandFailed) || !strings.Contains(err.Error(), "teardown: docker compose down: ") ||
			!strings.HasSuffix(err.Error(), "shop's session was killed anyway") {
			t.Fatalf("got %v, want the failing teardown command named and the kill told", err)
		}
		if len(commands.ran) != 1 || !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q run, %q killed; want the first command only, and shop killed", commands.commandsRun(), killer.killed)
		}
	})

	t.Run("shows a failing teardown before killing the session weaver runs in", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{fails: map[string]bool{"docker compose down": true}}
		acks := &fakeAcks{}
		err := app.NewKillProject(killer, capturedSessions{current: "shop"}, tornDownShop(t), commands, acks).HandleCurrent()
		if !errors.Is(err, domain.ErrCommandFailed) {
			t.Fatalf("got %v, want ErrCommandFailed", err)
		}
		if len(acks.shown) != 1 || !strings.Contains(acks.shown[0], "teardown: docker compose down: ") {
			t.Fatalf("got %q shown, want the failing teardown command", acks.shown)
		}
	})

	t.Run("shows nothing when the teardown succeeds in the session weaver runs in", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		acks := &fakeAcks{}
		if err := app.NewKillProject(killer, capturedSessions{current: "shop"}, tornDownShop(t), &fakeCommands{}, acks).HandleCurrent(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(acks.shown) != 0 {
			t.Fatalf("got %q shown, want nothing", acks.shown)
		}
	})

	t.Run("reports failing to show a failing teardown, with the teardown", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{fails: map[string]bool{"docker compose down": true}}
		lost := errors.New("terminal gone")
		err := app.NewKillProject(killer, capturedSessions{current: "shop"}, tornDownShop(t), commands, &fakeAcks{err: lost}).HandleCurrent()
		if !errors.Is(err, lost) || !errors.Is(err, domain.ErrCommandFailed) {
			t.Fatalf("got %v, want both the lost terminal and the failing teardown", err)
		}
	})

	t.Run("leaves a failing teardown to the error when weaver runs outside the session", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		commands := &fakeCommands{fails: map[string]bool{"docker compose down": true}}
		acks := &fakeAcks{}
		_ = app.NewKillProject(killer, capturedSessions{current: "blog"}, tornDownShop(t), commands, acks).Handle("shop")
		if len(acks.shown) != 0 {
			t.Fatalf("got %q shown, want nothing", acks.shown)
		}
	})

	t.Run("kills the session when its config can't be read, saying teardown was skipped", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("broken")}
		registry := withBrokenConfig{inMemoryRegistry: noProjects()}
		err := app.NewKillProject(killer, capturedSessions{}, registry, &fakeCommands{}, &fakeAcks{}).Handle("broken")
		if !errors.Is(err, domain.ErrInvalidConfig) || !strings.HasPrefix(err.Error(), "teardown skipped: ") {
			t.Fatalf("got %v, want teardown skipped for the invalid config", err)
		}
		if !slices.Equal(killer.killed, []string{"broken"}) {
			t.Fatalf("got %q killed, want broken", killer.killed)
		}
	})
}

func TestKillProject(t *testing.T) {
	t.Run("kills the project's session", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		if err := app.NewKillProject(killer, capturedSessions{}, noProjects(), &fakeCommands{}, &fakeAcks{}).Handle("shop"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("reports a session that isn't running", func(t *testing.T) {
		err := app.NewKillProject(&fakeKiller{}, capturedSessions{}, noProjects(), &fakeCommands{}, &fakeAcks{}).Handle("shop")
		if !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("rejects an invalid name before killing anything", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("my.app")}
		err := app.NewKillProject(killer, capturedSessions{}, noProjects(), &fakeCommands{}, &fakeAcks{}).Handle("my.app")
		if !errors.Is(err, domain.ErrInvalidProjectName) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrInvalidProjectName and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills the session weaver runs in", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop", "blog")}
		if err := app.NewKillProject(killer, capturedSessions{current: "shop"}, noProjects(), &fakeCommands{}, &fakeAcks{}).HandleCurrent(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("kills nothing outside tmux", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		err := app.NewKillProject(killer, capturedSessions{}, noProjects(), &fakeCommands{}, &fakeAcks{}).HandleCurrent()
		if !errors.Is(err, domain.ErrNotInsideTmux) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrNotInsideTmux and nothing killed", err, killer.killed)
		}
	})

	t.Run("leaves a current session no project could be named after", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("my session")}
		err := app.NewKillProject(killer, capturedSessions{current: "my session"}, noProjects(), &fakeCommands{}, &fakeAcks{}).HandleCurrent()
		if !errors.Is(err, domain.ErrInvalidProjectName) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrInvalidProjectName and nothing killed", err, killer.killed)
		}
	})

	t.Run("leaves a current session weaver didn't start", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]domain.RunningSession{"0": domain.NewRunningSession(false)}}
		err := app.NewKillProject(killer, capturedSessions{current: "0"}, noProjects(), &fakeCommands{}, &fakeAcks{}).HandleCurrent()
		if !errors.Is(err, domain.ErrSessionNotOwned) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrSessionNotOwned and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills nothing when it can't tell who started the current session", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop"), findErr: errors.New("tmux list-windows: lost server")}
		err := app.NewKillProject(killer, capturedSessions{current: "shop"}, noProjects(), &fakeCommands{}, &fakeAcks{}).HandleCurrent()
		if err == nil || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want the error and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills a named session weaver didn't start", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]domain.RunningSession{"0": domain.NewRunningSession(false)}}
		if err := app.NewKillProject(killer, capturedSessions{}, noProjects(), &fakeCommands{}, &fakeAcks{}).Handle("0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"0"}) {
			t.Fatalf("got %q killed, want 0", killer.killed)
		}
	})
}
