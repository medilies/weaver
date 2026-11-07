package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// ErrMissingDirectory is returned when a pane's directory doesn't
	// exist.
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
	projects  domain.ProjectRegistry
	sessions  domain.SessionLauncher
	workspace domain.Workspace
}

// NewOpenProject constructs the use case against its ports.
func NewOpenProject(
	projects domain.ProjectRegistry,
	sessions domain.SessionLauncher,
	workspace domain.Workspace,
) OpenProject {
	return OpenProject{projects: projects, sessions: sessions, workspace: workspace}
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
// the innermost one. Roots and dir are compared with their symlinks
// resolved, as they are on disk, and as written only when no root
// contains dir that way, as when dir is reached through a link inside a
// root that leads out of every root. So a link inside one root that leads
// into another opens the other. Invalid configs are skipped, so one
// broken project doesn't block the others, and named only when no
// project matches.
func (uc OpenProject) HandleDir(dir string) error {
	names, err := uc.projects.Names()
	if err != nil {
		return err
	}
	realDir := uc.workspace.RealPath(dir)
	var resolved, written []rootMatch
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
		root := project.Root()
		resolvedRoot, err := domain.NewProjectRoot(uc.workspace.RealPath(root.String()))
		switch {
		case err == nil && resolvedRoot.Contains(realDir):
			resolved = append(resolved, rootMatch{project: project, root: resolvedRoot.String()})
		case root.Contains(dir):
			written = append(written, rootMatch{project: project, root: root.String()})
		}
	}
	if len(resolved) == 0 {
		resolved = written
	}
	found := innermost(resolved)
	switch len(found) {
	case 0:
		err := fmt.Errorf("%w: %s (weave list shows the registered projects)", ErrNoProjectAt, dir)
		if len(invalid) > 0 {
			err = fmt.Errorf("%w; skipped invalid configs:\n%w", err, errors.Join(invalid...))
		}
		return err
	case 1:
		return uc.open(found[0].project)
	}
	first, second := found[0].project, found[1].project
	if first.Root().String() != second.Root().String() {
		return fmt.Errorf("%w: %s, rooted at %s, and %s, rooted at %s, share a root", ErrAmbiguousProject,
			first.Name(), first.Root(), second.Name(), second.Root())
	}
	return fmt.Errorf("%w: %s and %s are both rooted at %s", ErrAmbiguousProject, first.Name(), second.Name(), first.Root())
}

// rootMatch is a project whose root, in one form, contains a directory.
type rootMatch struct {
	project domain.Project
	root    string
}

// innermost are the matches with the longest root. Roots that contain the
// same directory, in the same form, nest, so the longest is the innermost,
// and roots as long are the same directory.
func innermost(matches []rootMatch) []rootMatch {
	var found []rootMatch
	for _, m := range matches {
		switch {
		case len(found) == 0 || len(m.root) > len(found[0].root):
			found = []rootMatch{m}
		case len(m.root) == len(found[0].root):
			found = append(found, m)
		}
	}
	return found
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

// start checks pane directories exist before changing anything.
func (uc OpenProject) start(project domain.Project) error {
	plan := project.Plan()
	if err := uc.checkDirs(plan); err != nil {
		return err
	}
	return uc.sessions.Start(plan)
}

// restore re-creates the project windows missing from a session weaver
// started, usually because they were closed. Sessions started otherwise
// are left as they are: their windows aren't weaver's to complete.
func (uc OpenProject) restore(project domain.Project, session domain.RunningSession) error {
	placements := project.Restoration(session.Windows())
	if len(placements) == 0 {
		return nil
	}
	for _, placement := range placements {
		if err := uc.checkWindowDirs(placement.Window()); err != nil {
			return err
		}
	}
	return uc.sessions.Restore(project.Name(), placements)
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
