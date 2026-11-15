package app_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

func interrupting(t *testing.T, tasks *fakeTasks) (app.Interrupt, domain.ProjectName) {
	t.Helper()
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
	name, err := domain.NewProjectName("shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return app.NewInterrupt(configs, tasks), name
}

func TestInterruptTask(t *testing.T) {
	tailIn := func(t *testing.T, places ...string) []domain.RunningCommand {
		t.Helper()
		var runs []domain.RunningCommand
		for i, place := range places {
			runs = append(runs, domain.NewRunningCommand(windowName(t, "logs"), 0, command(t, "tail -f app.log")).
				RunningIn(place, place != "%3", time.Unix(int64(i), 0)))
		}
		return runs
	}
	tail := app.TaskAt{Window: windowName(t, "logs"), Pane: 0, Task: 0}

	t.Run("interrupts the task in its pane before its throwaway windows", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.commands = map[string][]domain.RunningCommand{"shop": tailIn(t, "%8", "%3", "%9")}
		uc, name := interrupting(t, tasks)
		notice, err := uc.HandleTask(name, tail)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.stopped, []string{"%3"}) || notice != "Interrupted tail in logs. 2 more copies run." {
			t.Fatalf("interrupted %q, said %q; want %%3 and the copies left", tasks.stopped, notice)
		}
	})

	t.Run("interrupts the most recent throwaway window when its pane doesn't run it", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.commands = map[string][]domain.RunningCommand{"shop": tailIn(t, "%8", "%9")}
		uc, name := interrupting(t, tasks)
		notice, err := uc.HandleTask(name, tail)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.stopped, []string{"%9"}) || notice != "Interrupted tail in a throwaway window. 1 more copy runs." {
			t.Fatalf("interrupted %q, said %q; want the newest, %%9", tasks.stopped, notice)
		}
	})

	t.Run("says there is nothing to interrupt when the task doesn't run", func(t *testing.T) {
		tasks := runningShop(t)
		uc, name := interrupting(t, tasks)
		if _, err := uc.HandleTask(name, tail); !errors.Is(err, app.ErrNothingToInterrupt) || len(tasks.stopped) > 0 {
			t.Fatalf("got %v, interrupted %q; want ErrNothingToInterrupt", err, tasks.stopped)
		}
	})

	t.Run("refuses a session weaver didn't start", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}}
		uc, name := interrupting(t, tasks)
		if _, err := uc.HandleTask(name, tail); !errors.Is(err, domain.ErrSessionNotOwned) {
			t.Fatalf("got %v, want ErrSessionNotOwned", err)
		}
	})
}

func TestInterruptPane(t *testing.T) {
	front := app.PaneAt{Window: windowName(t, "shop"), Pane: 1}

	t.Run("types ctrl+c into a busy pane", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		uc, name := interrupting(t, tasks)
		notice, err := uc.HandlePane(name, front)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.stopped, []string{"shop/1"}) || notice != "Interrupted shop › front." {
			t.Fatalf("interrupted %q, said %q; want shop/1", tasks.stopped, notice)
		}
	})

	t.Run("says there is nothing to interrupt in an idle pane", func(t *testing.T) {
		tasks := runningShop(t)
		uc, name := interrupting(t, tasks)
		if _, err := uc.HandlePane(name, front); !errors.Is(err, app.ErrNothingToInterrupt) || len(tasks.stopped) > 0 {
			t.Fatalf("got %v, interrupted %q; want ErrNothingToInterrupt", err, tasks.stopped)
		}
	})

	t.Run("refuses the pane the browser runs in", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.weaver = "shop/1"
		uc, name := interrupting(t, tasks)
		if _, err := uc.HandlePane(name, front); !errors.Is(err, domain.ErrPaneRunsWeaver) || len(tasks.stopped) > 0 {
			t.Fatalf("got %v, interrupted %q; want ErrPaneRunsWeaver", err, tasks.stopped)
		}
	})

	t.Run("refuses a project that isn't running", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{}}
		uc, name := interrupting(t, tasks)
		if _, err := uc.HandlePane(name, front); !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})
}
