package domain

import (
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// sessionPrefix starts the name of every project's session.
const sessionPrefix = "w/"

// sessionNameChars are what tmux turns into "_" in session names.
var sessionNameChars = strings.NewReplacer(".", "_", ":", "_")

// ProjectRef is how the session ports tell a project apart: its root,
// which identifies it, and its name, which names its session.
type ProjectRef struct {
	root ProjectRoot
	name ProjectName
}

// NewProjectRef refers to the project named name, rooted at root.
func NewProjectRef(root ProjectRoot, name ProjectName) ProjectRef {
	return ProjectRef{root: root, name: name}
}

func (r ProjectRef) Root() ProjectRoot {
	return r.root
}

func (r ProjectRef) Name() ProjectName {
	return r.name
}

// String is the project's name, as messages name it.
func (r ProjectRef) String() string {
	return r.name.String()
}

// SessionNames are the names the project's session is given: usual, "w/"
// and its name, else, when another session has that one, apart, "w/" and
// its root. tmux turns "." and ":" in session names into "_", and so do
// they.
func (r ProjectRef) SessionNames() (usual, apart string) {
	return sessionNameChars.Replace(sessionPrefix + r.name.String()),
		sessionNameChars.Replace(sessionPrefix + strings.TrimPrefix(r.root.String(), "/"))
}

// SessionAmong is the project's session among sessions: the one weaver
// started for its root, else one started in its root under one of its
// session names that weaver didn't start for another project.
func (r ProjectRef) SessionAmong(sessions []ListedSession) (ListedSession, bool) {
	for _, s := range sessions {
		if s.startedFor == r.root.String() {
			return s, true
		}
	}
	usual, apart := r.SessionNames()
	for _, s := range sessions {
		if s.startedFor == "" && (s.name == usual || s.name == apart) && filepath.Clean(s.path) == r.root.String() {
			return s, true
		}
	}
	return ListedSession{}, false
}

// ListedSession is a running tmux session as SessionLister lists it: its
// name, the directory it started in, the root of the project weaver
// started it for, if it did, and when it was last attached, or started.
type ListedSession struct {
	name       string
	path       string
	startedFor string
	active     time.Time
}

// NewListedSession is the session named name, started in path.
func NewListedSession(name, path string) ListedSession {
	return ListedSession{name: name, path: path}
}

// StartedFor is s, started by weaver for the project rooted at root.
func (s ListedSession) StartedFor(root string) ListedSession {
	s.startedFor = root
	return s
}

// ActiveAt is s, last attached, or else started, at active.
func (s ListedSession) ActiveAt(active time.Time) ListedSession {
	s.active = active
	return s
}

func (s ListedSession) Name() string {
	return s.name
}

// Path is the directory the session started in.
func (s ListedSession) Path() string {
	return s.path
}

// Owned reports whether weaver started the session.
func (s ListedSession) Owned() bool {
	return s.startedFor != ""
}

// Root is the root of the project weaver started the session for; empty
// when it didn't.
func (s ListedSession) Root() string {
	return s.startedFor
}

// Active is when the session was last attached, or else started.
func (s ListedSession) Active() time.Time {
	return s.active
}

// RunningSession is a project's running tmux session.
type RunningSession struct {
	name    string
	owned   bool
	windows []WindowName
}

// NewRunningSession describes a running session: owned when weaver
// started it, and the project windows it still has, in order.
func NewRunningSession(owned bool, windows ...WindowName) RunningSession {
	return RunningSession{owned: owned, windows: slices.Clone(windows)}
}

// Named is s, named name in tmux.
func (s RunningSession) Named(name string) RunningSession {
	s.name = name
	return s
}

// Name is the session's name in tmux.
func (s RunningSession) Name() string {
	return s.name
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
	// Find reports the project's running session, if any, as
	// ProjectRef.SessionAmong finds it.
	Find(project ProjectRef) (RunningSession, bool, error)
}

// CommandTracker finds the commands weaver typed into projects' running
// sessions that still run. Infrastructure provides the implementation.
type CommandTracker interface {
	// RunningCommands are the commands TaskRunner typed for the panes of
	// windows in the project's running session that still run: in the
	// pane, or in a throwaway window it ran apart from the pane in. An
	// error wrapping ErrSessionNotRunning when no session runs.
	RunningCommands(project ProjectRef, windows []WindowPlan) ([]RunningCommand, error)
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
	Idle(project ProjectRef, windows []WindowPlan) (map[WindowName][]int, error)
	// Preview is window in the project's running session as it is now:
	// the window tagged with its name, only its zoomed pane when it has
	// one, each pane showing the bottom of its screen, but for the one
	// weaver runs in, and matched to window's panes as TaskRunner finds
	// them. An error wrapping ErrSessionNotRunning when no session runs,
	// or ErrWindowNotRunning when the session lacks the window, or loses
	// it or a pane while it is read.
	Preview(project ProjectRef, window WindowPlan) (WindowPreview, error)
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
	Attach(project ProjectRef) error
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
	Restore(project ProjectRef, placements []WindowPlacement) error
}

// SessionReloader rebuilds panes and windows of projects' running
// sessions as their configs say now, ending what runs in them.
// Infrastructure provides the implementation.
type SessionReloader interface {
	SessionFinder
	// ReloadPane restarts the pane at index of window in place, its size
	// and position kept, as opening the project starts it. An error
	// wrapping ErrPaneNotRunning when the session lacks the pane, or
	// ErrPaneRunsWeaver when weaver runs in it.
	ReloadPane(project ProjectRef, window WindowPlan, index int) error
	// ReloadWindow replaces window, in the project's running session,
	// with one built as opening the project builds it, at the same index,
	// and the current window still when it was. The throwaway windows of
	// its tasks are left alone. An error wrapping ErrWindowNotRunning when
	// the session lacks it, or ErrPaneRunsWeaver when weaver runs in one
	// of its panes.
	ReloadWindow(project ProjectRef, window WindowPlan) error
	// Restore adds windows to the project's running session, each where
	// its placement says, as SessionLauncher does.
	Restore(project ProjectRef, placements []WindowPlacement) error
}

// SessionLister lists running tmux sessions. Infrastructure provides the
// implementation.
type SessionLister interface {
	// Running is the running sessions, projects' or not.
	Running() ([]ListedSession, error)
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
	// Adopt renames the session to the project's session name, as Start
	// names a session, so that it counts as the project's, when it started
	// in the project's root; it stays one weaver didn't start. Any other is
	// left as it is.
	Adopt(session string, project ProjectRef) error
}

// SessionKiller finds and kills projects' tmux sessions. Infrastructure
// provides the implementation.
type SessionKiller interface {
	SessionFinder
	SessionLister
	// Kill ends the project's session and everything running in it, or
	// returns an error wrapping ErrSessionNotRunning.
	Kill(project ProjectRef) error
	// InterruptPanes types ctrl+c into each pane of the project's session
	// whose shell isn't idle at its prompt, throwaway windows' included,
	// but the one weaver runs in, as if the user pressed it there; then
	// waits, up to within, until their shells are idle. It returns the
	// panes still busy then, each as its window's name, followed by " › "
	// and its own when it has one; or an error wrapping
	// ErrSessionNotRunning.
	InterruptPanes(project ProjectRef, within time.Duration) ([]string, error)
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
	Busy(project ProjectRef, window WindowPlan, index int) (bool, error)
	// Type types cmd into that pane and runs it: a task's command, or the
	// pane's own run again. Landing, the pane becomes its session's
	// current one, where attaching lands; otherwise the session's current
	// window and pane stay.
	Type(project ProjectRef, window WindowPlan, index int, cmd Command, landing bool) error
	// RunApart runs task, a task of the pane at index of window, in a
	// throwaway window of the project's running session, named after it,
	// starting in the pane's dir with its env and before commands, and
	// kept open once the task ends. Landing, the window becomes its
	// session's current one, as Type's pane does. Capture skips throwaway
	// windows, and restoration ignores them.
	RunApart(project ProjectRef, window WindowPlan, index int, task Task, landing bool) error
	CommandTracker
	// Reach makes the pane run runs in, its pane's or a throwaway
	// window's, its session's current one, where attaching lands; an
	// error wrapping ErrPaneNotRunning once it is gone.
	Reach(project ProjectRef, run RunningCommand) error
	// Interrupt types ctrl+c into the pane at index of window, in the
	// project's running session, as if the user pressed it there; an
	// error wrapping ErrPaneNotRunning when the session lacks the pane,
	// or ErrPaneRunsWeaver when weaver runs in it.
	Interrupt(project ProjectRef, window WindowPlan, index int) error
	// InterruptRun types ctrl+c into the pane run runs in, as Interrupt
	// does; an error wrapping ErrPaneNotRunning once it is gone.
	InterruptRun(project ProjectRef, run RunningCommand) error
}
