package app

import (
	"slices"
	"time"

	"github.com/medilies/weaver/internal/session/domain"
)

// ProjectStatus is a registered project and whether its session runs.
type ProjectStatus struct {
	project domain.ProjectRef
	running bool
	active  time.Time
}

func (s ProjectStatus) Name() domain.ProjectName {
	return s.project.Name()
}

// Root is the project's root, which identifies it.
func (s ProjectStatus) Root() domain.ProjectRoot {
	return s.project.Root()
}

// Ref is how the session ports tell the project apart.
func (s ProjectStatus) Ref() domain.ProjectRef {
	return s.project
}

// Running reports whether the project's session runs.
func (s ProjectStatus) Running() bool {
	return s.running
}

// ListProjects lists the registered projects and whether their sessions
// run.
type ListProjects struct {
	projects domain.ProjectRegistry
	sessions domain.SessionLister
}

// NewListProjects constructs the use case against its ports.
func NewListProjects(projects domain.ProjectRegistry, sessions domain.SessionLister) ListProjects {
	return ListProjects{projects: projects, sessions: sessions}
}

// Handle lists the projects: those running first, the one last attached
// or started at the top, then the others in the settings' order.
func (uc ListProjects) Handle() ([]ProjectStatus, error) {
	refs, err := uc.projects.Registered()
	if err != nil {
		return nil, err
	}
	sessions, err := uc.sessions.Running()
	if err != nil {
		return nil, err
	}
	statuses := make([]ProjectStatus, 0, len(refs))
	for _, ref := range refs {
		session, running := ref.SessionAmong(sessions)
		statuses = append(statuses, ProjectStatus{project: ref, running: running, active: session.Active()})
	}
	slices.SortStableFunc(statuses, func(a, b ProjectStatus) int {
		switch {
		case a.running != b.running && a.running:
			return -1
		case a.running != b.running:
			return 1
		case a.running:
			return b.active.Compare(a.active)
		}
		return 0
	})
	return statuses, nil
}
