// Package infra adapts the Session bounded context to the outside world:
// project configs stored as YAML files, the user's tmux server, the local
// filesystem, the editor and the terminal forms.
package infra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// configExtension is the extension of the config files.
const configExtension = ".yml"

// FileProjectRegistry reads and saves project configs in a directory of
// "<name>.yml" files, in the format of projectFile.
type FileProjectRegistry struct {
	dir      string
	warnings io.Writer
	// warned holds the skipped files already warned about: each is
	// warned about once per registry and its copies.
	warned map[string]bool
}

// NewFileProjectRegistry constructs a registry rooted at dir.
func NewFileProjectRegistry(dir string) FileProjectRegistry {
	return FileProjectRegistry{dir: dir, warnings: io.Discard, warned: map[string]bool{}}
}

// WithWarnings writes to w why config files are skipped, e.g. "edit.yml",
// which `weave edit` can't open since it runs the edit command.
func (r FileProjectRegistry) WithWarnings(w io.Writer) FileProjectRegistry {
	r.warnings = w
	return r
}

// Names implements domain.ProjectRegistry. It reads file names only:
// config files whose stem isn't exactly a valid project name are
// skipped, with a warning, and a missing directory has no projects.
func (r FileProjectRegistry) Names() ([]domain.ProjectName, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []domain.ProjectName
	for _, entry := range entries {
		stem, isConfig := strings.CutSuffix(entry.Name(), configExtension)
		// Dot-files, such as the .#shop.yml lock Emacs makes while
		// shop.yml is edited, aren't configs and aren't worth a warning.
		if entry.IsDir() || !isConfig || strings.HasPrefix(stem, ".") {
			continue
		}
		name, err := domain.NewProjectName(stem)
		if err != nil {
			r.warnSkipped(entry.Name(), err)
			continue
		}
		// "shop .yml" would list as shop, whose config is shop.yml.
		if name.String() != stem {
			r.warnSkipped(entry.Name(), fmt.Errorf("blank space around its name, read as %q", name))
			continue
		}
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b domain.ProjectName) int { return strings.Compare(a.String(), b.String()) })
	return names, nil
}

func (r FileProjectRegistry) warnSkipped(file string, reason error) {
	path := filepath.Join(r.dir, file)
	if r.warned[path] {
		return
	}
	r.warned[path] = true
	// A warning that can't be written changes nothing worth failing for.
	_, _ = fmt.Fprintf(r.warnings, "warning: skipping %s: %v; rename it to register it\n", path, reason)
}

// FindByName implements domain.ProjectRegistry.
func (r FileProjectRegistry) FindByName(name domain.ProjectName) (domain.Project, error) {
	path, err := r.find(name)
	if err != nil {
		return domain.Project{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Project{}, fmt.Errorf("reading %s: %w", path, err)
	}
	project, err := decodeProject(name, raw)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w: %w", path, domain.ErrInvalidConfig, err)
	}
	return project, nil
}

// Document implements app.ProjectDocuments.
func (r FileProjectRegistry) Document(name domain.ProjectName) (app.ConfigDocument, error) {
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
// written, in the project's config file if it is a valid config. Otherwise the stored config stays and the error wraps
// domain.ErrInvalidConfig.
func (r FileProjectRegistry) ReplaceDocument(name domain.ProjectName, content []byte) error {
	path, err := r.find(name)
	if err != nil {
		return err
	}
	if _, err := decodeProject(name, content); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidConfig, err)
	}
	if err := r.writeSchema(); err != nil {
		return err
	}
	return writeFileAtomic(path, content)
}

// Commented implements app.ProjectDocuments: any comment counts but the
// schema header weaver writes first.
func (r FileProjectRegistry) Commented(name domain.ProjectName) (bool, error) {
	document, err := r.Document(name)
	if err != nil {
		return false, err
	}
	commented, err := hasComments(document.Content)
	if err != nil {
		return false, fmt.Errorf("%s: %w: %w", document.File, domain.ErrInvalidConfig, err)
	}
	return commented, nil
}

// Save implements domain.ProjectRegistry. It writes the config and
// saves the JSON Schema next to it, which it points to so editors
// complete and check it.
func (r FileProjectRegistry) Save(project domain.Project) error {
	doc, err := encodeProject(project)
	if err != nil {
		return err
	}
	if err := r.writeSchema(); err != nil {
		return err
	}
	return writeFileAtomic(r.path(project.Name()), doc)
}

// Delete implements domain.ProjectRegistry: it removes the project's
// config file.
func (r FileProjectRegistry) Delete(name domain.ProjectName) error {
	err := os.Remove(r.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return err
}

// find is the path of the config file of project name.
func (r FileProjectRegistry) find(name domain.ProjectName) (string, error) {
	path := r.path(name)
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return path, err
}

// path is where the config of project name lives.
func (r FileProjectRegistry) path(name domain.ProjectName) string {
	return filepath.Join(r.dir, name.String()+configExtension)
}

// writeSchema saves the JSON Schema next to the configs, unless it is
// there already.
func (r FileProjectRegistry) writeSchema() error {
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
// so a reader never sees half a file. A symbolic link, as dotfiles
// managers make, stays: the file it points to is replaced. Errors name
// that file, not only the temporary one.
func writeFileAtomic(path string, data []byte) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("saving %s: %w", path, err)
		}
	}()
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("broken link: %w", err)
		}
		path = target
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	// Some filesystems, such as XFS, may otherwise rename an empty file
	// into place if the machine crashes.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
