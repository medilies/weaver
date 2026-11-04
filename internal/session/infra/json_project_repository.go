// Package infra adapts the Session bounded context to the outside world:
// today, project configs stored as JSON files on disk.
package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/domain"
)

// projectFile is the on-disk shape of a project config, e.g.
// ~/.config/weaver/projects/<name>.json:
//
//	{"root": "/home/ilies/dev/weaver"}
type projectFile struct {
	Root string `json:"root"`
}

// JSONProjectRepository reads project configs from a directory of
// "<name>.json" files.
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

	var file projectFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return domain.Project{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	rootPath, err := expandHome(file.Root)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w", path, err)
	}
	root, err := domain.NewProjectRoot(rootPath)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w", path, err)
	}

	return domain.NewProject(name, root), nil
}
