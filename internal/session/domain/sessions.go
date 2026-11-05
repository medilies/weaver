package domain

import "slices"

// RunningSession is a running tmux session named after a project.
type RunningSession struct {
	owned   bool
	windows []WindowName
}

// NewRunningSession describes a running session: owned when weaver
// started it, and the project windows it still has, in order.
func NewRunningSession(owned bool, windows ...WindowName) RunningSession {
	return RunningSession{owned: owned, windows: slices.Clone(windows)}
}

// Owned reports whether weaver started the session, which makes it safe
// to add the project windows it lacks.
func (s RunningSession) Owned() bool {
	return s.owned
}

// Windows are the project windows the session still has, in order.
func (s RunningSession) Windows() []WindowName {
	return slices.Clone(s.windows)
}

// SessionLauncher starts projects' tmux sessions and puts the user in
// them. Infrastructure provides the implementation.
type SessionLauncher interface {
	// Find reports the running session named after the project, if any.
	Find(name ProjectName) (RunningSession, bool, error)
	// Start creates a detached session from plan. If a step fails, none
	// of the session is left running.
	Start(plan SessionPlan) error
	// Restore adds windows to the project's running session, each where
	// its placement says.
	Restore(name ProjectName, placements []WindowPlacement) error
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
