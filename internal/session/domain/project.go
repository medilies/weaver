// Package domain holds the Session bounded context's entities, value
// objects and ports. It imports nothing outside the standard library.
package domain

import (
	"regexp"
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

// ProjectRoot is the directory a project's panes and windows are rooted
// at, e.g. the repository checkout.
type ProjectRoot struct {
	path string
}

// NewProjectRoot validates and constructs a ProjectRoot.
func NewProjectRoot(path string) (ProjectRoot, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ProjectRoot{}, ErrEmptyProjectRoot
	}
	return ProjectRoot{path: trimmed}, nil
}

func (r ProjectRoot) String() string {
	return r.path
}

// Project is a registered dev project: a name and the directory its
// session is rooted at.
type Project struct {
	name ProjectName
	root ProjectRoot
}

// NewProject constructs a Project from its already-validated parts.
func NewProject(name ProjectName, root ProjectRoot) Project {
	return Project{name: name, root: root}
}

func (p Project) Name() ProjectName {
	return p.name
}

func (p Project) Root() ProjectRoot {
	return p.root
}

// Describe renders a short human-readable summary, e.g. for `weaver show`.
func (p Project) Describe() string {
	return p.name.String() + " (" + p.root.String() + ")"
}
