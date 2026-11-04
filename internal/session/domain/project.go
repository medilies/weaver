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

// Project is a registered dev project: a name, the directory its session
// is rooted at, and the windows its session opens.
type Project struct {
	name    ProjectName
	root    ProjectRoot
	windows []Window
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

func defaultWindow(name ProjectName) Window {
	return Window{name: WindowName(name), panes: []Pane{NewPane()}}
}

func (p Project) Name() ProjectName {
	return p.name
}

func (p Project) Root() ProjectRoot {
	return p.root
}

// Windows are the windows the project's session opens, in order.
func (p Project) Windows() []Window {
	return slices.Clone(p.windows)
}

// Describe renders a short human-readable summary, e.g. for `weaver show`.
func (p Project) Describe() string {
	return p.name.String() + " (" + p.root.String() + ")"
}
