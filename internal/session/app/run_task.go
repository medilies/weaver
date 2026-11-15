package app

import (
	"errors"
	"fmt"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// TaskAt is a task by where it is: the window of the pane offering it,
// the pane's index in that window and its own in the pane's tasks, from 0.
type TaskAt struct {
	Window domain.WindowName
	Pane   int
	Task   int
}

// RunTask runs a pane's task in its project's running session: typed into
// the pane when its shell is idle, otherwise, if the user agrees, in a
// throwaway window, or there at once when they ask for one. Then it puts
// the user where the task runs, unless they chose to stay where they are.
// A task is never typed into the pane the browser runs in. A task already
// running isn't run again, unless asked to: the user is put where it runs.
type RunTask struct {
	projects domain.ProjectRegistry
	tasks    domain.TaskRunner
	confirm  Confirmer
}

// NewRunTask constructs the use case against its ports.
func NewRunTask(projects domain.ProjectRegistry, tasks domain.TaskRunner, confirm Confirmer) RunTask {
	return RunTask{projects: projects, tasks: tasks, confirm: confirm}
}

// Handle runs the task at of project, then calls leaving, which frees the
// terminal for attaching, and puts the user where the task runs. A task
// that already runs isn't run again: the user is put where it runs, in
// its pane when it runs there, else in its most recent copy. A project
// that isn't running is refused, with an error wrapping
// ErrSessionNotRunning, and one whose session weaver didn't start, whose
// panes it can't tell, with ErrSessionNotOwned; a task of the pane the
// browser runs in with ErrPaneRunsWeaver; declining to run the task apart
// is ErrAborted.
func (uc RunTask) Handle(name domain.ProjectName, at TaskAt, leaving func() error) error {
	window, _, task, err := uc.found(name, at)
	if err != nil {
		return err
	}
	copies, err := copiesOf(uc.tasks, name, window, at.Pane, task.Command())
	if err != nil {
		return err
	}
	if len(copies) > 0 {
		if err := uc.tasks.Reach(name, copies[0]); err != nil {
			return fmt.Errorf("%s: %w", task.Label(), err)
		}
	} else if _, _, err := uc.started(name, at, true, func(string) {}); err != nil {
		return err
	}
	if err := leaving(); err != nil {
		return err
	}
	if err := uc.tasks.Attach(name); err != nil {
		return fmt.Errorf("%s running, but: %w", task.Label(), err)
	}
	return nil
}

// HandleStaying runs the task at of project as Handle does, but leaves
// the user where they are, the session's current window and pane
// included, and says where it runs: "Running migrate in api › back."
// It calls working with what it is doing once there is no more to ask.
func (uc RunTask) HandleStaying(name domain.ProjectName, at TaskAt, working func(string)) (string, error) {
	task, place, err := uc.started(name, at, false, working)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Running %s in %s.", task.Label(), place), nil
}

// HandleApart runs the task at of project in a throwaway window without
// asking, busy pane or not, leaving the user where they are as
// HandleStaying does, and says so.
func (uc RunTask) HandleApart(name domain.ProjectName, at TaskAt, working func(string)) (string, error) {
	window, _, task, err := uc.found(name, at)
	if err != nil {
		return "", err
	}
	working(apartWorking(task))
	if err := uc.tasks.RunApart(name, window, at.Pane, task, false); err != nil {
		return "", err
	}
	return fmt.Sprintf("Running %s in a throwaway window.", task.Label()), nil
}

// found is the task at of project, with its pane and that pane's window,
// once its session is found running and owned.
func (uc RunTask) found(name domain.ProjectName, at TaskAt) (domain.WindowPlan, domain.PanePlan, domain.Task, error) {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, err
	}
	window, pane, task, err := taskOf(project, at)
	if err != nil {
		return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, err
	}
	if err := requireOwned(uc.tasks, name); err != nil {
		return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, err
	}
	return window, pane, task, nil
}

// started runs the task at of project, landing there or not, and returns
// it and where it runs: its pane, "api › back", or the throwaway window it
// ran apart in, calling working with what it is doing.
func (uc RunTask) started(name domain.ProjectName, at TaskAt, landing bool, working func(string)) (domain.Task, string, error) {
	window, pane, task, err := uc.found(name, at)
	if err != nil {
		return domain.Task{}, "", err
	}
	apart, err := uc.run(name, window, at.Pane, pane, task, landing, working)
	if err != nil {
		return domain.Task{}, "", err
	}
	place := window.Name().String()
	switch {
	case apart:
		place = "a throwaway window"
	case !pane.Name().IsZero():
		place += " › " + pane.Name().String()
	}
	return task, place, nil
}

// run types task into pane, at index of window, when its shell is idle,
// or else, if the user agrees, runs it in a throwaway window, and reports
// whether it did so, calling working with what it does once agreed. The
// pane the browser runs in is refused, pointing to w, which runs a task
// apart.
func (uc RunTask) run(name domain.ProjectName, window domain.WindowPlan, index int, pane domain.PanePlan, task domain.Task, landing bool, working func(string)) (bool, error) {
	shown := pane.Name().String()
	if pane.Name().IsZero() {
		shown = window.Name().String()
	}
	busy, err := uc.tasks.Busy(name, window, index)
	if errors.Is(err, domain.ErrPaneRunsWeaver) {
		return false, browserPane{fmt.Sprintf("%s runs this browser: w runs %s in a throwaway window", shown, task.Label())}
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", where(window, pane), err)
	}
	if !busy {
		working(fmt.Sprintf("Running %s…", task.Label()))
		return false, uc.tasks.Type(name, window, index, task.Command(), landing)
	}
	yes, err := uc.confirm.Confirm(fmt.Sprintf("%s is busy: run %s in a throwaway window?", shown, task.Label()))
	if err != nil {
		return false, err
	}
	if !yes {
		return false, ErrAborted
	}
	working(apartWorking(task))
	return true, uc.tasks.RunApart(name, window, index, task, landing)
}

// copiesOf are the copies of cmd typed for the pane at index of window
// still running in the project's session, in the order the user reaches
// them: the one in the pane itself, then the others, most recent first.
func copiesOf(tracker domain.CommandTracker, name domain.ProjectName, window domain.WindowPlan, index int, cmd domain.Command) ([]domain.RunningCommand, error) {
	runs, err := tracker.RunningCommands(name, []domain.WindowPlan{window})
	if err != nil {
		return nil, err
	}
	copies := slices.DeleteFunc(runs, func(r domain.RunningCommand) bool {
		return !r.Of(window.Name(), index, cmd)
	})
	slices.SortStableFunc(copies, func(a, b domain.RunningCommand) int {
		if a.Apart() != b.Apart() {
			if a.Apart() {
				return 1
			}
			return -1
		}
		return b.Since().Compare(a.Since())
	})
	return copies, nil
}

// apartWorking is what running task in a throwaway window is doing.
func apartWorking(task domain.Task) string {
	return fmt.Sprintf("Running %s in a throwaway window…", task.Label())
}

// taskOf finds the task at of project, with its pane and that pane's
// window as the project opens them.
func taskOf(project domain.Project, at TaskAt) (domain.WindowPlan, domain.PanePlan, domain.Task, error) {
	for _, window := range project.Plan().Windows() {
		if window.Name() != at.Window {
			continue
		}
		panes := window.Panes()
		if at.Pane < 0 || at.Pane >= len(panes) {
			return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, fmt.Errorf("window %q: %w: #%d", at.Window, domain.ErrPaneNotFound, at.Pane+1)
		}
		pane := panes[at.Pane]
		tasks := pane.Tasks()
		if at.Task < 0 || at.Task >= len(tasks) {
			return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, fmt.Errorf("%s: %w: #%d", where(window, pane), domain.ErrTaskNotFound, at.Task+1)
		}
		return window, pane, tasks[at.Task], nil
	}
	return domain.WindowPlan{}, domain.PanePlan{}, domain.Task{}, fmt.Errorf("%w: %q", domain.ErrWindowNotFound, at.Window)
}

// browserPane is domain.ErrPaneRunsWeaver as the browser tells it: the
// pane runs the browser, and w runs the task apart.
type browserPane struct{ text string }

func (e browserPane) Error() string { return e.text }

func (e browserPane) Unwrap() error { return domain.ErrPaneRunsWeaver }
