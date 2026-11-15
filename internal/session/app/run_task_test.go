package app_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeTasks is an in-memory domain.TaskRunner over fakeSessions: the
// panes busy running something, by window and index, and what it was
// asked to run.
type fakeTasks struct {
	*fakeSessions
	busy    map[string]bool // "window/index"
	weaver  string          // the pane weaver runs in, "window/index"
	typed   []string        // "window/index: command"
	apart   []string        // "throwaway window in dir: command"
	landing []bool          // whether each run landed there
	reached []string        // the places of the running commands reached
}

func (f *fakeTasks) Reach(_ domain.ProjectName, run domain.RunningCommand) error {
	f.reached = append(f.reached, run.Place())
	return nil
}

func (f *fakeTasks) Busy(_ domain.ProjectName, window domain.WindowPlan, index int) (bool, error) {
	if paneKey(window, index) == f.weaver {
		return true, domain.ErrPaneRunsWeaver
	}
	return f.busy[paneKey(window, index)], nil
}

func (f *fakeTasks) Type(_ domain.ProjectName, window domain.WindowPlan, index int, cmd domain.Command, landing bool) error {
	f.typed = append(f.typed, paneKey(window, index)+": "+cmd.String())
	f.landing = append(f.landing, landing)
	return nil
}

func (f *fakeTasks) RunApart(_ domain.ProjectName, window domain.WindowPlan, index int, task domain.Task, landing bool) error {
	f.apart = append(f.apart, task.ThrowawayWindow().String()+" in "+window.Panes()[index].Dir()+": "+task.Command().String())
	f.landing = append(f.landing, landing)
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
	return app.NewRunTask(configs, tasks, confirm).Handle(name, at, func() error { return nil })
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

	t.Run("goes where a running task runs rather than running it again", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.commands = map[string][]domain.RunningCommand{"shop": {
			domain.NewRunningCommand(windowName(t, "logs"), 0, command(t, "tail -f app.log")).RunningIn("%3", false, time.Unix(1, 0)),
		}}
		if err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "logs"), Pane: 0, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks.typed)+len(tasks.apart) > 0 || !slices.Equal(tasks.reached, []string{"%3"}) || !slices.Equal(tasks.attached, []string{"shop"}) {
			t.Fatalf("ran %q %q, reached %q, attached %q; want %%3 reached and attached, nothing run", tasks.typed, tasks.apart, tasks.reached, tasks.attached)
		}
	})

	t.Run("goes to a running task's pane over its throwaway windows", func(t *testing.T) {
		tasks := runningShop(t)
		tail := command(t, "tail -f app.log")
		logs := windowName(t, "logs")
		tasks.commands = map[string][]domain.RunningCommand{"shop": {
			domain.NewRunningCommand(logs, 0, tail).RunningIn("%7", true, time.Unix(9, 0)),
			domain.NewRunningCommand(logs, 0, tail).RunningIn("%3", false, time.Unix(1, 0)),
		}}
		if err := runTask(t, tasks, answer(true), app.TaskAt{Window: logs, Pane: 0, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.reached, []string{"%3"}) {
			t.Fatalf("reached %q, want the pane %%3", tasks.reached)
		}
	})

	t.Run("goes to a running task's most recent throwaway window when its pane doesn't run it", func(t *testing.T) {
		tasks := runningShop(t)
		tail := command(t, "tail -f app.log")
		logs := windowName(t, "logs")
		tasks.commands = map[string][]domain.RunningCommand{"shop": {
			domain.NewRunningCommand(logs, 0, tail).RunningIn("%7", true, time.Unix(5, 0)),
			domain.NewRunningCommand(logs, 0, tail).RunningIn("%9", true, time.Unix(9, 0)),
			domain.NewRunningCommand(logs, 0, command(t, "vim app.log")).RunningIn("%3", false, time.Unix(1, 0)),
		}}
		if err := runTask(t, tasks, answer(true), app.TaskAt{Window: logs, Pane: 0, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.reached, []string{"%9"}) {
			t.Fatalf("reached %q, want the newest throwaway window's pane %%9", tasks.reached)
		}
	})

	t.Run("refuses a project that isn't running", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{}}
		err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !errors.Is(err, domain.ErrSessionNotRunning) || !strings.Contains(err.Error(), "open or start it first") {
			t.Fatalf("got %v, want ErrSessionNotRunning saying to open or start it first", err)
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

	t.Run("lands where the task runs, so the user is put there", func(t *testing.T) {
		tasks := runningShop(t)
		if err := runTask(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.landing, []bool{true}) {
			t.Fatalf("got landing %v, want the run to land", tasks.landing)
		}
	})

	t.Run("refuses the pane the browser runs in, pointing to w", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.weaver = "shop/1"
		err := runTask(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !errors.Is(err, domain.ErrPaneRunsWeaver) || err.Error() != "front runs this browser: w runs migrate in a throwaway window" {
			t.Fatalf("got %v, want front refused as running this browser, pointing to w", err)
		}
		if len(tasks.typed)+len(tasks.apart)+len(tasks.attached) > 0 {
			t.Fatalf("ran %q %q, attached %q, want nothing run", tasks.typed, tasks.apart, tasks.attached)
		}
	})

	t.Run("rejects a task the pane doesn't offer", func(t *testing.T) {
		err := runTask(t, runningShop(t), answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 0, Task: 0})
		if !errors.Is(err, domain.ErrTaskNotFound) {
			t.Fatalf("got %v, want ErrTaskNotFound", err)
		}
	})
}

func TestRunTaskStaying(t *testing.T) {
	// staying runs the task at of the tasked shop project, its session in
	// tasks, confirming with confirm, leaving the user where they are, and
	// returns what it said.
	staying := func(t *testing.T, tasks *fakeTasks, confirm app.Confirmer, at app.TaskAt) string {
		t.Helper()
		configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
		name, err := domain.NewProjectName("shop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		said, err := app.NewRunTask(configs, tasks, confirm).HandleStaying(name, at, func(string) {})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks.attached) != 0 {
			t.Fatalf("got %q attached, want the user left where they are", tasks.attached)
		}
		return said
	}

	t.Run("types the task into its pane, saying where", func(t *testing.T) {
		tasks := runningShop(t)
		said := staying(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if said != "Running migrate in shop › front." || len(tasks.typed) != 1 {
			t.Fatalf("got %q, typed %q; want migrate typed in front", said, tasks.typed)
		}
	})

	t.Run("names the window of a window's only, unnamed pane", func(t *testing.T) {
		said := staying(t, runningShop(t), answer(false), app.TaskAt{Window: windowName(t, "logs"), Pane: 0, Task: 0})
		if said != "Running tail in logs." {
			t.Fatalf("got %q, want tail running in logs", said)
		}
	})

	t.Run("leaves the session's current window and pane where they are", func(t *testing.T) {
		tasks := runningShop(t)
		staying(t, tasks, answer(false), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if !slices.Equal(tasks.landing, []bool{false}) {
			t.Fatalf("got landing %v, want the run not to land", tasks.landing)
		}
	})

	t.Run("says a task run apart runs in a throwaway window", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		said := staying(t, tasks, answer(true), app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0})
		if said != "Running migrate in a throwaway window." || len(tasks.apart) != 1 {
			t.Fatalf("got %q, apart %q; want migrate run apart", said, tasks.apart)
		}
	})
}

func TestRunTaskTellsWhatItIsDoing(t *testing.T) {
	// told runs the task at of the tasked shop project as HandleStaying
	// does, its session in tasks, answering confirm, and returns what it
	// told it was doing.
	told := func(t *testing.T, tasks *fakeTasks, confirm app.Confirmer) []string {
		t.Helper()
		configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
		name, err := domain.NewProjectName("shop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var working []string
		_, _ = app.NewRunTask(configs, tasks, confirm).HandleStaying(name, app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0}, func(text string) {
			working = append(working, text)
		})
		return working
	}

	t.Run("tells it runs the task in its idle pane", func(t *testing.T) {
		if got, want := told(t, runningShop(t), answer(false)), []string{"Running migrate…"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("tells it runs the task apart once agreed", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		if got, want := told(t, tasks, answer(true)), []string{"Running migrate in a throwaway window…"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("tells nothing when running apart is declined", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.busy = map[string]bool{"shop/1": true}
		if got := told(t, tasks, answer(false)); len(got) != 0 {
			t.Fatalf("got %q, want nothing told", got)
		}
	})
}

func TestRunTaskApart(t *testing.T) {
	// apart runs the task at of the tasked shop project in a throwaway
	// window, its session in tasks, asking confirm.
	apart := func(t *testing.T, tasks *fakeTasks, confirm app.Confirmer, at app.TaskAt) (string, error) {
		t.Helper()
		configs := inMemoryRegistry{projects: map[string]domain.Project{"shop": tasked(t)}}
		name, err := domain.NewProjectName("shop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return app.NewRunTask(configs, tasks, confirm).HandleApart(name, at, func(string) {})
	}
	migrate := func(t *testing.T) app.TaskAt {
		return app.TaskAt{Window: windowName(t, "shop"), Pane: 1, Task: 0}
	}

	t.Run("runs the task in a throwaway window without asking, its pane idle", func(t *testing.T) {
		tasks, confirm := runningShop(t), &countingConfirmer{}
		said, err := apart(t, tasks, confirm, migrate(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if said != "Running migrate in a throwaway window." || len(tasks.apart) != 1 || len(tasks.typed) > 0 || confirm.asked != 0 {
			t.Fatalf("got %q, apart %q, typed %q after %d questions; want migrate apart, unasked", said, tasks.apart, tasks.typed, confirm.asked)
		}
	})

	t.Run("runs a task of the pane the browser runs in", func(t *testing.T) {
		tasks := runningShop(t)
		tasks.weaver = "shop/1"
		if _, err := apart(t, tasks, answer(false), migrate(t)); err != nil || len(tasks.apart) != 1 {
			t.Fatalf("got %v, apart %q; want migrate apart", err, tasks.apart)
		}
	})

	t.Run("leaves the user where they are", func(t *testing.T) {
		tasks := runningShop(t)
		if _, err := apart(t, tasks, answer(false), migrate(t)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(tasks.landing, []bool{false}) || len(tasks.attached) > 0 {
			t.Fatalf("got landing %v, attached %q; want neither", tasks.landing, tasks.attached)
		}
	})

	t.Run("refuses a project that isn't running", func(t *testing.T) {
		tasks := &fakeTasks{fakeSessions: &fakeSessions{}}
		if _, err := apart(t, tasks, answer(true), migrate(t)); !errors.Is(err, domain.ErrSessionNotRunning) || len(tasks.apart) > 0 {
			t.Fatalf("got %v, apart %q; want ErrSessionNotRunning and nothing run", err, tasks.apart)
		}
	})
}
