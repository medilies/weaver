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
//
// TODO: compare names case-insensitively? "Edit" passes, and on a
// case-insensitive filesystem (macOS) "Edit.yml" and "edit.yml" are one
// file, so "shop" and "Shop" would overwrite each other.
var reservedNames = []string{"completion", "edit", "freeze", "help", "kill", "list", "show"}

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
// rooted at.
type ProjectRoot struct {
	path string
}

// NewProjectRoot validates and constructs a ProjectRoot.
func NewProjectRoot(path string) (ProjectRoot, error) {
	trimmed, err := oneLine(path)
	if err != nil {
		return ProjectRoot{}, err
	}
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
	_, ok := under(r.path, path)
	return ok
}

// Relative is dir relative to the root when it is an absolute directory
// at or under it, the zero PaneDir for the root itself. Other directories
// are kept as they are.
func (r ProjectRoot) Relative(dir PaneDir) PaneDir {
	if !filepath.IsAbs(dir.path) {
		return dir
	}
	rel, ok := under(r.path, dir.path)
	if !ok {
		return dir
	}
	relative, _ := NewPaneDir(rel) // a cleaned path, already one line
	return relative
}

// under is path relative to base, if path is base or a directory under
// it: "." for base itself.
func under(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
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
// is rooted at, and the windows its session opens.
type Project struct {
	name    ProjectName
	root    ProjectRoot
	env     Env
	before  []Command
	clears  Clearing
	windows []Window
	// defaulted is true until the project is given windows: it opens the
	// window named after it.
	defaulted bool
}

// NewProject constructs a Project from its already-validated parts. Until
// it is given windows, it opens one window, named after it, holding one
// shell in its root.
func NewProject(name ProjectName, root ProjectRoot) Project {
	return Project{name: name, root: root, windows: []Window{defaultWindow(name)}, defaulted: true}
}

// WithWindows replaces the windows the project's session opens, in order.
// Window names must be unique. With no windows, the project falls back to
// its default window.
func (p Project) WithWindows(windows ...Window) (Project, error) {
	if len(windows) == 0 {
		p.windows, p.defaulted = []Window{defaultWindow(p.name)}, true
		return p, nil
	}
	seen := make(map[string]bool, len(windows))
	for _, window := range windows {
		if seen[window.name.value] {
			return Project{}, fmt.Errorf("window %q: %w", window.name.value, ErrDuplicateWindowName)
		}
		seen[window.name.value] = true
	}
	p.windows, p.defaulted = slices.Clone(windows), false
	return p, nil
}

// Named is the project named name instead. Until it is given windows, the
// window it opens follows the name.
func (p Project) Named(name ProjectName) Project {
	p.name = name
	if p.defaulted {
		p.windows = []Window{defaultWindow(name)}
	}
	return p
}

// RootedAt is the project rooted at root instead: its windows' and
// panes' relative dirs follow it.
func (p Project) RootedAt(root ProjectRoot) Project {
	p.root = root
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

// WithClearing sets whether every pane's before commands are hidden once
// they ran, unless its window or itself sets it.
func (p Project) WithClearing(c Clearing) Project {
	p.clears = c
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

// Env is the variables every pane gets, unless the pane overrides them.
func (p Project) Env() Env {
	return p.env
}

// Before is the commands run first in every pane.
func (p Project) Before() []Command {
	return slices.Clone(p.before)
}

// Clearing is the project's setting of whether its panes' before
// commands are hidden once they ran.
func (p Project) Clearing() Clearing {
	return p.clears
}

// Windows are the windows the project's session opens, in order.
func (p Project) Windows() []Window {
	return slices.Clone(p.windows)
}

// AddingWindow is the project with window opened last.
func (p Project) AddingWindow(window Window) (Project, error) {
	return p.WithWindows(append(p.Windows(), window)...)
}

// ReplacingWindow is the project with the window at index replaced, as
// when its panes change.
func (p Project) ReplacingWindow(index int, window Window) (Project, error) {
	if err := p.checkWindowIndex(index); err != nil {
		return Project{}, err
	}
	windows := p.Windows()
	windows[index] = window
	return p.WithWindows(windows...)
}

// WithoutWindow is the project without the window at index. A project
// keeps at least one window.
func (p Project) WithoutWindow(index int) (Project, error) {
	if err := p.checkWindowIndex(index); err != nil {
		return Project{}, err
	}
	if len(p.windows) == 1 {
		return Project{}, ErrLastWindow
	}
	return p.WithWindows(slices.Delete(p.Windows(), index, index+1)...)
}

// MovingWindow is the project with the window at from moved to to, the
// others shifting to make room.
func (p Project) MovingWindow(from, to int) (Project, error) {
	if err := p.checkWindowIndex(from); err != nil {
		return Project{}, err
	}
	if err := p.checkWindowIndex(to); err != nil {
		return Project{}, err
	}
	window := p.windows[from]
	return p.WithWindows(slices.Insert(slices.Delete(p.Windows(), from, from+1), to, window)...)
}

func (p Project) checkWindowIndex(index int) error {
	if index < 0 || index >= len(p.windows) {
		return fmt.Errorf("%w: #%d of %d", ErrWindowNotFound, index+1, len(p.windows))
	}
	return nil
}
