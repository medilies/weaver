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
