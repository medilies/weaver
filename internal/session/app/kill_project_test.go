package app_test

import (
	"errors"
	"fmt"
	"slices"
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

func TestKillProject(t *testing.T) {
	t.Run("kills the project's session", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		if err := app.NewKillProject(killer, capturedSessions{}).Handle("shop"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("reports a session that isn't running", func(t *testing.T) {
		err := app.NewKillProject(&fakeKiller{}, capturedSessions{}).Handle("shop")
		if !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("rejects an invalid name before killing anything", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("my.app")}
		err := app.NewKillProject(killer, capturedSessions{}).Handle("my.app")
		if !errors.Is(err, domain.ErrInvalidProjectName) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrInvalidProjectName and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills the session weaver runs in", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop", "blog")}
		if err := app.NewKillProject(killer, capturedSessions{current: "shop"}).HandleCurrent(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("kills nothing outside tmux", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop")}
		err := app.NewKillProject(killer, capturedSessions{}).HandleCurrent()
		if !errors.Is(err, domain.ErrNotInsideTmux) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrNotInsideTmux and nothing killed", err, killer.killed)
		}
	})

	t.Run("leaves a current session no project could be named after", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("my session")}
		err := app.NewKillProject(killer, capturedSessions{current: "my session"}).HandleCurrent()
		if !errors.Is(err, domain.ErrInvalidProjectName) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrInvalidProjectName and nothing killed", err, killer.killed)
		}
	})

	t.Run("leaves a current session weaver didn't start", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]domain.RunningSession{"0": domain.NewRunningSession(false)}}
		err := app.NewKillProject(killer, capturedSessions{current: "0"}).HandleCurrent()
		if !errors.Is(err, domain.ErrSessionNotOwned) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrSessionNotOwned and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills nothing when it can't tell who started the current session", func(t *testing.T) {
		killer := &fakeKiller{running: runningOwned("shop"), findErr: errors.New("tmux list-windows: lost server")}
		err := app.NewKillProject(killer, capturedSessions{current: "shop"}).HandleCurrent()
		if err == nil || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want the error and nothing killed", err, killer.killed)
		}
	})

	t.Run("kills a named session weaver didn't start", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]domain.RunningSession{"0": domain.NewRunningSession(false)}}
		if err := app.NewKillProject(killer, capturedSessions{}).Handle("0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"0"}) {
			t.Fatalf("got %q killed, want 0", killer.killed)
		}
	})
}
