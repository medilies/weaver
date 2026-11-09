package app_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeTasks is an in-memory domain.TaskRunner over fakeSessions: the
// panes busy running something, by window and index, and what it was
// asked to run.
type fakeTasks struct {
	*fakeSessions
	busy  map[string]bool // "window/index"
	typed []string        // "window/index: command"
	apart []string        // "throwaway window in dir: command"
}

func (f *fakeTasks) Busy(_ domain.ProjectName, window domain.WindowPlan, index int) (bool, error) {
	return f.busy[paneKey(window, index)], nil
}

func (f *fakeTasks) Type(_ domain.ProjectName, window domain.WindowPlan, index int, task domain.Task) error {
	f.typed = append(f.typed, paneKey(window, index)+": "+task.Command().String())
	return nil
}

func (f *fakeTasks) RunApart(_ domain.ProjectName, pane domain.PanePlan, task domain.Task) error {
	f.apart = append(f.apart, task.ThrowawayWindow().String()+" in "+pane.Dir()+": "+task.Command().String())
	return nil
}

func paneKey(window domain.WindowPlan, index int) string {
	return window.Name().String() + "/" + strconv.Itoa(index)
}

// tasked is shop, its front pane offering a migrate task and an unnamed
// npm run build one, and its logs window's only pane a tail task.
func tasked(t *testing.T) domain.Project {
	t.Helper()
	newTask := func(name, raw string) domain.Task {
		t.Helper()
		cmd, err := domain.NewCommand(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		task, err := domain.NewTask(name, cmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return task
	}
	project := shop(t)
	windows := project.Windows()
	shopWindow, err := windows[0].ReplacingPane(1, windows[0].Panes()[1].Offering(
		newTask("migrate", "php artisan migrate"), newTask("", "npm run build")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logs, err := windows[1].ReplacingPane(0, windows[1].Panes()[0].Offering(newTask("tail", "tail -f app.log")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	project, err = project.WithWindows(shopWindow, logs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return project
}

// runTask runs the task at of the tasked shop project, its session in
// sessions, confirming with confirm.
func runTask(t *testing.T, tasks *fakeTasks, confirm app.Confirmer, at app.TaskAt) error {
	t.Helper()
	configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
	name, err := domain.NewProjectName("shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return app.NewRunTask(configs, tasks, confirm).Handle(name, at)
}

func runningShop(t *testing.T) *fakeTasks {
	t.Helper()
	return &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": owned(t, "shop", "logs")}}}
}

func TestRunTask(t *testing.T) {
	t.Run("types the task into its pane when its shell is idle", func(t *testing.T) {
		tasks := runningShop(t)
		if err := runTask(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop/1: php artisan migrate"}; len(tasks.typed) != 1 || tasks.typed[0] != want[0] {
			t.Fatalf("got %q typed, want %q", tasks.typed, want)
		}
	})

	t.Run("puts the user in the session after typing the task", func(t *testing.T) {
		tasks := runningShop(t)
		if err := runTask(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "logs"), Pane: 0, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop"}; !slices.Equal(tasks.attached, want) {
			t.Fatalf("got %q attached, want %q", tasks.attached, want)
		}
	})

	t.Run("refuses a project that isn't running", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{}}
		err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !errors.Is(err, domain.ErrSessionNotRunning) || !strings.Contains(err.Error(), "open it first") {
			t.Fatalf("got %v, want ErrSessionNotRunning saying to open it first", err)
		}
		if len(tasks.typed)+len(tasks.apart) > 0 {
			t.Fatalf("ran %q %q, want nothing run", tasks.typed, tasks.apart)
		}
	})

	t.Run("refuses a session weaver didn't start", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{running: map[string]domain.RunningSession{"shop": domain.NewRunningSession(false)}}}
		err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !errors.Is(err, domain.ErrSessionNotOwned) || len(tasks.typed)+len(tasks.apart) > 0 {
			t.Fatalf("got %v, ran %q %q, want ErrSessionNotOwned and nothing run", err, tasks.typed, tasks.apart)
		}
	})

	t.Run("runs the task in a throwaway window when its pane is busy and the user agrees", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		if err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 1}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "npm in /srv/shop/front: npm run build"; len(tasks.apart) != 1 || tasks.apart[0] != want || len(tasks.typed) > 0 {
			t.Fatalf("got %q apart and %q typed, want %q apart only", tasks.apart, tasks.typed, want)
		}
	})

	t.Run("puts the user in the session after running the task apart", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		if err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 1}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop"}; !slices.Equal(tasks.attached, want) {
			t.Fatalf("got %q attached, want %q", tasks.attached, want)
		}
	})

	t.Run("runs nothing when its pane is busy and the user declines", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		err := runTask(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !errors.Is(err, app.ErrAborted) || len(tasks.typed)+len(tasks.apart)+len(tasks.attached) > 0 {
			t.Fatalf("got %v, ran %q %q, attached %q, want ErrAborted and nothing run", err, tasks.typed, tasks.apart, tasks.attached)
		}
	})

	t.Run("rejects a task the pane doesn't offer", func(t *testing.T) {
		err := runTask(t, runningShop(t), answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 0, Task: 0})
		if !errors.Is(err, domain.ErrTaskNotFound) {
			t.Fatalf("got %v, want ErrTaskNotFound", err)
		}
	})
}
