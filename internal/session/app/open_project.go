package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrMissingDirectory is returned when a pane's directory doesn't exist.
var ErrMissingDirectory = errors.New("directory not found")

// OpenProject puts the user in a project's tmux session, creating it
// first, after its setup commands, when it isn't running, or re-creating
// the windows it lacks when weaver started it. Started, it does the same
// but leaves the user where they are.
type OpenProject struct {
	projects  domain.ProjectRegistry
	sessions  domain.SessionLauncher
	workspace domain.Workspace
	commands  domain.CommandRunner
}

// NewOpenProject constructs the use case against its ports.
func NewOpenProject(
	projects domain.ProjectRegistry,
	sessions domain.SessionLauncher,
	workspace domain.Workspace,
	commands domain.CommandRunner,
) OpenProject {
	return OpenProject{projects: projects, sessions: sessions, workspace: workspace, commands: commands}
}

// Handle opens the project registered at root.
func (uc OpenProject) Handle(root domain.ProjectRoot) error {
	project, err := uc.projects.Find(root)
	if err != nil {
		return err
	}
	return uc.open(project)
}

// HandleStart starts the project registered at root, and says what it
// did: "Started shop.", "Restored "logs" in shop." or "shop is already
// running."
func (uc OpenProject) HandleStart(root domain.ProjectRoot) (string, error) {
	project, err := uc.projects.Find(root)
	if err != nil {
		return "", err
	}
	return uc.ready(project)
}

func (uc OpenProject) open(project domain.Project) error {
	if _, err := uc.ready(project); err != nil {
		return err
	}
	return uc.sessions.Attach(project.Ref())
}

// ready starts the project's session, or restores the windows its owned
// session lacks, and says what it did.
func (uc OpenProject) ready(project domain.Project) (string, error) {
	name := project.Name()
	session, running, err := uc.sessions.Find(project.Ref())
	switch {
	case err != nil:
		return "", err
	case !running:
		if err := uc.start(project); err != nil {
			return "", err
		}
		return fmt.Sprintf("Started %s.", name), nil
	case session.Owned():
		restored, err := uc.restore(project, session)
		switch {
		case err != nil:
			return "", err
		case len(restored) == 0:
			return fmt.Sprintf("%s is already running.", name), nil
		}
		return fmt.Sprintf("Restored %s in %s.", strings.Join(restored, ", "), name), nil
	}
	return fmt.Sprintf("%s is already running.", name), nil
}

// start runs the setup commands in the root, which must exist, then
// checks pane directories exist, which a setup command may create, before
// building the session. A failing setup command stops it, with no session
// built. What setup commands did is left for the user to undo when a
// later step fails: with no session, there is nothing to kill, so no
// teardown.
func (uc OpenProject) start(project domain.Project) error {
	setup := project.Setup()
	if len(setup) > 0 {
		root := project.Root().String()
		exists, err := uc.workspace.Exists(root)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("root: %w: %s (create it, or edit %s with e in weave)", ErrMissingDirectory, root, project.Name())
		}
	}
	for _, cmd := range setup {
		if err := uc.commands.Run(cmd, project.Root().String(), project.Env()); err != nil {
			return fmt.Errorf("setup: %s: %w; %s's session wasn't started", cmd, err, project.Name())
		}
	}
	plan := project.Plan()
	err := uc.checkDirs(project.Name(), plan)
	if err == nil {
		err = uc.sessions.Start(plan)
	}
	if err != nil && len(setup) > 0 {
		return fmt.Errorf("%w; the setup commands already ran, and no teardown will: undo them yourself if needed", err)
	}
	return err
}

// restore re-creates the project windows missing from a session weaver
// started, usually because they were closed. Sessions started otherwise
// are left as they are: their windows aren't weaver's to complete.
// It returns the restored windows' names, quoted.
func (uc OpenProject) restore(project domain.Project, session domain.RunningSession) ([]string, error) {
	placements := project.Restoration(session.Windows())
	if len(placements) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(placements))
	for _, placement := range placements {
		if err := uc.checkWindowDirs(project.Name(), placement.Window()); err != nil {
			return nil, err
		}
		names = append(names, fmt.Sprintf("%q", placement.Window().Name().String()))
	}
	return names, uc.sessions.Restore(project.Ref(), placements)
}

func (uc OpenProject) checkDirs(project domain.ProjectName, plan domain.SessionPlan) error {
	for _, window := range plan.Windows() {
		if err := uc.checkWindowDirs(project, window); err != nil {
			return err
		}
	}
	return nil
}

// checkWindowDirs checks the window's pane directories exist. Its error
// says how to fix a missing one, since weaver creates none.
func (uc OpenProject) checkWindowDirs(project domain.ProjectName, window domain.WindowPlan) error {
	for _, pane := range window.Panes() {
		exists, err := uc.workspace.Exists(pane.Dir())
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%s: %w: %s (create it, or edit %s with e in weave)",
				where(window, pane), ErrMissingDirectory, pane.Dir(), project)
		}
	}
	return nil
}

// where names a pane in errors.
func where(window domain.WindowPlan, pane domain.PanePlan) string {
	if pane.Name().IsZero() {
		return fmt.Sprintf("window %q", window.Name())
	}
	return fmt.Sprintf("window %q, pane %q", window.Name(), pane.Name())
}
