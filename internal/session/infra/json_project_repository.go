// Package infra adapts the Session bounded context to the outside world:
// today, project configs stored as JSON files on disk.
package infra

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/domain"
)

// JSONProjectRepository reads project configs from a directory of
// "<name>.json" files, in the format of projectFile.
type JSONProjectRepository struct {
	dir string
}

// NewJSONProjectRepository constructs a repository rooted at dir.
func NewJSONProjectRepository(dir string) JSONProjectRepository {
	return JSONProjectRepository{dir: dir}
}

// FindByName implements domain.ProjectRepository.
func (r JSONProjectRepository) FindByName(name domain.ProjectName) (domain.Project, error) {
	path := filepath.Join(r.dir, name.String()+".json")

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Project{}, domain.NewProjectNotFound(name)
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("reading %s: %w", path, err)
	}

	project, err := decodeProject(name, raw)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w", path, err)
	}
	return project, nil
}
