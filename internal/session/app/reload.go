package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// Reload rebuilds a pane, or a whole window, of a project's owned session
// as its config says now, ending what runs in it: the pane restarted in
// place, the window replaced where it is. A window the session lacks is
// restored, as opening restores it. Never the pane the browser runs in,
// nor its window. The user stays where they are.
type Reload struct {
	projects domain.ProjectRegistry
	sessions domain.SessionReloader
}

// NewReload constructs the use case against its ports.
func NewReload(projects domain.ProjectRegistry, sessions domain.SessionReloader) Reload {
	return Reload{projects: projects, sessions: sessions}
}

// HandlePane restarts the pane at of project, and says so: "Reloaded api
// › back.". A pane the session lacks is refused, pointing to reloading its
// window; a project that isn't running wraps ErrSessionNotRunning, and one
// weaver didn't start ErrSessionNotOwned.
func (uc Reload) HandlePane(name domain.ProjectName, at PaneAt) (string, error) {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return "", err
	}
	window, pane, err := paneOf(project, at)
	if err != nil {
		return "", err
	}
	if err := requireOwned(uc.sessions, name); err != nil {
		return "", err
	}
	shown := shownPane(window, pane)
	err = uc.sessions.ReloadPane(name, window, at.Pane)
	switch {
	case errors.Is(err, domain.ErrPaneRunsWeaver):
		return "", fmt.Errorf("%s runs this browser: %w", shown, err)
	case errors.Is(err, domain.ErrPaneNotRunning):
		return "", fmt.Errorf("%s: %w: reload the %s window to put it back", shown, err, window.Name())
	case err != nil:
		return "", fmt.Errorf("%s: %w", shown, err)
	}
	return fmt.Sprintf("Reloaded %s.", shown), nil
}

// HandleWindow replaces the window named window of project, or restores
// it when the session lacks it, and says which: "Reloaded api." or
// "Restored api.". A project that isn't running wraps
// ErrSessionNotRunning, and one weaver didn't start ErrSessionNotOwned;
// the window the browser runs in ErrPaneRunsWeaver.
func (uc Reload) HandleWindow(name domain.ProjectName, window domain.WindowName) (string, error) {
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return "", err
	}
	var plan domain.WindowPlan
	found := false
	for _, w := range project.Plan().Windows() {
		if w.Name() == window {
			plan, found = w, true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%w: %q", domain.ErrWindowNotFound, window)
	}
	session, err := ownedSession(uc.sessions, name)
	if err != nil {
		return "", err
	}
	for _, placement := range project.Restoration(session.Windows()) {
		if placement.Window().Name() != window {
			continue
		}
		if err := uc.sessions.Restore(name, []domain.WindowPlacement{placement}); err != nil {
			return "", fmt.Errorf("%s: %w", window, err)
		}
		return fmt.Sprintf("Restored %s.", window), nil
	}
	err = uc.sessions.ReloadWindow(name, plan)
	switch {
	case errors.Is(err, domain.ErrPaneRunsWeaver):
		return "", fmt.Errorf("the %s window runs this browser: %w", window, err)
	case err != nil:
		return "", fmt.Errorf("%s: %w", window, err)
	}
	return fmt.Sprintf("Reloaded %s.", window), nil
}
