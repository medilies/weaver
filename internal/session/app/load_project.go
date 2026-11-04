// Package app holds the Session bounded context's use cases. It depends
// only on the domain package, never on infrastructure or the CLI.
package app

import "github.com/medilies/weaver/internal/session/domain"

// LoadProject looks up a registered project by name.
type LoadProject struct {
	repo domain.ProjectRepository
}

// NewLoadProject constructs the use case against a repository port.
func NewLoadProject(repo domain.ProjectRepository) LoadProject {
	return LoadProject{repo: repo}
}

// Handle resolves the project for the given name.
func (uc LoadProject) Handle(rawName string) (domain.Project, error) {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return domain.Project{}, err
	}
	return uc.repo.FindByName(name)
}
