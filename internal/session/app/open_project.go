package app

import (
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// OpenProject puts the user in a project's tmux session, starting it
// first when it isn't running.
type OpenProject struct {
	projects  domain.ProjectRepository
	sessions  domain.SessionLauncher
	workspace domain.Workspace
	runtimes  domain.RuntimeResolver
}

// NewOpenProject constructs the use case against its ports.
func NewOpenProject(
	projects domain.ProjectRepository,
	sessions domain.SessionLauncher,
	workspace domain.Workspace,
	runtimes domain.RuntimeResolver,
) OpenProject {
	return OpenProject{projects: projects, sessions: sessions, workspace: workspace, runtimes: runtimes}
}

// Handle opens the project named rawName.
func (uc OpenProject) Handle(rawName string) error {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return err
	}
	project, err := uc.projects.FindByName(name)
	if err != nil {
		return err
	}
	running, err := uc.sessions.IsRunning(name)
	if err != nil {
		return err
	}
	if !running {
		if err := uc.start(project); err != nil {
			return err
		}
	}
	return uc.sessions.Attach(name)
}

// start checks what can be checked before changing anything: pinned
// versions first, then pane directories once missing repositories are
// cloned.
func (uc OpenProject) start(project domain.Project) error {
	plan, err := project.Plan(uc.runtimes)
	if err != nil {
		return err
	}
	if err := uc.cloneMissing(project); err != nil {
		return err
	}
	if err := uc.checkDirs(plan); err != nil {
		return err
	}
	return uc.sessions.Start(plan)
}

func (uc OpenProject) cloneMissing(project domain.Project) error {
	for _, checkout := range project.Checkouts().List() {
		dir := checkout.Path(project.Root())
		exists, err := uc.workspace.Exists(dir)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := uc.workspace.Clone(checkout, dir); err != nil {
			return err
		}
	}
	return nil
}

func (uc OpenProject) checkDirs(plan domain.SessionPlan) error {
	for _, window := range plan.Windows() {
		for _, pane := range window.Panes() {
			exists, err := uc.workspace.Exists(pane.Dir())
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("%s: %w: %s", where(window, pane), domain.ErrMissingDirectory, pane.Dir())
			}
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
