package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

func reloading(t *testing.T, tasks *fakeTasks) (app.Reload, domain.ProjectRef) {
	t.Helper()
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
	name := tasked(t).Ref()
	return app.NewReload(configs, tasks), name
}

func TestReloadPane(t *testing.T) {
	front := app.PaneAt{Window: windowName(t, "shop"), Pane: 1}

	t.Run("restarts the pane and says so", func(t *testing.T) {
		tasks := runningShop(t)
		uc, name := reloading(t, tasks)
		notice, err := uc.HandlePane(name, front)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.reloaded, []string{"shop/1"}) || notice != "Reloaded shop › front." {
			t.Fatalf("reloaded %q, said %q; want shop/1", tasks.reloaded, notice)
		}
	})

	t.Run("points to reloading the window of a pane the session lacks", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.gone = map[string]bool{"shop/1": true}
		uc, name := reloading(t, tasks)
		_, err := uc.HandlePane(name, front)
		if !errors.Is(err, domain.ErrPaneNotRunning) || !strings.Contains(err.Error(), "reload the shop window") {
			t.Fatalf("got %v, want ErrPaneNotRunning pointing to the window", err)
		}
	})

	t.Run("refuses the pane the browser runs in", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.weaver = "shop/1"
		uc, name := reloading(t, tasks)
		if _, err := uc.HandlePane(name, front); !errors.Is(err, domain.ErrPaneRunsWeaver) {
			t.Fatalf("got %v, want ErrPaneRunsWeaver", err)
		}
	})

	t.Run("refuses a session weaver didn't start", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}}
		uc, name := reloading(t, tasks)
		if _, err := uc.HandlePane(name, front); !errors.Is(err, domain.ErrSessionNotOwned) || len(tasks.reloaded) > 0 {
			t.Fatalf("got %v, reloaded %q; want ErrSessionNotOwned", err, tasks.reloaded)
		}
	})
}

func TestReloadWindow(t *testing.T) {
	t.Run("replaces a window the session has", func(t *testing.T) {
		tasks := runningShop(t)
		uc, name := reloading(t, tasks)
		notice, err := uc.HandleWindow(name, windowName(t, "logs"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.reloaded, []string{"logs"}) || notice != "Reloaded logs." {
			t.Fatalf("reloaded %q, said %q; want logs", tasks.reloaded, notice)
		}
	})

	t.Run("restores a window the session lacks", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop")}}}
		uc, name := reloading(t, tasks)
		notice, err := uc.HandleWindow(name, windowName(t, "logs"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks.reloaded) > 0 || !slices.Equal(tasks.restored, []string{"logs"}) || notice != "Restored logs." {
			t.Fatalf("reloaded %q, restored %q, said %q; want logs restored", tasks.reloaded, tasks.restored, notice)
		}
	})

	t.Run("refuses the window the browser runs in", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.weaver = "logs/0"
		uc, name := reloading(t, tasks)
		if _, err := uc.HandleWindow(name, windowName(t, "logs")); !errors.Is(err, domain.ErrPaneRunsWeaver) {
			t.Fatalf("got %v, want ErrPaneRunsWeaver", err)
		}
	})

	t.Run("refuses a project that isn't running", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{}}
		uc, name := reloading(t, tasks)
		if _, err := uc.HandleWindow(name, windowName(t, "logs")); !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})
}
