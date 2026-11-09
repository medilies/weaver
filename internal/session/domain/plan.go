package domain

import "slices"

// SessionPlan is a project resolved for tmux: every directory absolute,
// every pane's environment merged over its window's and project's, and
// its commands to run first after its project's and window's. Starting a
// session follows it without decisions.
type SessionPlan struct {
	name    ProjectName
	windows []WindowPlan
	focused int
}

// Name is the tmux session name.
func (p SessionPlan) Name() ProjectName {
	return p.name
}

// Windows are created in order.
func (p SessionPlan) Windows() []WindowPlan {
	return slices.Clone(p.windows)
}

// FocusedWindow is the index of the window starting the session lands
// in.
func (p SessionPlan) FocusedWindow() int {
	return p.focused
}

// WindowPlan is one window of a SessionPlan.
type WindowPlan struct {
	name    WindowName
	panes   []PanePlan
	focused int
	grid    Grid
	gridded bool
}

// SizedTo is the window as tmux makes it in cols×rows: a grid with a
// column or row in cells has its splits sized in cells for that size;
// any other window is as it is, its splits in percentages.
func (w WindowPlan) SizedTo(cols, rows int) WindowPlan {
	if !w.gridded || !w.grid.inCells {
		return w
	}
	carved := w.grid.carvedIn(cols, rows)
	w.panes = slices.Clone(w.panes)
	for i := range min(len(w.panes), len(carved)) {
		if split, ok := carved[i].Split(); ok {
			w.panes[i].split = split
		}
	}
	return w
}

// NeedsSize says whether SizedTo changes the window: whether a grid sizes
// a column or row in cells.
func (w WindowPlan) NeedsSize() bool {
	return w.gridded && w.grid.inCells
}

func (w WindowPlan) Name() WindowName {
	return w.name
}

// Panes are created in order; each later one splits an earlier one.
func (w WindowPlan) Panes() []PanePlan {
	return slices.Clone(w.panes)
}

// FocusedPane is the index of the pane the window selects.
func (w WindowPlan) FocusedPane() int {
	return w.focused
}

// PanePlan is one pane of a WindowPlan.
type PanePlan struct {
	name    PaneName
	dir     string
	split   Split
	splits  bool
	env     []EnvVar
	note    []string
	before  []Command
	clears  bool
	startup Startup
	tasks   []Task
}

func (p PanePlan) Name() PaneName {
	return p.name
}

// Dir is the absolute directory the pane's shell starts in.
func (p PanePlan) Dir() string {
	return p.dir
}

// Split is how the pane is created, unless it is its window's first pane.
func (p PanePlan) Split() (Split, bool) {
	return p.split, p.splits
}

// Env is the pane's variables merged over its window's and its project's,
// sorted by name.
func (p PanePlan) Env() []EnvVar {
	return slices.Clone(p.env)
}

// Note is the lines printed before the pane's shell starts, or after its
// before commands when it clears them.
func (p PanePlan) Note() []string {
	return slices.Clone(p.note)
}

// Before is the commands run in the pane's shell before its startup: the
// project's, then the window's, then the pane's own.
func (p PanePlan) Before() []Command {
	return slices.Clone(p.before)
}

// Clears tells whether clear is typed after the pane's before commands,
// then its note printed: when the lowest level setting it sets it on,
// and there are before commands to hide.
func (p PanePlan) Clears() bool {
	return p.clears
}

func (p PanePlan) Startup() Startup {
	return p.startup
}

// Tasks are the commands the pane offers to run on request, in order.
func (p PanePlan) Tasks() []Task {
	return slices.Clone(p.tasks)
}

// Plan resolves the project for tmux.
func (p Project) Plan() SessionPlan {
	windows := make([]WindowPlan, 0, len(p.windows))
	for _, window := range p.windows {
		windows = append(windows, p.planWindow(window))
	}
	return SessionPlan{name: p.name, windows: windows, focused: p.FocusedWindow()}
}

func (p Project) planWindow(window Window) WindowPlan {
	panes := make([]PanePlan, 0, len(window.panes))
	for _, pane := range window.panes {
		panes = append(panes, p.planPane(window, pane))
	}
	return WindowPlan{name: window.name, panes: panes, focused: window.FocusedPane(), grid: window.grid, gridded: window.gridded}
}

func (p Project) planPane(window Window, pane Pane) PanePlan {
	before := slices.Concat(p.before, window.before, pane.before)
	return PanePlan{
		name:    pane.name,
		dir:     p.root.Resolve(window.dirOf(pane)),
		split:   pane.split,
		splits:  pane.splits,
		env:     p.env.Overlay(window.env).Overlay(pane.env).Vars(),
		note:    slices.Clone(pane.note),
		before:  before,
		clears:  len(before) > 0 && pane.clears.Over(window.clears.Over(p.clears)) == ClearingOn,
		startup: pane.startup,
		tasks:   slices.Clone(pane.tasks),
	}
}
