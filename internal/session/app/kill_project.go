package app

import (
	"errors"
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
// domain.ErrNotInsideTmux. Only a session weaver started is killed: one
// that merely has a project's name, as tmux's own "0", is left running,
// and Handle kills it when named.
func (uc KillProject) HandleCurrent() error {
	current, err := uc.locator.Current()
	if errors.Is(err, domain.ErrNotInsideTmux) {
		return fmt.Errorf("%w; name a project", err)
	}
	if err != nil {
		return err
	}
	name, err := domain.NewProjectName(current)
	if err != nil {
		return fmt.Errorf("the session you're in, %q, isn't a project's: %w", current, err)
	}
	session, running, err := uc.sessions.Find(name)
	if err != nil {
		return err
	}
	if running && !session.Owned() {
		return fmt.Errorf("%w: %s; run weave kill %s to kill it anyway", domain.ErrSessionNotOwned, name, name)
	}
	return uc.sessions.Kill(name)
}
