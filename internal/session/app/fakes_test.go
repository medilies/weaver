package app_test

import (
	"fmt"
	"maps"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// inMemoryRegistry is a fake domain.ProjectRegistry, so these tests
// never touch disk.
type inMemoryRegistry struct {
	projects map[string]domain.Project
}

func (r inMemoryRegistry) FindByName(name domain.ProjectName) (domain.Project, error) {
	project, ok := r.projects[name.String()]
	if !ok {
		return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return project, nil
}

func (r inMemoryRegistry) Save(project domain.Project) error {
	r.projects[project.Name().String()] = project
	return nil
}

func (r inMemoryRegistry) Delete(name domain.ProjectName) error {
	if _, ok := r.projects[name.String()]; !ok {
		return fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	delete(r.projects, name.String())
	return nil
}

func (r inMemoryRegistry) Names() ([]domain.ProjectName, error) {
	var names []domain.ProjectName
	for _, key := range slices.Sorted(maps.Keys(r.projects)) {
		names = append(names, r.projects[key].Name())
	}
	return names, nil
}
