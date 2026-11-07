package domain

// Workspace is the filesystem projects live in. Infrastructure provides
// the implementation.
type Workspace interface {
	// Exists reports whether dir is an existing directory.
	Exists(dir string) (bool, error)
	// Executable reports whether program can be run from dir: found on
	// PATH or, when it holds a slash, at that path from dir.
	Executable(program, dir string) bool
}
