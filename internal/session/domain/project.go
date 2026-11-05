// Package domain holds the Session bounded context's entities, value
// objects and ports. It imports nothing outside the standard library.
package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// projectNamePattern keeps names usable as a filename, a tmux session
// name and a command-line argument: tmux rewrites "." and ":" in session
// names and expands "#", and a leading "-" reads as a flag.
var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// reservedNames are weave's own commands: `weave <name>` runs the
// command, so a project named after one could never be opened.
var reservedNames = []string{"add", "completion", "edit", "help", "kill", "list", "show"}

// ProjectName is a registered project's identifier: non-empty, trimmed,
// and used both as a lookup key and as the tmux session name.
type ProjectName struct {
	value string
}

// NewProjectName validates and constructs a ProjectName.
func NewProjectName(value string) (ProjectName, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ProjectName{}, ErrEmptyProjectName
	}
	if !projectNamePattern.MatchString(trimmed) {
		return ProjectName{}, ErrInvalidProjectName
	}
	if slices.Contains(reservedNames, trimmed) {
		return ProjectName{}, fmt.Errorf("%w: %q", ErrReservedProjectName, trimmed)
	}
	return ProjectName{value: trimmed}, nil
}

func (n ProjectName) String() string {
	return n.value
}

// ProjectRoot is the absolute directory a project's panes and windows are
// rooted at, e.g. the repository checkout.
type ProjectRoot struct {
	path string
}

// NewProjectRoot validates and constructs a ProjectRoot.
func NewProjectRoot(path string) (ProjectRoot, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ProjectRoot{}, ErrEmptyProjectRoot
	}
	if !filepath.IsAbs(trimmed) {
		return ProjectRoot{}, ErrRelativeProjectRoot
	}
	return ProjectRoot{path: filepath.Clean(trimmed)}, nil
}

func (r ProjectRoot) String() string {
	return r.path
}

// Contains reports whether path is the root or a directory under it:
// /srv/shop contains /srv/shop/back, not /srv/shopping.
func (r ProjectRoot) Contains(path string) bool {
	rel, err := filepath.Rel(r.path, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Resolve is the absolute directory dir points to: the root itself, a
// directory under it, or dir as is when absolute.
func (r ProjectRoot) Resolve(dir PaneDir) string {
	switch {
	case dir.IsRoot():
		return r.path
	case filepath.IsAbs(dir.path):
		return dir.path
	default:
		return filepath.Join(r.path, dir.path)
	}
}

// Project is a registered dev project: a name, the directory its session
// is rooted at, the repositories expected in it, and the windows its
// session opens.
type Project struct {
	name      ProjectName
	root      ProjectRoot
	checkouts Checkouts
	env       Env
	before    []Command
	windows   []Window
}

// NewProject constructs a Project from its already-validated parts. Until
// it is given windows, it opens one window, named after it, holding one
// shell in its root.
func NewProject(name ProjectName, root ProjectRoot) Project {
	return Project{name: name, root: root, windows: []Window{defaultWindow(name)}}
}

// WithWindows replaces the windows the project's session opens, in order.
// Window names must be unique. With no windows, the project falls back to
// its default window.
func (p Project) WithWindows(windows ...Window) (Project, error) {
	if len(windows) == 0 {
		p.windows = []Window{defaultWindow(p.name)}
		return p, nil
	}
	seen := make(map[string]bool, len(windows))
	for _, window := range windows {
		if seen[window.name.value] {
			return Project{}, fmt.Errorf("window %q: %w", window.name.value, ErrDuplicateWindowName)
		}
		seen[window.name.value] = true
	}
	p.windows = slices.Clone(windows)
	return p, nil
}

// WithCheckouts sets the repositories expected in the project root.
func (p Project) WithCheckouts(checkouts Checkouts) Project {
	p.checkouts = checkouts
	return p
}

// WithEnv sets variables for every pane of the project.
func (p Project) WithEnv(env Env) Project {
	p.env = env
	return p
}

// RunningFirst runs cmds in every pane of the project, in order, before
// its window's and its own.
func (p Project) RunningFirst(cmds ...Command) Project {
	p.before = slices.Clone(cmds)
	return p
}

func defaultWindow(name ProjectName) Window {
	return Window{name: WindowName(name), panes: []Pane{NewPane()}}
}

func (p Project) Name() ProjectName {
	return p.name
}

func (p Project) Root() ProjectRoot {
	return p.root
}

func (p Project) Checkouts() Checkouts {
	return p.checkouts
}

// Env is the variables every pane gets, unless the pane overrides them.
func (p Project) Env() Env {
	return p.env
}

// Before is the commands run first in every pane.
func (p Project) Before() []Command {
	return slices.Clone(p.before)
}

// Windows are the windows the project's session opens, in order.
func (p Project) Windows() []Window {
	return slices.Clone(p.windows)
}
