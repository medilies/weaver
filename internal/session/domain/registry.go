package domain

// ProjectRegistry is the port application use cases depend on to look
// up registered projects. Infrastructure provides the implementation.
type ProjectRegistry interface {
	// Registered is the registered projects, in the settings' order, each
	// root once, those whose config is missing or invalid included.
	Registered() ([]ProjectRef, error)
	// Find is the project registered at root; ErrProjectNotFound when
	// root isn't registered, an error wrapping ErrInvalidConfig when its
	// config can't be read or isn't valid.
	Find(root ProjectRoot) (Project, error)
	// Save writes project's config: to config, relative to its root or
	// absolute, when given, else to its registered config, else to
	// weaver.yml in its root. A root not registered yet is registered
	// first in the list, and one registered with another config is
	// pointed at config.
	Save(project Project, config string) error
	// Delete unregisters the project at root, leaving its config, or
	// returns ErrProjectNotFound.
	Delete(root ProjectRoot) error
}
