package infra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// ErrTrailingData is returned when a config holds a second document.
	ErrTrailingData = errors.New("unexpected data after the config object")
	// ErrCommandAndPrefill is returned when a pane both runs a command and
	// leaves one at the prompt.
	ErrCommandAndPrefill = errors.New(`a pane can't have both "command" and "prefill"`)
	// ErrUnknownSplitTarget is returned when "split.of" names no pane of
	// the window.
	ErrUnknownSplitTarget = errors.New("split.of names no pane of this window")
)

// projectFile is the on-disk shape of a project config, e.g.
// ~/.config/weaver/projects/<name>.yml; the README documents it.
type projectFile struct {
	Root         string            `yaml:"root"`
	Repositories map[string]string `yaml:"repositories,omitempty"`
	Env          map[string]string `yaml:"env,omitempty"`
	Runtimes     map[string]string `yaml:"runtimes,omitempty"`
	Windows      []windowFile      `yaml:"windows,omitempty"`
}

type windowFile struct {
	Name  string     `yaml:"name"`
	Panes []paneFile `yaml:"panes,omitempty"`
}

type paneFile struct {
	Name     string            `yaml:"name,omitempty"`
	Dir      string            `yaml:"dir,omitempty"`
	Split    *splitFile        `yaml:"split,omitempty"`
	Command  string            `yaml:"command,omitempty"`
	Prefill  string            `yaml:"prefill,omitempty"`
	Note     []string          `yaml:"note,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Runtimes map[string]string `yaml:"runtimes,omitempty"`
}

type splitFile struct {
	Of        string `yaml:"of,omitempty"`
	Direction string `yaml:"direction"`
	Size      string `yaml:"size,omitempty"`
}

// schemaHeader points YAML editors (e.g. VS Code's YAML extension) at the
// schema saved next to the configs.
const schemaHeader = "# yaml-language-server: $schema=./" + schemaFileName + "\n"

// decodeProject strictly parses a config: unknown fields (usually typos)
// and a second document are errors, not silently ignored. Values are read
// as written, so "php: 8.10" pins 8.10, not the number 8.1.
func decodeProject(name domain.ProjectName, raw []byte) (domain.Project, error) {
	var file projectFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	// An empty document leaves the root empty, which toProject reports.
	if err := decoder.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return domain.Project{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return domain.Project{}, ErrTrailingData
	}
	return file.toProject(name)
}

func (f projectFile) toProject(name domain.ProjectName) (domain.Project, error) {
	rootPath, err := expandHome(f.Root)
	if err != nil {
		return domain.Project{}, err
	}
	root, err := domain.NewProjectRoot(rootPath)
	if err != nil {
		return domain.Project{}, err
	}
	checkouts, err := toCheckouts(f.Repositories)
	if err != nil {
		return domain.Project{}, fmt.Errorf("repositories: %w", err)
	}
	env, err := domain.NewEnv(f.Env)
	if err != nil {
		return domain.Project{}, fmt.Errorf("env: %w", err)
	}
	runtimes, err := toRuntimes(f.Runtimes)
	if err != nil {
		return domain.Project{}, fmt.Errorf("runtimes: %w", err)
	}
	windows := make([]domain.Window, 0, len(f.Windows))
	for _, w := range f.Windows {
		window, err := w.toWindow()
		if err != nil {
			return domain.Project{}, fmt.Errorf("window %q: %w", w.Name, err)
		}
		windows = append(windows, window)
	}
	return domain.NewProject(name, root).
		WithCheckouts(checkouts).
		WithEnv(env).
		WithRuntimes(runtimes).
		WithWindows(windows...)
}

func toCheckouts(repositories map[string]string) (domain.Checkouts, error) {
	checkouts := make([]domain.Checkout, 0, len(repositories))
	for dir, remote := range repositories {
		checkout, err := domain.NewCheckout(dir, remote)
		if err != nil {
			return domain.Checkouts{}, err
		}
		checkouts = append(checkouts, checkout)
	}
	return domain.NewCheckouts(checkouts...)
}

func toRuntimes(pins map[string]string) (domain.Runtimes, error) {
	runtimes := make([]domain.Runtime, 0, len(pins))
	for kind, version := range pins {
		runtime, err := domain.NewRuntime(kind, version)
		if err != nil {
			return domain.Runtimes{}, err
		}
		runtimes = append(runtimes, runtime)
	}
	return domain.NewRuntimes(runtimes...)
}

func (w windowFile) toWindow() (domain.Window, error) {
	name, err := domain.NewWindowName(w.Name)
	if err != nil {
		return domain.Window{}, err
	}
	panes := make([]domain.Pane, 0, len(w.Panes))
	for i, p := range w.Panes {
		pane, err := p.toPane(i, w.Panes)
		if err != nil {
			return domain.Window{}, fmt.Errorf("pane %s: %w", p.ref(i), err)
		}
		panes = append(panes, pane)
	}
	return domain.NewWindow(name, panes...)
}

// toPane builds the pane at index of its window; siblings are all the
// window's panes, so "split.of" can be resolved by name.
func (p paneFile) toPane(index int, siblings []paneFile) (domain.Pane, error) {
	pane := domain.NewPane()
	if p.Name != "" {
		name, err := domain.NewPaneName(p.Name)
		if err != nil {
			return domain.Pane{}, err
		}
		pane = pane.Named(name)
	}

	dirPath, err := expandHome(p.Dir)
	if err != nil {
		return domain.Pane{}, err
	}
	dir, err := domain.NewPaneDir(dirPath)
	if err != nil {
		return domain.Pane{}, fmt.Errorf("dir: %w", err)
	}
	pane = pane.In(dir)

	if p.Split != nil {
		split, err := p.Split.toSplit(index, siblings)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("split: %w", err)
		}
		pane = pane.Splitting(split)
	}

	if pane, err = p.withStartup(pane); err != nil {
		return domain.Pane{}, err
	}

	if len(p.Note) > 0 {
		pane = pane.WithNote(p.Note...)
	}

	env, err := domain.NewEnv(p.Env)
	if err != nil {
		return domain.Pane{}, fmt.Errorf("env: %w", err)
	}
	runtimes, err := toRuntimes(p.Runtimes)
	if err != nil {
		return domain.Pane{}, fmt.Errorf("runtimes: %w", err)
	}
	return pane.WithEnv(env).WithRuntimes(runtimes), nil
}

func (p paneFile) withStartup(pane domain.Pane) (domain.Pane, error) {
	switch {
	case p.Command != "" && p.Prefill != "":
		return domain.Pane{}, ErrCommandAndPrefill
	case p.Command != "":
		cmd, err := domain.NewCommand(p.Command)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("command: %w", err)
		}
		return pane.Running(cmd), nil
	case p.Prefill != "":
		cmd, err := domain.NewCommand(p.Prefill)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("prefill: %w", err)
		}
		return pane.Prefilling(cmd), nil
	}
	return pane, nil
}

// ref names a pane in errors: by name, else by 1-based position.
func (p paneFile) ref(index int) string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return strconv.Quote(name)
	}
	return "#" + strconv.Itoa(index+1)
}

// toSplit resolves "of" to a pane index, defaulting to the previous pane.
func (s splitFile) toSplit(index int, siblings []paneFile) (domain.Split, error) {
	of := index - 1
	if target := strings.TrimSpace(s.Of); target != "" {
		of = -1
		for i, sibling := range siblings {
			if strings.TrimSpace(sibling.Name) == target {
				of = i
				break
			}
		}
		if of < 0 {
			return domain.Split{}, fmt.Errorf("%w: %q", ErrUnknownSplitTarget, s.Of)
		}
	}
	direction, err := domain.ParseDirection(s.Direction)
	if err != nil {
		return domain.Split{}, err
	}
	size, err := domain.ParseSize(s.Size)
	if err != nil {
		return domain.Split{}, err
	}
	return domain.NewSplit(of, direction, size)
}

// encodeProject writes a config in its canonical form: defaults left out,
// "split.of" only when it isn't the previous pane, pane directories under
// the root relative to it, the home directory as "~", and pointing to the
// schema saved next to it with a header line.
func encodeProject(project domain.Project) ([]byte, error) {
	root := project.Root()
	file := projectFile{
		Root:     contractHome(root.String()),
		Env:      envMap(project.Env()),
		Runtimes: runtimeMap(project.Runtimes()),
	}
	for _, checkout := range project.Checkouts().List() {
		if file.Repositories == nil {
			file.Repositories = make(map[string]string)
		}
		file.Repositories[checkout.Dir()] = checkout.Remote()
	}
	if windows := project.Windows(); !onlyDefaultWindow(project.Name(), windows) {
		for _, window := range windows {
			file.Windows = append(file.Windows, windowFileOf(root, window))
		}
	}

	var doc bytes.Buffer
	doc.WriteString(schemaHeader)
	encoder := yaml.NewEncoder(&doc)
	encoder.SetIndent(2)
	if err := encoder.Encode(file); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return doc.Bytes(), nil
}

func windowFileOf(root domain.ProjectRoot, window domain.Window) windowFile {
	file := windowFile{Name: window.Name().String()}
	panes := window.Panes()
	if len(panes) == 1 && isPlain(panes[0]) {
		return file
	}
	for i := range panes {
		file.Panes = append(file.Panes, paneFileOf(root, i, panes))
	}
	return file
}

// paneFileOf is the file shape of panes[index].
func paneFileOf(root domain.ProjectRoot, index int, panes []domain.Pane) paneFile {
	pane := panes[index]
	file := paneFile{
		Name:     pane.Name().String(),
		Dir:      dirOf(root, pane.Dir()),
		Note:     pane.Note(),
		Env:      envMap(pane.Env()),
		Runtimes: runtimeMap(pane.Runtimes()),
	}
	if split, ok := pane.Split(); ok {
		file.Split = &splitFile{Direction: split.Direction().String(), Size: split.Size().String()}
		if split.Of() != index-1 {
			file.Split.Of = panes[split.Of()].Name().String()
		}
	}
	if cmd, ok := pane.Startup().Command(); ok {
		if pane.Startup().Submits() {
			file.Command = cmd.String()
		} else {
			file.Prefill = cmd.String()
		}
	}
	return file
}

// dirOf writes a pane directory under the root relative to it.
func dirOf(root domain.ProjectRoot, dir domain.PaneDir) string {
	path := dir.String()
	if !filepath.IsAbs(path) {
		return path
	}
	if !root.Contains(path) {
		return contractHome(path)
	}
	rel, err := filepath.Rel(root.String(), path)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

// isPlain reports whether pane is a bare shell in the root, which a
// window without panes means.
func isPlain(pane domain.Pane) bool {
	_, splits := pane.Split()
	_, types := pane.Startup().Command()
	return pane.Name().IsZero() && pane.Dir().IsRoot() && !splits && !types &&
		len(pane.Note()) == 0 && len(pane.Env().Vars()) == 0 && len(pane.Runtimes().List()) == 0
}

// onlyDefaultWindow reports whether windows are what a project without
// windows opens: one window named after it, holding a bare shell.
func onlyDefaultWindow(name domain.ProjectName, windows []domain.Window) bool {
	if len(windows) != 1 || windows[0].Name().String() != name.String() {
		return false
	}
	panes := windows[0].Panes()
	return len(panes) == 1 && isPlain(panes[0])
}

func envMap(env domain.Env) map[string]string {
	vars := env.Vars()
	if len(vars) == 0 {
		return nil
	}
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Name()] = v.Value()
	}
	return m
}

func runtimeMap(runtimes domain.Runtimes) map[string]string {
	list := runtimes.List()
	if len(list) == 0 {
		return nil
	}
	m := make(map[string]string, len(list))
	for _, runtime := range list {
		m[string(runtime.Kind())] = runtime.Version()
	}
	return m
}
