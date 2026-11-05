package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// ErrMissingDirectory is returned when a pane's directory doesn't
	// exist, and no repository would be cloned there.
	ErrMissingDirectory = errors.New("directory not found")
	// ErrNoProjectAt is returned when no registered project's root
	// contains a directory.
	ErrNoProjectAt = errors.New("no registered project contains this directory")
	// ErrAmbiguousProject is returned when two registered projects share
	// the root containing a directory.
	ErrAmbiguousProject = errors.New("several projects share this directory's root")
)

// OpenProject puts the user in a project's tmux session, starting it
// first when it isn't running, or re-creating the windows it lacks when
// weaver started it.
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
// the innermost one. Invalid configs are skipped, so one broken project
// doesn't block the others, and named only when no project matches.
func (uc OpenProject) HandleDir(dir string) error {
	names, err := uc.projects.Names()
	if err != nil {
		return err
	}
	var found []domain.Project
	var invalid []error
	for _, name := range names {
		project, err := uc.projects.FindByName(name)
		if errors.Is(err, domain.ErrInvalidConfig) {
			invalid = append(invalid, err)
			continue
		}
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
		err := fmt.Errorf("%w: %s (weave list shows the registered projects)", ErrNoProjectAt, dir)
		if len(invalid) > 0 {
			err = fmt.Errorf("%w; skipped invalid configs:\n%w", err, errors.Join(invalid...))
		}
		return err
	case 1:
		return uc.open(found[0])
	}
	return fmt.Errorf("%w: %s and %s are both rooted at %s", ErrAmbiguousProject,
		found[0].Name(), found[1].Name(), found[0].Root())
}

func (uc OpenProject) open(project domain.Project) error {
	session, running, err := uc.sessions.Find(project.Name())
	if err != nil {
		return err
	}
	switch {
	case !running:
		err = uc.start(project)
	case session.Owned():
		err = uc.restore(project, session)
	}
	if err != nil {
		return err
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

// restore re-creates the project windows missing from a session weaver
// started, usually because they were closed. Sessions started otherwise
// are left as they are: their windows aren't weaver's to complete.
func (uc OpenProject) restore(project domain.Project, session domain.RunningSession) error {
	placements, err := project.Restoration(session.Windows(), uc.runtimes)
	if err != nil || len(placements) == 0 {
		return err
	}
	for _, placement := range placements {
		if err := uc.checkWindowDirs(placement.Window()); err != nil {
			return err
		}
	}
	return uc.sessions.Restore(project.Name(), placements)
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
		if err := uc.checkWindowDirs(window); err != nil {
			return err
		}
	}
	return nil
}

func (uc OpenProject) checkWindowDirs(window domain.WindowPlan) error {
	for _, pane := range window.Panes() {
		exists, err := uc.workspace.Exists(pane.Dir())
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%s: %w: %s", where(window, pane), ErrMissingDirectory, pane.Dir())
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
