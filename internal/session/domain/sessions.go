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

// SessionFinder looks up projects' running tmux sessions. Infrastructure
// provides the implementation.
type SessionFinder interface {
	// Find reports the running session named after the project, if any.
	Find(name ProjectName) (RunningSession, bool, error)
}

// SessionAttacher puts the user in projects' running tmux sessions.
// Infrastructure provides the implementation.
type SessionAttacher interface {
	// Attach puts the user in the project's running session.
	Attach(name ProjectName) error
}

// SessionLauncher starts projects' tmux sessions and puts the user in
// them. Infrastructure provides the implementation.
type SessionLauncher interface {
	SessionFinder
	SessionAttacher
	// Start creates a detached session from plan. If a step fails, none
	// of the session is left running.
	Start(plan SessionPlan) error
	// Restore adds windows to the project's running session, each where
	// its placement says.
	Restore(name ProjectName, placements []WindowPlacement) error
}

// SessionLister lists running tmux sessions. Infrastructure provides the
// implementation.
type SessionLister interface {
	// Running is the names of the running sessions, projects' or not.
	Running() ([]string, error)
}

// SessionLocator tells which tmux session weaver runs in. Infrastructure
// provides the implementation.
type SessionLocator interface {
	// Current is the session weaver runs in, or ErrNotInsideTmux.
	Current() (string, error)
}

// SessionCapturer reads the layout of a running tmux session, weaver's or
// not. Infrastructure provides the implementation.
type SessionCapturer interface {
	SessionLocator
	// Capture returns the session's windows in order, but for its
	// throwaway windows, their panes in an order that recreates them,
	// with absolute directories and the commands running in them typed
	// at their prompts, not run.
	Capture(session string) ([]Window, error)
}

// SessionKiller finds and kills projects' tmux sessions. Infrastructure
// provides the implementation.
type SessionKiller interface {
	SessionFinder
	// Kill ends the project's session and everything running in it, or
	// returns an error wrapping ErrSessionNotRunning.
	Kill(name ProjectName) error
}

// TaskRunner runs panes' tasks in projects' running sessions, and puts
// the user where they run. A pane is found in the window of its project's
// session tagged with its window's name: the window's only pane, or the
// one labelled with its name. Infrastructure provides the implementation.
type TaskRunner interface {
	SessionFinder
	SessionAttacher
	// Busy reports whether something runs in the pane at index of window,
	// in the project's running session, rather than its shell idling at
	// its prompt; an error wrapping ErrPaneNotRunning when the session
	// lacks the pane.
	Busy(project ProjectName, window WindowPlan, index int) (bool, error)
	// Type types task's command into that pane and runs it, the pane
	// becoming its session's current one.
	Type(project ProjectName, window WindowPlan, index int, task Task) error
	// RunApart runs task in a throwaway window of the project's running
	// session, named after it, starting in pane's dir with its env and
	// before commands, and kept open once the task ends, the window
	// becoming its session's current one. Capture skips
	// throwaway windows, and restoration ignores them.
	RunApart(project ProjectName, pane PanePlan, task Task) error
}
