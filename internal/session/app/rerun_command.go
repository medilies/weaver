package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrNothingToRerun is returned when a pane has no command that exited:
// it has none, prefills it, or its command is running.
var ErrNothingToRerun = errors.New("nothing to rerun")

// PaneAt is a pane by where it is: its window and its index in that
// window, from 0.
type PaneAt struct {
	Window domain.WindowName
	Pane   int
}

// RerunCommand types a pane's command into it again once it exited, as a
// task is typed, in its project's running session, then puts the user in
// that pane.
type RerunCommand struct {
	projects domain.ProjectRegistry
	tasks    domain.TaskRunner
}

// NewRerunCommand constructs the use case against its ports.
func NewRerunCommand(projects domain.ProjectRegistry, tasks domain.TaskRunner) RerunCommand {
	return RerunCommand{projects: projects, tasks: tasks}
}

// Handle reruns the command of the pane at of project, and puts the user
// in that pane. A project that isn't running is refused, with an error
// wrapping ErrSessionNotRunning, and one whose session weaver didn't
// start with ErrSessionNotOwned; a pane without a command run, or whose
// command is running, with ErrNothingToRerun.
func (uc RerunCommand) Handle(name domain.ProjectName, at PaneAt) error {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return err
	}
	window, pane, err := paneOf(project, at)
	if err != nil {
		return err
	}
	cmd, ok := pane.Startup().Command()
	if !ok || !pane.Startup().Submits() {
		return fmt.Errorf("%w: %s has no command", ErrNothingToRerun, where(window, pane))
	}
	if err := requireOwned(uc.tasks, name); err != nil {
		return err
	}
	busy, err := uc.tasks.Busy(name, window, at.Pane)
	if err != nil {
		return fmt.Errorf("%s: %w", where(window, pane), err)
	}
	if busy {
		return fmt.Errorf("%w: the command of %s is running", ErrNothingToRerun, where(window, pane))
	}
	if err := uc.tasks.Type(name, window, at.Pane, cmd); err != nil {
		return err
	}
	if err := uc.tasks.Attach(name); err != nil {
		return fmt.Errorf("%s rerun, but: %w", cmd, err)
	}
	return nil
}

// paneOf finds the pane at of project, and its window, as the project
// opens them.
func paneOf(project domain.Project, at PaneAt) (domain.WindowPlan, domain.PanePlan, error) {
	for _, window := range project.Plan().Windows() {
		if window.Name() != at.Window {
			continue
		}
		panes := window.Panes()
		if at.Pane < 0 || at.Pane >= len(panes) {
			return domain.WindowPlan{}, domain.PanePlan{}, fmt.Errorf("window %q: %w: #%d", at.Window, domain.ErrPaneNotFound, at.Pane+1)
		}
		return window, panes[at.Pane], nil
	}
	return domain.WindowPlan{}, domain.PanePlan{}, fmt.Errorf("%w: %q", domain.ErrWindowNotFound, at.Window)
}

// requireOwned returns an error wrapping ErrSessionNotRunning unless the
// project's session runs, and ErrSessionNotOwned unless weaver started
// it, as only then can its panes be told apart.
func requireOwned(sessions domain.SessionFinder, name domain.ProjectName) error {
	session, running, err := sessions.Find(name)
	switch {
	case err != nil:
		return err
	case !running:
		return fmt.Errorf("%s: %w: open it first", name, domain.ErrSessionNotRunning)
	case !session.Owned():
		return fmt.Errorf("%s: %w: its panes can't be told apart", name, domain.ErrSessionNotOwned)
	}
	return nil
}
