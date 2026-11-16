package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/medilies/weaver/internal/session/domain"
)

// interruptGrace is how long a kill waits for the panes it interrupted to
// be back at their prompts.
const interruptGrace = 5 * time.Second

// KillProject kills a project's tmux session and everything running in
// it; first, when weaver started it, it interrupts its panes, then runs
// its teardown commands.
type KillProject struct {
	sessions     domain.SessionKiller
	locator      domain.SessionLocator
	projects     domain.ProjectRegistry
	commands     domain.CommandRunner
	acknowledger Acknowledger
}

// NewKillProject constructs the use case against its ports.
func NewKillProject(
	sessions domain.SessionKiller,
	locator domain.SessionLocator,
	projects domain.ProjectRegistry,
	commands domain.CommandRunner,
	acknowledger Acknowledger,
) KillProject {
	return KillProject{sessions: sessions, locator: locator, projects: projects, commands: commands, acknowledger: acknowledger}
}

// Handle kills the session of the project registered at root, weaver's
// or not. It says which panes still ran once it gave up waiting on them,
// as in "api › logs still ran; killed anyway.", and nothing otherwise.
func (uc KillProject) Handle(root domain.ProjectRoot) (string, error) {
	return uc.HandleShowing(root, func(string) {})
}

// HandleShowing is Handle, telling working what it does meanwhile, as in
// "Interrupting shop's panes…".
func (uc KillProject) HandleShowing(root domain.ProjectRoot, working func(text string)) (string, error) {
	project, err := registeredAt(uc.projects, root)
	if err != nil {
		return "", err
	}
	session, running, err := uc.sessions.Find(project)
	if err != nil {
		return "", err
	}
	if !running {
		return "", fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	return uc.kill(project, session, working)
}

// HandleCurrent kills the session weaver runs in, or returns
// domain.ErrNotInsideTmux. Only a session weaver started is killed, its
// project's or one whose project was unregistered since: one that merely
// counts as a project's, started by hand, is left running, and Handle
// kills it when its project is named. Weaver's own pane goes with the
// session, so what still ran isn't told.
func (uc KillProject) HandleCurrent() error {
	current, err := uc.locator.Current()
	if errors.Is(err, domain.ErrNotInsideTmux) {
		return fmt.Errorf("%w; name a project", err)
	}
	if err != nil {
		return err
	}
	sessions, err := uc.sessions.Running()
	if err != nil {
		return err
	}
	var listed domain.ListedSession
	for _, s := range sessions {
		if s.Name() == current {
			listed = s
		}
	}
	if !listed.Owned() {
		return fmt.Errorf("%w: %s; name its project to kill it anyway", domain.ErrSessionNotOwned, current)
	}
	root, err := domain.NewProjectRoot(listed.Root())
	if err != nil {
		return err
	}
	project, err := registeredAt(uc.projects, root)
	if errors.Is(err, domain.ErrProjectNotFound) {
		name, nameErr := domain.NewProjectName(filepath.Base(root.String()))
		if nameErr != nil {
			return nameErr
		}
		project, err = domain.NewProjectRef(root, name), nil
	}
	if err != nil {
		return err
	}
	session, running, err := uc.sessions.Find(project)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	_, err = uc.kill(project, session, func(string) {})
	return err
}

// registeredAt is the project registered at root, its config read or
// not, or an error wrapping domain.ErrProjectNotFound.
func registeredAt(projects domain.ProjectRegistry, root domain.ProjectRoot) (domain.ProjectRef, error) {
	refs, err := projects.Registered()
	if err != nil {
		return domain.ProjectRef{}, err
	}
	for _, ref := range refs {
		if ref.Root() == root {
			return ref, nil
		}
	}
	return domain.ProjectRef{}, fmt.Errorf("%w: nothing registered at %s", domain.ErrProjectNotFound, root)
}

// kill kills the project's session. When owned, it first interrupts its
// panes, so that what runs in them can clean up, and then runs its
// teardown commands, since only a session weaver started had its setup
// commands run. The session is killed even when interrupting or the
// teardown fails, which is then reported: first to the user, who reads it
// before the kill, when weaver runs inside that session, since its pane
// goes with it.
func (uc KillProject) kill(name domain.ProjectRef, session domain.RunningSession, working func(string)) (string, error) {
	var lingering []string
	var teardown error
	if session.Owned() {
		working(fmt.Sprintf("Interrupting %s's panes…", name))
		var err error
		lingering, err = uc.sessions.InterruptPanes(name, interruptGrace)
		if err != nil && !errors.Is(err, domain.ErrSessionNotRunning) {
			teardown = fmt.Errorf("interrupting the panes: %w", err)
		}
		working(fmt.Sprintf("Killing %s…", name))
		teardown = errors.Join(teardown, uc.tearDown(name))
	}
	if teardown != nil && uc.inside(session) {
		message := fmt.Sprintf("%v\n\n%s's session will be killed anyway, with this pane.", teardown, name)
		if err := uc.acknowledger.Acknowledge(message); err != nil {
			return "", errors.Join(err, teardown)
		}
	}
	if err := uc.sessions.Kill(name); err != nil {
		return "", errors.Join(err, teardown)
	}
	if teardown != nil {
		return "", fmt.Errorf("%w; %s's session was killed anyway", teardown, name)
	}
	if len(lingering) == 0 {
		return "", nil
	}
	return fmt.Sprintf("%s still ran; killed anyway.", strings.Join(lingering, ", ")), nil
}

// inside reports whether weaver runs in session.
func (uc KillProject) inside(session domain.RunningSession) bool {
	current, err := uc.locator.Current()
	return err == nil && current == session.Name()
}

// tearDown runs the project's teardown commands in its root, in order,
// stopping at a failing one. A project without a config has none, as when
// it was deleted while running; one whose config can't be read is
// reported.
func (uc KillProject) tearDown(name domain.ProjectRef) error {
	project, err := uc.projects.Find(name.Root())
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
