package domain

// SessionLauncher starts projects' tmux sessions and puts the user in
// them. Infrastructure provides the implementation.
type SessionLauncher interface {
	// IsRunning reports whether a session named after the project runs.
	IsRunning(name ProjectName) (bool, error)
	// Start creates a detached session from plan. If a step fails, none
	// of the session is left running.
	Start(plan SessionPlan) error
	// Attach puts the user in the project's running session.
	Attach(name ProjectName) error
}

// SessionLister lists running tmux sessions. Infrastructure provides the
// implementation.
type SessionLister interface {
	// Running is the names of the running sessions, projects' or not.
	Running() ([]string, error)
}

// SessionKiller stops projects' tmux sessions. Infrastructure provides the
// implementation.
type SessionKiller interface {
	// Kill stops the project's session and everything running in it, or
	// returns an error wrapping ErrSessionNotRunning.
	Kill(name ProjectName) error
}
