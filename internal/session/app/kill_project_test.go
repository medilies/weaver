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
	running map[string]bool
	killed  []string
}

func (k *fakeKiller) Kill(name domain.ProjectName) error {
	if !k.running[name.String()] {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	k.killed = append(k.killed, name.String())
	return nil
}

func TestKillProject(t *testing.T) {
	t.Run("kills the project's session", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]bool{"shop": true}}
		if err := app.NewKillProject(killer).Handle("shop"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(killer.killed, []string{"shop"}) {
			t.Fatalf("got %q killed, want shop", killer.killed)
		}
	})

	t.Run("reports a session that isn't running", func(t *testing.T) {
		err := app.NewKillProject(&fakeKiller{}).Handle("shop")
		if !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("rejects an invalid name before killing anything", func(t *testing.T) {
		killer := &fakeKiller{running: map[string]bool{"my.app": true}}
		err := app.NewKillProject(killer).Handle("my.app")
		if !errors.Is(err, domain.ErrInvalidProjectName) || len(killer.killed) != 0 {
			t.Fatalf("got %v with %q killed, want ErrInvalidProjectName and nothing killed", err, killer.killed)
		}
	})
}
