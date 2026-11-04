package domain

import "errors"

var (
	// ErrEmptyProjectName is returned when a project name is blank.
	ErrEmptyProjectName = errors.New("project name must not be empty")
	// ErrInvalidProjectName is returned when a project name contains a
	// path separator: it doubles as a filename and a tmux session name,
	// neither of which can contain one.
	ErrInvalidProjectName = errors.New("project name must not contain a path separator")
	// ErrEmptyProjectRoot is returned when a project root path is blank.
	ErrEmptyProjectRoot = errors.New("project root must not be empty")
)

// ProjectNotFound is returned when a repository has no project under the
// given name.
type ProjectNotFound struct {
	name ProjectName
}

// NewProjectNotFound constructs a ProjectNotFound for the given name.
func NewProjectNotFound(name ProjectName) ProjectNotFound {
	return ProjectNotFound{name: name}
}

// Name is the project name that could not be found.
func (e ProjectNotFound) Name() ProjectName {
	return e.name
}

func (e ProjectNotFound) Error() string {
	return "project not found: " + e.name.String()
}
