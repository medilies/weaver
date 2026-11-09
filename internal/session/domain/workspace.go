package domain

// Workspace is the filesystem projects live in. Infrastructure provides
// the implementation.
type Workspace interface {
	// Exists reports whether dir is an existing directory.
	Exists(dir string) (bool, error)
	// Executable reports whether program can be run from dir: found on
	// PATH or, when it holds a slash, at that path from dir.
	Executable(program, dir string) bool
	// RealPath is path with its symlinks resolved, or path as is when it
	// can't be, as when it doesn't exist.
	RealPath(path string) string
}

// CommandRunner runs commands outside any session, as a project's setup
// and teardown commands. Infrastructure provides the implementation.
type CommandRunner interface {
	// Run runs cmd in dir with env added to weaver's own environment,
	// letting the user see its output, and waits for it to end. When it
	// fails or is interrupted, the error wraps ErrCommandFailed and ends
	// with the last lines it printed as errors.
	Run(cmd Command, dir string, env Env) error
}
