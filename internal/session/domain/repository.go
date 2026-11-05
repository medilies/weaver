package domain

// ProjectRepository is the port application use cases depend on to look
// up registered projects. Infrastructure provides the implementation.
type ProjectRepository interface {
	FindByName(name ProjectName) (Project, error)
	// Names lists the registered projects' names, sorted.
	Names() ([]ProjectName, error)
	// Save registers project, replacing a project of the same name.
	Save(project Project) error
	// Delete unregisters the project, or returns ProjectNotFound.
	Delete(name ProjectName) error
}
