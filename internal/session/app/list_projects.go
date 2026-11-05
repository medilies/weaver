package app

import "github.com/medilies/weaver/internal/session/domain"

// ProjectStatus is a registered project and whether its session runs.
type ProjectStatus struct {
	name    domain.ProjectName
	running bool
}

func (s ProjectStatus) Name() domain.ProjectName {
	return s.name
}

// Running reports whether the project's session runs.
func (s ProjectStatus) Running() bool {
	return s.running
}

// ListProjects lists the registered projects and whether their sessions
// run.
type ListProjects struct {
	projects domain.ProjectRepository
	sessions domain.SessionLister
}

// NewListProjects constructs the use case against its ports.
func NewListProjects(projects domain.ProjectRepository, sessions domain.SessionLister) ListProjects {
	return ListProjects{projects: projects, sessions: sessions}
}

// Handle lists the projects sorted by name.
func (uc ListProjects) Handle() ([]ProjectStatus, error) {
	names, err := uc.projects.Names()
	if err != nil {
		return nil, err
	}
	running, err := uc.sessions.Running()
	if err != nil {
		return nil, err
	}
	sessions := make(map[string]bool, len(running))
	for _, session := range running {
		sessions[session] = true
	}
	statuses := make([]ProjectStatus, 0, len(names))
	for _, name := range names {
		statuses = append(statuses, ProjectStatus{name: name, running: sessions[name.String()]})
	}
	return statuses, nil
}
