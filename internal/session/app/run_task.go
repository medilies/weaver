package app

import (
	"fmt"

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
// throwaway window. Then it puts the user where the task runs, unless they
// chose to stay where they are.
type RunTask struct {
	projects domain.ProjectRegistry
	tasks    domain.TaskRunner
	confirm  Confirmer
}

// NewRunTask constructs the use case against its ports.
func NewRunTask(projects domain.ProjectRegistry, tasks domain.TaskRunner, confirm Confirmer) RunTask {
	return RunTask{projects: projects, tasks: tasks, confirm: confirm}
}

// Handle runs the task at of project, and puts the user where it runs. A project
// that isn't running is refused, with an error wrapping
// ErrSessionNotRunning, and one whose session weaver didn't start, whose
// panes it can't tell, with ErrSessionNotOwned; declining to run the
// task apart is ErrAborted.
func (uc RunTask) Handle(name domain.ProjectName, at TaskAt) error {
	task, _, err := uc.started(name, at)
	if err != nil {
		return err
	}
	if err := uc.tasks.Attach(name); err != nil {
		return fmt.Errorf("%s running, but: %w", task.Label(), err)
	}
	return nil
}

// HandleStaying runs the task at of project as Handle does, but leaves
// the user where they are, and says where it runs: "Running migrate in
// api › back."
func (uc RunTask) HandleStaying(name domain.ProjectName, at TaskAt) (string, error) {
	task, place, err := uc.started(name, at)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Running %s in %s.", task.Label(), place), nil
}

// started runs the task at of project, and returns it and where it runs:
// its pane, "api › back", or the throwaway window it ran apart in.
func (uc RunTask) started(name domain.ProjectName, at TaskAt) (domain.Task, string, error) {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return domain.Task{}, "", err
	}
	window, pane, task, err := taskOf(project, at)
	if err != nil {
		return domain.Task{}, "", err
	}
	if err := requireOwned(uc.tasks, name); err != nil {
		return domain.Task{}, "", err
	}
	apart, err := uc.run(name, window, at.Pane, pane, task)
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
// whether it did so.
func (uc RunTask) run(name domain.ProjectName, window domain.WindowPlan, index int, pane domain.PanePlan, task domain.Task) (bool, error) {
	busy, err := uc.tasks.Busy(name, window, index)
	if err != nil {
		return false, fmt.Errorf("%s: %w", where(window, pane), err)
	}
	if !busy {
		return false, uc.tasks.Type(name, window, index, task.Command())
	}
	shown := pane.Name().String()
	if pane.Name().IsZero() {
		shown = window.Name().String()
	}
	yes, err := uc.confirm.Confirm(fmt.Sprintf("%s is busy: run %s in a throwaway window?", shown, task.Label()))
	if err != nil {
		return false, err
	}
	if !yes {
		return false, ErrAborted
	}
	return true, uc.tasks.RunApart(name, window, index, task)
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
