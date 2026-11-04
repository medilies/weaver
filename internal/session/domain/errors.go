package domain

import "errors"

var (
	// ErrEmptyProjectName is returned when a project name is blank.
	ErrEmptyProjectName = errors.New("project name must not be empty")
	// ErrEmptyProjectRoot is returned when a project root path is blank.
	ErrEmptyProjectRoot = errors.New("project root must not be empty")
)

// ProjectNotFound is returned when a repository has no project under the
// given name.
type ProjectNotFound struct {
	Name ProjectName
}

// NewProjectNotFound constructs a ProjectNotFound for the given name.
func NewProjectNotFound(name ProjectName) ProjectNotFound {
	return ProjectNotFound{Name: name}
}

func (e ProjectNotFound) Error() string {
	return "project not found: " + e.Name.String()
}
