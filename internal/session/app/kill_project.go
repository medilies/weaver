package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// KillProject kills a project's tmux session and everything running in
// it, after its teardown commands when weaver started it.
type KillProject struct {
	sessions domain.SessionKiller
	locator  domain.SessionLocator
	projects domain.ProjectRegistry
	commands domain.CommandRunner
}

// NewKillProject constructs the use case against its ports.
func NewKillProject(
	sessions domain.SessionKiller,
	locator domain.SessionLocator,
	projects domain.ProjectRegistry,
	commands domain.CommandRunner,
) KillProject {
	return KillProject{sessions: sessions, locator: locator, projects: projects, commands: commands}
}

// Handle kills the session of the project named rawName, weaver's or
// not.
func (uc KillProject) Handle(rawName string) error {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return err
	}
	session, running, err := uc.sessions.Find(name)
	if err != nil {
		return err
	}
	return uc.kill(name, running && session.Owned())
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
	return uc.kill(name, running)
}

// kill kills the project's session, first running its teardown commands
// when owned, since only a session weaver started had its setup commands
// run. The session is killed even when the teardown fails, which is then
// reported.
func (uc KillProject) kill(name domain.ProjectName, owned bool) error {
	var teardown error
	if owned {
		teardown = uc.tearDown(name)
	}
	if err := uc.sessions.Kill(name); err != nil {
		return errors.Join(err, teardown)
	}
	if teardown != nil {
		return fmt.Errorf("%w; %s's session was killed anyway", teardown, name)
	}
	return nil
}

// tearDown runs the project's teardown commands in its root, in order,
// stopping at a failing one. A project without a config has none, as when
// it was deleted while running; one whose config can't be read is
// reported.
func (uc KillProject) tearDown(name domain.ProjectName) error {
	project, err := uc.projects.FindByName(name)
	if errors.Is(err, domain.ErrProjectNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("teardown skipped: %w", err)
	}
	for _, cmd := range project.Teardown() {
		if err := uc.commands.Run(cmd, project.Root().String(), project.Env()); err != nil {
			return fmt.Errorf("teardown: %s: %w", cmd, err)
		}
	}
	return nil
}
