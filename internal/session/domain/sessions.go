package domain

import (
	"slices"
	"time"
)

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

// CommandTracker finds the commands weaver typed into projects' running
// sessions that still run. Infrastructure provides the implementation.
type CommandTracker interface {
	// RunningCommands are the commands TaskRunner typed for the panes of
	// windows in the project's running session that still run: in the
	// pane, or in a throwaway window it ran apart from the pane in. An
	// error wrapping ErrSessionNotRunning when no session runs.
	RunningCommands(project ProjectName, windows []WindowPlan) ([]RunningCommand, error)
}

// SessionInspector finds projects' running tmux sessions and looks into
// their panes. Infrastructure provides the implementation.
type SessionInspector interface {
	SessionFinder
	CommandTracker
	// Idle is, by window, the indexes of the panes of windows whose shell
	// idles at its prompt in the project's running session, nothing
	// running in its terminal's foreground. A pane is found as TaskRunner
	// finds it; one the session lacks isn't idle. An error wrapping
	// ErrSessionNotRunning when no session runs.
	Idle(project ProjectName, windows []WindowPlan) (map[WindowName][]int, error)
	// Preview is window in the project's running session as it is now:
	// the window tagged with its name, only its zoomed pane when it has
	// one, each pane showing the bottom of its screen, but for the one
	// weaver runs in, and matched to window's panes as TaskRunner finds
	// them. An error wrapping ErrSessionNotRunning when no session runs,
	// or ErrWindowNotRunning when the session lacks the window, or loses
	// it or a pane while it is read.
	Preview(project ProjectName, window WindowPlan) (WindowPreview, error)
}

// RunningCommand is a command weaver typed for a pane of a project's
// running session, still running there or in a throwaway window: a copy
// of it, when the same command runs in several.
type RunningCommand struct {
	window  WindowName
	pane    int
	command Command
	place   string
	apart   bool
	since   time.Time
}

// NewRunningCommand is command, typed for the pane at index pane of the
// window named window, running.
func NewRunningCommand(window WindowName, pane int, command Command) RunningCommand {
	return RunningCommand{window: window, pane: pane, command: command}
}

// RunningIn is r running in place, how the session's adapter finds the
// pane it runs in, apart when that is a throwaway window's, and typed at
// since.
func (r RunningCommand) RunningIn(place string, apart bool, since time.Time) RunningCommand {
	r.place, r.apart, r.since = place, apart, since
	return r
}

// Window is the name of the window of the pane the command was typed for.
func (r RunningCommand) Window() WindowName {
	return r.window
}

// Pane is the index of that pane in its window.
func (r RunningCommand) Pane() int {
	return r.pane
}

// Place is how the session's adapter finds the pane the command runs in.
func (r RunningCommand) Place() string {
	return r.place
}

// Apart reports whether the command runs in a throwaway window rather
// than in the pane it was typed for.
func (r RunningCommand) Apart() bool {
	return r.apart
}

// Since is when the command was typed; the zero time when unknown.
func (r RunningCommand) Since() time.Time {
	return r.since
}

// Of reports whether r is cmd, typed for the pane at index pane of the
// window named window.
func (r RunningCommand) Of(window WindowName, pane int, cmd Command) bool {
	return r.window == window && r.pane == pane && r.Runs(cmd)
}

// Runs reports whether it is cmd that runs.
func (r RunningCommand) Runs(cmd Command) bool {
	return r.command.String() == cmd.String()
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
	// lacks the pane, or ErrPaneRunsWeaver when weaver runs in it.
	Busy(project ProjectName, window WindowPlan, index int) (bool, error)
	// Type types cmd into that pane and runs it: a task's command, or the
	// pane's own run again. Landing, the pane becomes its session's
	// current one, where attaching lands; otherwise the session's current
	// window and pane stay.
	Type(project ProjectName, window WindowPlan, index int, cmd Command, landing bool) error
	// RunApart runs task, a task of the pane at index of window, in a
	// throwaway window of the project's running session, named after it,
	// starting in the pane's dir with its env and before commands, and
	// kept open once the task ends. Landing, the window becomes its
	// session's current one, as Type's pane does. Capture skips throwaway
	// windows, and restoration ignores them.
	RunApart(project ProjectName, window WindowPlan, index int, task Task, landing bool) error
	CommandTracker
	// Reach makes the pane run runs in, its pane's or a throwaway
	// window's, its session's current one, where attaching lands; an
	// error wrapping ErrPaneNotRunning once it is gone.
	Reach(project ProjectName, run RunningCommand) error
}
