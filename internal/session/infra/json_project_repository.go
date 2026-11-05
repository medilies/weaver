// Package infra adapts the Session bounded context to the outside world:
// project configs stored as JSON files, the user's tmux server, git, and
// the toolchains installed on the machine.
package infra

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// Names implements domain.ProjectRepository. It reads file names only:
// "<name>.json" files whose stem isn't a valid project name are skipped,
// and a missing directory has no projects.
func (r JSONProjectRepository) Names() ([]domain.ProjectName, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []domain.ProjectName
	for _, entry := range entries {
		stem, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if !isJSON || entry.IsDir() {
			continue
		}
		if name, err := domain.NewProjectName(stem); err == nil {
			names = append(names, name)
		}
	}
	return names, nil
}

// FindByName implements domain.ProjectRepository.
func (r JSONProjectRepository) FindByName(name domain.ProjectName) (domain.Project, error) {
	path := r.path(name)

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

// Save implements domain.ProjectRepository. It also saves the JSON Schema
// next to the configs, which they point to so editors complete and check
// them.
func (r JSONProjectRepository) Save(project domain.Project) error {
	doc, err := encodeProject(project)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	if current, err := os.ReadFile(filepath.Join(r.dir, schemaFileName)); err != nil || !bytes.Equal(current, schema) {
		if err := writeFileAtomic(filepath.Join(r.dir, schemaFileName), schema); err != nil {
			return err
		}
	}
	return writeFileAtomic(r.path(project.Name()), doc)
}

// path is where the config of project name lives.
func (r JSONProjectRepository) path(name domain.ProjectName) string {
	return filepath.Join(r.dir, name.String()+".json")
}

// writeFileAtomic replaces path with data, readable by its owner only,
// so a reader never sees half a file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
