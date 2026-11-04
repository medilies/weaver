package domain

import "errors"

var (
	// ErrEmptyProjectName is returned when a project name is blank.
	ErrEmptyProjectName = errors.New("project name must not be empty")
	// ErrInvalidProjectName is returned when a project name has characters
	// a filename, a tmux session name or a command-line argument can't
	// carry safely.
	ErrInvalidProjectName = errors.New("project name must start with a letter or digit and contain only letters, digits, '-' and '_'")
	// ErrEmptyProjectRoot is returned when a project root path is blank.
	ErrEmptyProjectRoot = errors.New("project root must not be empty")
	// ErrRelativeProjectRoot is returned when a project root is not an
	// absolute path: `weaver <project>` runs from anywhere.
	ErrRelativeProjectRoot = errors.New("project root must be an absolute path")
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
