package domain

// Workspace is the filesystem projects live in. Infrastructure provides
// the implementation.
type Workspace interface {
	// Exists reports whether dir is an existing directory.
	Exists(dir string) (bool, error)
	// Clone clones checkout's remote into dir, creating missing parents.
	Clone(checkout Checkout, dir string) error
}
