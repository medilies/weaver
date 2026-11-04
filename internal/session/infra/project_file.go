package infra

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// ErrTrailingData is returned when a config has more after its object.
	ErrTrailingData = errors.New("unexpected data after the config object")
	// ErrCommandAndPrefill is returned when a pane both runs a command and
	// leaves one at the prompt.
	ErrCommandAndPrefill = errors.New(`a pane can't have both "command" and "prefill"`)
	// ErrUnknownSplitTarget is returned when "split.of" names no pane of
	// the window.
	ErrUnknownSplitTarget = errors.New("split.of names no pane of this window")
)

// projectFile is the on-disk shape of a project config, e.g.
// ~/.config/weaver/projects/<name>.json; the README documents it.
type projectFile struct {
	Schema       string            `json:"$schema,omitempty"`
	Root         string            `json:"root"`
	Repositories map[string]string `json:"repositories,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Runtimes     map[string]string `json:"runtimes,omitempty"`
	Windows      []windowFile      `json:"windows,omitempty"`
}

type windowFile struct {
	Name  string     `json:"name"`
	Panes []paneFile `json:"panes,omitempty"`
}

type paneFile struct {
	Name     string            `json:"name,omitempty"`
	Dir      string            `json:"dir,omitempty"`
	Split    *splitFile        `json:"split,omitempty"`
	Command  string            `json:"command,omitempty"`
	Prefill  string            `json:"prefill,omitempty"`
	Note     []string          `json:"note,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Runtimes map[string]string `json:"runtimes,omitempty"`
}

type splitFile struct {
	Of        string `json:"of,omitempty"`
	Direction string `json:"direction"`
	Size      string `json:"size,omitempty"`
}

// decodeProject strictly parses a config: unknown fields (usually typos)
// and anything after the object are errors, not silently ignored.
func decodeProject(name domain.ProjectName, raw []byte) (domain.Project, error) {
	var file projectFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return domain.Project{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
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
