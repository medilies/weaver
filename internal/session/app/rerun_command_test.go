package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// rerun reruns the command of the pane at of shop, its front pane
// running npm run dev and its logs window's pane prefilling tail -f.
func rerun(t *testing.T, tasks *fakeTasks, at app.PaneAt) error {
	t.Helper()
	project := shop(t)
	windows := project.Windows()
	shopWindow, err := windows[0].ReplacingPane(1, windows[0].Panes()[1].Running(command(t, "npm run dev")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logs, err := windows[1].ReplacingPane(0, windows[1].Panes()[0].Prefilling(command(t, "tail -f app.log")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project, err = project.WithWindows(shopWindow, logs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": project}}
	return app.NewRerunCommand(configs, tasks).Handle(project.Name(), at)
}

func TestRerunCommand(t *testing.T) {
	front := func(t *testing.T) app.PaneAt { return app.PaneAt{Window: windowName(t, "shop"), Pane: 1} }

	t.Run("types an exited command into its pane again", func(t *testing.T) {
		tasks := runningShop(t)
		if err := rerun(t, tasks, front(t)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop/1: npm run dev"}; !slices.Equal(tasks.typed, want) {
			t.Fatalf("got %q typed, want %q", tasks.typed, want)
		}
	})

	t.Run("puts the user in the session after typing it", func(t *testing.T) {
		tasks := runningShop(t)
		if err := rerun(t, tasks, front(t)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop"}; !slices.Equal(tasks.attached, want) {
			t.Fatalf("got %q attached, want %q", tasks.attached, want)
		}
	})

	t.Run("refuses a command still running", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		if err := rerun(t, tasks, front(t)); !errors.Is(err, app.ErrNothingToRerun) || len(tasks.typed) > 0 {
			t.Fatalf("got %v with %q typed, want ErrNothingToRerun and nothing typed", err, tasks.typed)
		}
	})

	t.Run("refuses a pane without a command", func(t *testing.T) {
		tasks := runningShop(t)
		err := rerun(t, tasks, app.PaneAt{Window: windowName(t, "shop"), Pane: 0})
		if !errors.Is(err, app.ErrNothingToRerun) || len(tasks.typed) > 0 {
			t.Fatalf("got %v with %q typed, want ErrNothingToRerun and nothing typed", err, tasks.typed)
		}
	})

	t.Run("refuses a prefilled command, which never ran", func(t *testing.T) {
		tasks := runningShop(t)
		err := rerun(t, tasks, app.PaneAt{Window: windowName(t, "logs"), Pane: 0})
		if !errors.Is(err, app.ErrNothingToRerun) || len(tasks.typed) > 0 {
			t.Fatalf("got %v with %q typed, want ErrNothingToRerun and nothing typed", err, tasks.typed)
		}
	})

	t.Run("refuses a session weaver didn't start", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}}
		if err := rerun(t, tasks, front(t)); !errors.Is(err, domain.ErrSessionNotOwned) || len(tasks.typed) > 0 {
			t.Fatalf("got %v with %q typed, want ErrSessionNotOwned and nothing typed", err, tasks.typed)
		}
	})
}
