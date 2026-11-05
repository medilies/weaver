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
	return uc.open(project)
}

// HandleDir opens the project whose root contains dir; when roots nest,
// the innermost one.
func (uc OpenProject) HandleDir(dir string) error {
	names, err := uc.projects.Names()
	if err != nil {
		return err
	}
	var found []domain.Project
	for _, name := range names {
		project, err := uc.projects.FindByName(name)
		if err != nil {
			return err
		}
		if !project.Root().Contains(dir) {
			continue
		}
		switch {
		case len(found) == 0 || len(project.Root().String()) > len(found[0].Root().String()):
			found = []domain.Project{project}
		case len(project.Root().String()) == len(found[0].Root().String()):
			found = append(found, project)
		}
	}
	switch len(found) {
	case 0:
		return fmt.Errorf("%w: %s (weaver list shows the registered projects)", domain.ErrNoProjectAt, dir)
	case 1:
		return uc.open(found[0])
	}
	return fmt.Errorf("%w: %s and %s are both rooted at %s", domain.ErrAmbiguousProject,
		found[0].Name(), found[1].Name(), found[0].Root())
}

func (uc OpenProject) open(project domain.Project) error {
	running, err := uc.sessions.IsRunning(project.Name())
	if err != nil {
		return err
	}
	if !running {
		if err := uc.start(project); err != nil {
			return err
		}
	}
	return uc.sessions.Attach(project.Name())
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
