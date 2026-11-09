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
// throwaway window.
type RunTask struct {
	projects domain.ProjectRegistry
	tasks    domain.TaskRunner
	confirm  Confirmer
}

// NewRunTask constructs the use case against its ports.
func NewRunTask(projects domain.ProjectRegistry, tasks domain.TaskRunner, confirm Confirmer) RunTask {
	return RunTask{projects: projects, tasks: tasks, confirm: confirm}
}

// Handle runs the task at of project, and says where it runs. A project
// that isn't running is refused, with an error wrapping
// ErrSessionNotRunning, and one whose session weaver didn't start, whose
// panes it can't tell, with ErrSessionNotOwned; declining to run the
// task apart is ErrAborted.
func (uc RunTask) Handle(name domain.ProjectName, at TaskAt) (string, error) {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return "", err
	}
	window, pane, task, err := taskOf(project, at)
	if err != nil {
		return "", err
	}
	session, running, err := uc.tasks.Find(name)
	switch {
	case err != nil:
		return "", err
	case !running:
		return "", fmt.Errorf("%s: %w: open it first", name, domain.ErrSessionNotRunning)
	case !session.Owned():
		return "", fmt.Errorf("%s: %w: its panes can't be told apart", name, domain.ErrSessionNotOwned)
	}
	shown := pane.Name().String()
	if pane.Name().IsZero() {
		shown = window.Name().String()
	}
	busy, err := uc.tasks.Busy(name, window, at.Pane)
	if err != nil {
		return "", fmt.Errorf("%s: %w", where(window, pane), err)
	}
	if !busy {
		if err := uc.tasks.Type(name, window, at.Pane, task); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s running in %s.", task.Label(), shown), nil
	}
	yes, err := uc.confirm.Confirm(fmt.Sprintf("%s is busy: run %s in a throwaway window?", shown, task.Label()))
	if err != nil {
		return "", err
	}
	if !yes {
		return "", ErrAborted
	}
	if err := uc.tasks.RunApart(name, pane, task); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s running in the throwaway window %s.", task.Label(), task.ThrowawayWindow()), nil
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
