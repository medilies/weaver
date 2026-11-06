package app

import (
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// KillProject kills a project's tmux session and everything running in
// it.
type KillProject struct {
	sessions domain.SessionKiller
	locator  domain.SessionLocator
}

// NewKillProject constructs the use case against its ports.
func NewKillProject(sessions domain.SessionKiller, locator domain.SessionLocator) KillProject {
	return KillProject{sessions: sessions, locator: locator}
}

// Handle kills the session of the project named rawName.
func (uc KillProject) Handle(rawName string) error {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return err
	}
	return uc.sessions.Kill(name)
}

// HandleCurrent kills the session weaver runs in, or returns
// domain.ErrNotInsideTmux. A session no project could be named after is
// left running.
func (uc KillProject) HandleCurrent() error {
	current, err := uc.locator.Current()
	if err != nil {
		return err
	}
	name, err := domain.NewProjectName(current)
	if err != nil {
		return fmt.Errorf("the session you're in, %q, isn't a project's: %w", current, err)
	}
	return uc.sessions.Kill(name)
}
