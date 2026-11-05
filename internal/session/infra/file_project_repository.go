// Package infra adapts the Session bounded context to the outside world:
// project configs stored as JSON or YAML files, the user's tmux server,
// git, and the toolchains installed on the machine.
package infra

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// ErrSeveralConfigs is returned when a project has more than one config
// file, e.g. shop.json and shop.yml: neither can be told to count.
var ErrSeveralConfigs = errors.New("a project must have a single config file")

// configExtensions are the config files' formats; Save writes JSON.
var configExtensions = []string{".json", ".yml", ".yaml"}

// FileProjectRepository reads project configs from a directory of
// "<name>.json", "<name>.yml" or "<name>.yaml" files, in the format of
// projectFile, and saves them as JSON.
type FileProjectRepository struct {
	dir string
}

// NewFileProjectRepository constructs a repository rooted at dir.
func NewFileProjectRepository(dir string) FileProjectRepository {
	return FileProjectRepository{dir: dir}
}

// Names implements domain.ProjectRepository. It reads file names only:
// config files whose stem isn't a valid project name are skipped, and a
// missing directory has no projects.
func (r FileProjectRepository) Names() ([]domain.ProjectName, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []domain.ProjectName
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if entry.IsDir() || !slices.Contains(configExtensions, ext) {
			continue
		}
		if name, err := domain.NewProjectName(strings.TrimSuffix(entry.Name(), ext)); err == nil {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b domain.ProjectName) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(names), nil
}

// FindByName implements domain.ProjectRepository.
func (r FileProjectRepository) FindByName(name domain.ProjectName) (domain.Project, error) {
	path, err := r.find(name)
	if err != nil {
		return domain.Project{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Project{}, fmt.Errorf("reading %s: %w", path, err)
	}
	project, err := decodeProject(path, name, raw)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w: %w", path, domain.ErrInvalidConfig, err)
	}
	return project, nil
}

// Document implements app.ProjectDocuments.
func (r FileProjectRepository) Document(name domain.ProjectName) (app.ConfigDocument, error) {
	path, err := r.find(name)
	if err != nil {
		return app.ConfigDocument{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return app.ConfigDocument{}, err
	}
	return app.ConfigDocument{File: filepath.Base(path), Content: content}, nil
}

// ReplaceDocument implements app.ProjectDocuments: it stores content, as
// written, in the project's config file if it is a valid config in that
// file's format. Otherwise the stored config stays and the error wraps
// domain.ErrInvalidConfig.
func (r FileProjectRepository) ReplaceDocument(name domain.ProjectName, content []byte) error {
	path, err := r.find(name)
	if err != nil {
		return err
	}
	if _, err := decodeProject(path, name, content); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidConfig, err)
	}
	if err := r.writeSchema(); err != nil {
		return err
	}
	return writeFileAtomic(path, content)
}

// Save implements domain.ProjectRepository. It writes the config as JSON,
// replacing one in another format, and saves the JSON Schema next to it,
// which it points to so editors complete and check it.
func (r FileProjectRepository) Save(project domain.Project) error {
	doc, err := encodeProject(project)
	if err != nil {
		return err
	}
	if err := r.writeSchema(); err != nil {
		return err
	}
	path := r.path(project.Name(), ".json")
	if err := writeFileAtomic(path, doc); err != nil {
		return err
	}
	_, err = r.removeConfigs(project.Name(), path)
	return err
}

// Delete implements domain.ProjectRepository: it removes the project's
// config files.
func (r FileProjectRepository) Delete(name domain.ProjectName) error {
	removed, err := r.removeConfigs(name, "")
	if err == nil && !removed {
		return domain.NewProjectNotFound(name)
	}
	return err
}

// removeConfigs removes the config files of project name but the one at
// keep, and reports whether it removed any.
func (r FileProjectRepository) removeConfigs(name domain.ProjectName, keep string) (bool, error) {
	removed := false
	for _, ext := range configExtensions {
		path := r.path(name, ext)
		if path == keep {
			continue
		}
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		removed = true
	}
	return removed, nil
}

// find is the path of the config file of project name.
func (r FileProjectRepository) find(name domain.ProjectName) (string, error) {
	var found []string
	for _, ext := range configExtensions {
		path := r.path(name, ext)
		_, err := os.Stat(path)
		if err == nil {
			found = append(found, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	switch len(found) {
	case 0:
		return "", domain.NewProjectNotFound(name)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("%w: %s", ErrSeveralConfigs, strings.Join(found, ", "))
}

// path is where the config of project name lives in the format of ext.
func (r FileProjectRepository) path(name domain.ProjectName, ext string) string {
	return filepath.Join(r.dir, name.String()+ext)
}

// writeSchema saves the JSON Schema next to the configs, unless it is
// there already.
func (r FileProjectRepository) writeSchema() error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(r.dir, schemaFileName)
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, schema) {
		return nil
	}
	return writeFileAtomic(path, schema)
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
