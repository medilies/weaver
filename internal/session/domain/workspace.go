package domain

// Workspace is the filesystem projects live in. Infrastructure provides
// the implementation.
type Workspace interface {
	// Exists reports whether dir is an existing directory.
	Exists(dir string) (bool, error)
	// Clone clones checkout's remote into dir, creating missing parents.
	Clone(checkout Checkout, dir string) error
}

// CheckoutDetector finds the git repositories already in a project root.
// Infrastructure provides the implementation.
type CheckoutDetector interface {
	// Detect finds the repositories at root and in its immediate
	// directories, each with the remote it was cloned from.
	Detect(root ProjectRoot) (Checkouts, error)
}
