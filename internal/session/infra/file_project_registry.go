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

// configExtensions are the extensions of the config files. A new config
// gets the first; an existing one keeps its own.
var configExtensions = []string{".yml", ".yaml"}

// FileProjectRegistry reads and saves project configs in a directory of
// "<name>.yml" or "<name>.yaml" files, in the format of projectFile.
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

// WithWarnings writes to w why config files are skipped, e.g. "kill.yml",
// which `weave kill` can't open since it runs the kill command.
func (r FileProjectRegistry) WithWarnings(w io.Writer) FileProjectRegistry {
	r.warnings = w
	return r
}

// Names implements domain.ProjectRegistry. It reads file names only:
// config files whose stem isn't exactly a valid project name, and a
// project with two configs, a .yml and a .yaml or names differing only
// in case, are skipped, with a warning, and a missing directory has no
// projects.
func (r FileProjectRegistry) Names() ([]domain.ProjectName, error) {
	configs, err := r.configs()
	if err != nil {
		return nil, err
	}
	var names []domain.ProjectName
	for _, config := range configs {
		if config.skipped != nil {
			r.warnSkipped(config.file, config.skipped)
			continue
		}
		if same := r.filesOf(configs, config.name); len(same) > 1 {
			r.warnOnce(strings.Join(same, " and "), "two configs of one project; remove one to register it")
			continue
		}
		names = append(names, config.name)
	}
	slices.SortFunc(names, func(a, b domain.ProjectName) int { return strings.Compare(a.String(), b.String()) })
	return names, nil
}

// config is a config file in the directory, and the project it names,
// or why it names none: its stem isn't exactly a valid project name.
type config struct {
	file    string
	name    domain.ProjectName
	skipped error
}

// configs are the config files in the directory.
func (r FileProjectRegistry) configs() ([]config, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []config
	for _, entry := range entries {
		// Dot-files, such as the .#shop.yml lock Emacs makes while
		// shop.yml is edited, aren't configs and aren't worth a warning.
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		stem, isConfig := configStem(entry.Name())
		if !isConfig {
			continue
		}
		name, err := domain.NewProjectName(stem)
		// "shop .yml" would list as shop, whose config is shop.yml.
		if err == nil && name.String() != stem {
			err = fmt.Errorf("blank space around its name, read as %q", name)
		}
		configs = append(configs, config{file: entry.Name(), name: name, skipped: err})
	}
	return configs, nil
}

// filesOf are the paths of the configs among configs of the project
// name, or of a name differing from it only in case.
func (r FileProjectRegistry) filesOf(configs []config, name domain.ProjectName) []string {
	var files []string
	for _, config := range configs {
		if config.skipped == nil && config.name.SameAs(name) {
			files = append(files, filepath.Join(r.dir, config.file))
		}
	}
	return files
}

// configStem is file without its config extension, if it has one.
func configStem(file string) (string, bool) {
	for _, extension := range configExtensions {
		if stem, isConfig := strings.CutSuffix(file, extension); isConfig {
			return stem, true
		}
	}
	return "", false
}

func (r FileProjectRegistry) warnSkipped(file string, reason error) {
	r.warnOnce(filepath.Join(r.dir, file), fmt.Sprintf("%v; rename it to register it", reason))
}

// warnOnce warns that what is skipped, one path or more, is skipped.
func (r FileProjectRegistry) warnOnce(skipped, message string) {
	if r.warned[skipped] {
		return
	}
	r.warned[skipped] = true
	// A warning that can't be written changes nothing worth failing for.
	_, _ = fmt.Fprintf(r.warnings, "warning: skipping %s: %s\n", skipped, message)
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
// complete and check it. It refuses a new project whose name differs
// from a config's only in case, wrapping domain.ErrNameDiffersInCase.
func (r FileProjectRegistry) Save(project domain.Project) error {
	doc, err := encodeProject(project)
	if err != nil {
		return err
	}
	path, err := r.find(project.Name())
	if errors.Is(err, domain.ErrProjectNotFound) && !errors.Is(err, domain.ErrNameDiffersInCase) {
		path, err = r.paths(project.Name())[0], nil
	}
	if err != nil {
		return err
	}
	if err := r.writeSchema(); err != nil {
		return err
	}
	return writeFileAtomic(path, doc)
}

// Delete implements domain.ProjectRegistry: it removes the project's
// config file.
func (r FileProjectRegistry) Delete(name domain.ProjectName) error {
	path, err := r.find(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// find is the path of the config file of project name, which has one
// config, ending in .yml or .yaml. The file is looked up by its exact
// name, as on a case-sensitive filesystem, so that a config whose name
// differs only in case isn't taken for name's, wherever weave runs.
// Extensions are matched exactly too: on macOS, saving shop replaces a
// shop.YML, which no listing registers.
func (r FileProjectRegistry) find(name domain.ProjectName) (string, error) {
	configs, err := r.configs()
	if err != nil {
		return "", err
	}
	found := r.filesOf(configs, name)
	switch {
	case len(found) == 0:
		return "", fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	case len(found) > 1:
		return "", fmt.Errorf("%s has two configs, %s; remove one", name, strings.Join(found, " and "))
	case !slices.Contains(r.paths(name), found[0]):
		twin, _ := configStem(filepath.Base(found[0]))
		return "", fmt.Errorf("%w: %s; %s's config is %s, and %w", domain.ErrProjectNotFound, name, twin, found[0], domain.ErrNameDiffersInCase)
	}
	return found[0], nil
}

// paths are where the config of project name may live, the first where
// a new one is saved.
func (r FileProjectRegistry) paths(name domain.ProjectName) []string {
	paths := make([]string, 0, len(configExtensions))
	for _, extension := range configExtensions {
		paths = append(paths, filepath.Join(r.dir, name.String()+extension))
	}
	return paths
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
