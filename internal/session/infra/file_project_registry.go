// Package infra adapts the Session bounded context to the outside world:
// the settings and project configs stored as YAML files, the user's tmux
// server, the local filesystem, the editor and the terminal forms.
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

	"go.yaml.in/yaml/v3"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// configNames are the files a root's config is looked for in, in order,
// when its settings entry names none. A new config is the first.
var configNames = []string{"weaver.yml", "weaver.yaml", ".weaver.yml", ".weaver.yaml"}

// FileProjectRegistry registers projects in the settings file's projects
// list, each a root, and reads and saves their configs, in the format of
// projectFile, where their entries say.
type FileProjectRegistry struct {
	settings  string
	schemaURL string
	warnings  io.Writer
	// former is the directory configs were kept in before, warned about
	// while it still holds some.
	former string
	// warned holds what was already warned about: each is warned about
	// once per registry and its copies.
	warned map[string]bool
}

// NewFileProjectRegistry constructs a registry listing its projects in
// the settings file at settings.
func NewFileProjectRegistry(settings string) FileProjectRegistry {
	return FileProjectRegistry{settings: settings, warnings: io.Discard, warned: map[string]bool{}, schemaURL: SchemaURL("")}
}

// WithWarnings writes to w why settings entries are skipped, e.g. one
// whose root isn't absolute, and that former still holds configs.
func (r FileProjectRegistry) WithWarnings(w io.Writer, former string) FileProjectRegistry {
	r.warnings, r.former = w, former
	return r
}

// WithSchema points the configs it writes at the schema at url.
func (r FileProjectRegistry) WithSchema(url string) FileProjectRegistry {
	r.schemaURL = url
	return r
}

// SchemaURL is where the configs' schema of weave's version is published:
// at the release's tag, else on main.
func SchemaURL(version string) string {
	ref := "main"
	if released(version) {
		ref = version
	}
	return "https://raw.githubusercontent.com/medilies/weaver/" + ref + "/internal/session/infra/" + schemaFileName
}

// released reports whether version is a release's tag, as v1.2.3.
func released(version string) bool {
	numbers, ok := strings.CutPrefix(version, "v")
	parts := strings.Split(numbers, ".")
	return ok && len(parts) == 3 && !slices.ContainsFunc(parts, func(p string) bool {
		return p == "" || strings.Trim(p, "0123456789") != ""
	})
}

// entry is a project the settings list: its root, the config it names,
// as written, and the name it gives the project, if any.
type entry struct {
	root   domain.ProjectRoot
	config string
	name   string
}

// entries are the settings' projects, those that can't be read skipped
// with a warning; none when there is no settings file or list.
func (r FileProjectRegistry) entries() ([]entry, error) {
	doc, err := r.settingsDocument()
	if err != nil {
		return nil, err
	}
	list := projectsList(doc)
	if list == nil {
		return nil, nil
	}
	var entries []entry
	for i, item := range list.Content {
		e, err := parseEntry(item)
		if err != nil {
			r.warnOnce(fmt.Sprintf("projects[%d]", i), fmt.Sprintf("warning: skipping projects[%d] in %s: %v\n", i, r.settings, err))
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// parseEntry reads a projects list item: a root, or a map of its root,
// config and name.
func parseEntry(item *yaml.Node) (entry, error) {
	var raw struct{ root, config, name string }
	switch item.Kind {
	case yaml.ScalarNode:
		raw.root = item.Value
	case yaml.MappingNode:
		for i := 0; i+1 < len(item.Content); i += 2 {
			key, value := item.Content[i].Value, item.Content[i+1]
			if value.Kind != yaml.ScalarNode {
				return entry{}, fmt.Errorf("%s: want a string", key)
			}
			switch key {
			case "root":
				raw.root = value.Value
			case "config":
				raw.config = strings.TrimSpace(value.Value)
			case "name":
				raw.name = value.Value
			default:
				return entry{}, fmt.Errorf("unknown field %s", key)
			}
		}
	default:
		return entry{}, errors.New("want a root, or a map of root, config and name")
	}
	path, err := ExpandHome(strings.TrimSpace(raw.root))
	if err != nil {
		return entry{}, err
	}
	root, err := domain.NewProjectRoot(path)
	if err != nil {
		return entry{}, fmt.Errorf("root: %w", err)
	}
	e := entry{root: root, config: raw.config}
	if raw.name != "" {
		name, err := domain.NewProjectName(raw.name)
		if err != nil {
			return entry{}, fmt.Errorf("name: %w", err)
		}
		e.name = name.String()
	}
	return e, nil
}

// settingsDocument is the settings file's YAML, an empty document when
// there is none.
func (r FileProjectRegistry) settingsDocument() (*yaml.Node, error) {
	raw, err := os.ReadFile(r.settings)
	if errors.Is(err, os.ErrNotExist) {
		return &yaml.Node{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", r.settings, err)
	}
	return &doc, nil
}

// projectsList is the projects list of the settings doc, if it has one.
func projectsList(doc *yaml.Node) *yaml.Node {
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	settings := doc.Content[0]
	for i := 0; i+1 < len(settings.Content); i += 2 {
		if settings.Content[i].Value == "projects" && settings.Content[i+1].Kind == yaml.SequenceNode {
			return settings.Content[i+1]
		}
	}
	return nil
}

// at is the entry of root, or an error wrapping domain.ErrProjectNotFound,
// or domain.ErrRootListedTwice.
func (r FileProjectRegistry) at(root domain.ProjectRoot) (entry, error) {
	entries, err := r.entries()
	if err != nil {
		return entry{}, err
	}
	var found []entry
	for _, e := range entries {
		if e.root == root {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return entry{}, fmt.Errorf("%w: nothing registered at %s", domain.ErrProjectNotFound, root)
	case 1:
		return found[0], nil
	}
	return entry{}, fmt.Errorf("%s: %w", root, domain.ErrRootListedTwice)
}

// path is the file of e's config: the one it names, relative to its root
// or absolute, else the first of configNames in its root that exists,
// else the first.
func (e entry) path() (string, error) {
	if e.config != "" {
		path, err := ExpandHome(e.config)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.root.String(), path)
		}
		return filepath.Clean(path), nil
	}
	for _, name := range configNames {
		path := filepath.Join(e.root.String(), name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return filepath.Join(e.root.String(), configNames[0]), nil
}

// read is e's config as stored, or an error wrapping
// domain.ErrMissingConfig.
func (e entry) read() (string, []byte, error) {
	path, err := e.path()
	if err != nil {
		return "", nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil, fmt.Errorf("%s: %w", path, domain.ErrMissingConfig)
	}
	if err != nil {
		return path, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return path, raw, nil
}

// Registered implements domain.ProjectRegistry. A project is named by its
// entry, else by its config, when that can be read, else after its root.
// A root listed twice is listed once, where it is first.
func (r FileProjectRegistry) Registered() ([]domain.ProjectRef, error) {
	r.warnFormer()
	entries, err := r.entries()
	if err != nil {
		return nil, err
	}
	var refs []domain.ProjectRef
	for _, e := range entries {
		if slices.ContainsFunc(refs, func(ref domain.ProjectRef) bool { return ref.Root() == e.root }) {
			continue
		}
		name, err := domain.NewProjectName(e.name)
		if err != nil {
			_, raw, _ := e.read()
			name, err = domain.NewProjectName(configName(raw))
		}
		if err != nil {
			name, err = domain.NewProjectName(filepath.Base(e.root.String()))
		}
		if err != nil {
			r.warnOnce(e.root.String(), fmt.Sprintf("warning: skipping %s in %s: %v\n", e.root, r.settings, err))
			continue
		}
		refs = append(refs, domain.NewProjectRef(e.root, name))
	}
	return refs, nil
}

// warnFormer warns that the directory configs were kept in before still
// holds some, which aren't read.
func (r FileProjectRegistry) warnFormer() {
	if r.former == "" {
		return
	}
	files, _ := os.ReadDir(r.former)
	if !slices.ContainsFunc(files, func(f os.DirEntry) bool {
		name := f.Name()
		return !f.IsDir() && !strings.HasPrefix(name, ".") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
	}) {
		return
	}
	r.warnOnce(r.former, fmt.Sprintf("warning: the configs in %s aren't read anymore: move each to its root as weaver.yml, "+
		"without its root:, and list the root under projects: in %s\n", r.former, r.settings))
}

func (r FileProjectRegistry) warnOnce(key, message string) {
	if r.warned[key] {
		return
	}
	r.warned[key] = true
	// A warning that can't be written changes nothing worth failing for.
	_, _ = io.WriteString(r.warnings, message)
}

// Find implements domain.ProjectRegistry.
func (r FileProjectRegistry) Find(root domain.ProjectRoot) (domain.Project, error) {
	e, err := r.at(root)
	if err != nil {
		return domain.Project{}, err
	}
	path, raw, err := e.read()
	if err != nil {
		return domain.Project{}, err
	}
	project, err := decodeProject(raw, root, e.name)
	if err != nil {
		return domain.Project{}, fmt.Errorf("%s: %w: %w", path, domain.ErrInvalidConfig, err)
	}
	return project, nil
}

// Document implements app.ProjectDocuments.
func (r FileProjectRegistry) Document(root domain.ProjectRoot) (app.ConfigDocument, error) {
	e, err := r.at(root)
	if err != nil {
		return app.ConfigDocument{}, err
	}
	path, raw, err := e.read()
	if err != nil {
		return app.ConfigDocument{}, err
	}
	return app.ConfigDocument{File: path, Content: raw}, nil
}

// ReplaceDocument implements app.ProjectDocuments: it stores content, as
// written, in the project's config file if it is a valid config.
// Otherwise the stored config stays and the error wraps
// domain.ErrInvalidConfig.
func (r FileProjectRegistry) ReplaceDocument(root domain.ProjectRoot, content []byte) error {
	e, err := r.at(root)
	if err != nil {
		return err
	}
	path, err := e.path()
	if err != nil {
		return err
	}
	if _, err := decodeProject(content, root, e.name); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidConfig, err)
	}
	return writeFileAtomic(path, content)
}

// Commented implements app.ProjectDocuments: any comment counts but the
// schema header weaver writes first.
func (r FileProjectRegistry) Commented(root domain.ProjectRoot) (bool, error) {
	document, err := r.Document(root)
	if err != nil {
		return false, err
	}
	commented, err := hasComments(document.Content)
	if err != nil {
		return false, fmt.Errorf("%s: %w: %w", document.File, domain.ErrInvalidConfig, err)
	}
	return commented, nil
}

// ConfigAt implements app.ProjectDocuments.
func (r FileProjectRegistry) ConfigAt(root domain.ProjectRoot, config string) (domain.Project, app.ConfigDocument, error) {
	e, err := r.at(root)
	if errors.Is(err, domain.ErrProjectNotFound) {
		e, err = entry{root: root}, nil
	}
	if err != nil {
		return domain.Project{}, app.ConfigDocument{}, err
	}
	if config != "" {
		e.config = config
	}
	path, raw, err := e.read()
	if err != nil {
		return domain.Project{}, app.ConfigDocument{}, err
	}
	project, err := decodeProject(raw, root, e.name)
	if err != nil {
		return domain.Project{}, app.ConfigDocument{}, fmt.Errorf("%s: %w: %w", path, domain.ErrInvalidConfig, err)
	}
	return project, app.ConfigDocument{File: path, Content: raw}, nil
}

// Save implements domain.ProjectRegistry. A new root is listed as the
// home directory and the root from it, as "~/dev/shop", with the config
// it names when given.
func (r FileProjectRegistry) Save(project domain.Project, config string) error {
	e, err := r.at(project.Root())
	registered := err == nil
	if errors.Is(err, domain.ErrProjectNotFound) {
		e, err = entry{root: project.Root()}, nil
	}
	if err != nil {
		return err
	}
	_, stored, _ := e.read() // before repointing: the config whose name stays
	repointed := config != "" && config != e.config
	if config != "" {
		e.config = config
	}
	if e.name != "" && e.name != project.Name().String() {
		// The settings name the project: its new name goes there.
		e.name, repointed = project.Name().String(), true
	}
	doc, err := r.encoded(project, e, stored)
	if err != nil {
		return err
	}
	path, err := e.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, doc); err != nil {
		return err
	}
	switch {
	case !registered:
		return r.editProjects(func(list *yaml.Node) { list.Content = slices.Insert(list.Content, 0, e.node()) })
	case repointed:
		return r.editProjects(func(list *yaml.Node) {
			for i, item := range list.Content {
				if listed, err := parseEntry(item); err == nil && listed.root == e.root {
					list.Content[i] = e.node()
				}
			}
		})
	}
	return nil
}

// Encoded is the config saving project writes: named as its config names
// it when its settings entry names it, else only when its name isn't its
// root's directory's.
func (r FileProjectRegistry) Encoded(project domain.Project) ([]byte, error) {
	e, err := r.at(project.Root())
	if err != nil {
		e = entry{root: project.Root()}
	}
	_, stored, _ := e.read()
	return r.encoded(project, e, stored)
}

// encoded is project's config, stored holding the config it replaces.
func (r FileProjectRegistry) encoded(project domain.Project, e entry, stored []byte) ([]byte, error) {
	name := project.Name().String()
	switch {
	case e.name != "":
		name = configName(stored) // the settings name it: the config's stays
	case name == filepath.Base(e.root.String()):
		name = ""
	}
	return encodeProject(project, name, r.schemaURL)
}

// node is e as the settings list it: its root alone, or a map of its root
// and config.
func (e entry) node() *yaml.Node {
	root := &yaml.Node{Kind: yaml.ScalarNode, Value: contractHome(e.root.String())}
	if e.config == "" && e.name == "" {
		return root
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "root"}, root}}
	for _, field := range [][2]string{{"config", e.config}, {"name", e.name}} {
		if field[1] != "" {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: field[0]},
				&yaml.Node{Kind: yaml.ScalarNode, Value: field[1]})
		}
	}
	return node
}

// Delete implements domain.ProjectRegistry: it takes the root's entries
// out of the settings' projects list.
func (r FileProjectRegistry) Delete(root domain.ProjectRoot) error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(entries, func(e entry) bool { return e.root == root }) {
		return fmt.Errorf("%w: nothing registered at %s", domain.ErrProjectNotFound, root)
	}
	return r.editProjects(func(list *yaml.Node) {
		list.Content = slices.DeleteFunc(list.Content, func(item *yaml.Node) bool {
			listed, err := parseEntry(item)
			return err == nil && listed.root == root
		})
	})
}

// editProjects changes the settings' projects list with edit, creating
// the settings file or list when missing, and keeping the rest of the
// file and its comments.
func (r FileProjectRegistry) editProjects(edit func(list *yaml.Node)) error {
	doc, err := r.settingsDocument()
	if err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		doc = &yaml.Node{Kind: yaml.DocumentNode, HeadComment: doc.HeadComment, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	settings := doc.Content[0]
	if settings.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the settings aren't a map", r.settings)
	}
	var list *yaml.Node
	for i := 0; i+1 < len(settings.Content); i += 2 {
		if settings.Content[i].Value != "projects" {
			continue
		}
		switch value := settings.Content[i+1]; {
		case value.Kind == yaml.SequenceNode:
			list = value
		case value.Kind == yaml.ScalarNode && value.Tag == "!!null":
			list = &yaml.Node{Kind: yaml.SequenceNode}
			settings.Content[i+1] = list
		default:
			return fmt.Errorf("%s: projects: want a list", r.settings)
		}
	}
	if list == nil {
		list = &yaml.Node{Kind: yaml.SequenceNode}
		settings.Content = append(settings.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "projects"}, list)
	}
	edit(list)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.settings), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(r.settings, out.Bytes())
}

// writeFileAtomic replaces path with data, so a reader never sees half a
// file, keeping its permissions, or readable by its owner only when new. A symbolic link, as dotfiles
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
	// A file replaced keeps its permissions; a new one is its owner's only.
	if info, err := os.Stat(path); err == nil {
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
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
