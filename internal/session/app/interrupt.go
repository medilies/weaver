package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrNothingToInterrupt is returned when interrupting a pane whose shell
// idles at its prompt, or a task that doesn't run.
var ErrNothingToInterrupt = errors.New("nothing to interrupt")

// Interrupt types ctrl+c into a pane of a project's owned session, as if
// the user pressed it there: into a task's copy, or into a pane. Never
// into the pane the browser runs in. The user stays where they are.
type Interrupt struct {
	projects domain.ProjectRegistry
	tasks    domain.TaskRunner
}

// NewInterrupt constructs the use case against its ports.
func NewInterrupt(projects domain.ProjectRegistry, tasks domain.TaskRunner) Interrupt {
	return Interrupt{projects: projects, tasks: tasks}
}

// HandleTask interrupts one copy of the task at of project, the one going
// there would reach: its pane when the task runs there, else its most
// recent copy; so that pressing it again interrupts the next. It says
// what it interrupted: "Interrupted wait in api › logs.". A task that
// doesn't run is ErrNothingToInterrupt; a project that isn't running
// wraps ErrSessionNotRunning, and one weaver didn't start
// ErrSessionNotOwned.
func (uc Interrupt) HandleTask(name domain.ProjectRef, at TaskAt) (string, error) {
	project, err := uc.projects.Find(name.Root())
	if err != nil {
		return "", err
	}
	window, pane, task, err := taskOf(project, at)
	if err != nil {
		return "", err
	}
	if err := requireOwned(uc.tasks, name); err != nil {
		return "", err
	}
	copies, err := copiesOf(uc.tasks, name, window, at.Pane, task.Command())
	if err != nil {
		return "", err
	}
	if len(copies) == 0 {
		return "", fmt.Errorf("%w: %s isn't running", ErrNothingToInterrupt, task.Label())
	}
	if err := uc.tasks.InterruptRun(name, copies[0]); err != nil {
		return "", fmt.Errorf("%s: %w", task.Label(), err)
	}
	place := shownPane(window, pane)
	if copies[0].Apart() {
		place = "a throwaway window"
	}
	notice := fmt.Sprintf("Interrupted %s in %s.", task.Label(), place)
	switch more := len(copies) - 1; more {
	case 0:
	case 1:
		notice += " 1 more copy runs."
	default:
		notice += fmt.Sprintf(" %d more copies run.", more)
	}
	return notice, nil
}

// HandlePane interrupts what runs in the pane at of project, its command
// included, and says so: "Interrupted api › logs.". A pane whose shell
// idles is ErrNothingToInterrupt, and the pane the browser runs in
// ErrPaneRunsWeaver; a project that isn't running wraps
// ErrSessionNotRunning, and one weaver didn't start ErrSessionNotOwned.
func (uc Interrupt) HandlePane(name domain.ProjectRef, at PaneAt) (string, error) {
	project, err := uc.projects.Find(name.Root())
	if err != nil {
		return "", err
	}
	window, pane, err := paneOf(project, at)
	if err != nil {
		return "", err
	}
	if err := requireOwned(uc.tasks, name); err != nil {
		return "", err
	}
	shown := shownPane(window, pane)
	busy, err := uc.tasks.Busy(name, window, at.Pane)
	if errors.Is(err, domain.ErrPaneRunsWeaver) {
		return "", fmt.Errorf("%s runs this browser: %w", shown, err)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", where(window, pane), err)
	}
	if !busy {
		return "", fmt.Errorf("%w: %s idles at its prompt", ErrNothingToInterrupt, shown)
	}
	if err := uc.tasks.Interrupt(name, window, at.Pane); err != nil {
		return "", fmt.Errorf("%s: %w", where(window, pane), err)
	}
	return fmt.Sprintf("Interrupted %s.", shown), nil
}

// shownPane is pane of window as the browser shows it: "api › logs", or
// its window's name alone for an unnamed pane.
func shownPane(window domain.WindowPlan, pane domain.PanePlan) string {
	if pane.Name().IsZero() {
		return window.Name().String()
	}
	return window.Name().String() + " › " + pane.Name().String()
}
