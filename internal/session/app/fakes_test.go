package app_test

import (
	"fmt"
	"maps"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// inMemoryRepository is a fake domain.ProjectRepository, so these tests
// never touch disk.
type inMemoryRepository struct {
	projects map[string]domain.Project
}

func (r inMemoryRepository) FindByName(name domain.ProjectName) (domain.Project, error) {
	project, ok := r.projects[name.String()]
	if !ok {
		return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return project, nil
}

func (r inMemoryRepository) Save(project domain.Project) error {
	r.projects[project.Name().String()] = project
	return nil
}

func (r inMemoryRepository) Delete(name domain.ProjectName) error {
	if _, ok := r.projects[name.String()]; !ok {
		return fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	delete(r.projects, name.String())
	return nil
}

func (r inMemoryRepository) Names() ([]domain.ProjectName, error) {
	var names []domain.ProjectName
	for _, key := range slices.Sorted(maps.Keys(r.projects)) {
		names = append(names, r.projects[key].Name())
	}
	return names, nil
}
