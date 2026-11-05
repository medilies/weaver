package domain

import "slices"

// SessionPlan is a project resolved for tmux: every directory absolute,
// every pane's environment merged over its project's, and its commands to
// run first after its project's and window's. Starting a session follows
// it without decisions.
type SessionPlan struct {
	name    ProjectName
	windows []WindowPlan
}

// Name is the tmux session name.
func (p SessionPlan) Name() ProjectName {
	return p.name
}

// Windows are created in order.
func (p SessionPlan) Windows() []WindowPlan {
	return slices.Clone(p.windows)
}

// WindowPlan is one window of a SessionPlan.
type WindowPlan struct {
	name  WindowName
	panes []PanePlan
}

func (w WindowPlan) Name() WindowName {
	return w.name
}

// Panes are created in order; each later one splits an earlier one.
func (w WindowPlan) Panes() []PanePlan {
	return slices.Clone(w.panes)
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
	startup Startup
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

// Env is the pane's variables merged over its project's, sorted by name.
func (p PanePlan) Env() []EnvVar {
	return slices.Clone(p.env)
}

// Note is the lines printed before the pane's shell starts.
func (p PanePlan) Note() []string {
	return slices.Clone(p.note)
}

// Before is the commands run in the pane's shell before its startup: the
// project's, then the window's, then the pane's own.
func (p PanePlan) Before() []Command {
	return slices.Clone(p.before)
}

func (p PanePlan) Startup() Startup {
	return p.startup
}

// Plan resolves the project for tmux.
func (p Project) Plan() SessionPlan {
	windows := make([]WindowPlan, 0, len(p.windows))
	for _, window := range p.windows {
		windows = append(windows, p.planWindow(window))
	}
	return SessionPlan{name: p.name, windows: windows}
}

func (p Project) planWindow(window Window) WindowPlan {
	panes := make([]PanePlan, 0, len(window.panes))
	for _, pane := range window.panes {
		panes = append(panes, p.planPane(window, pane))
	}
	return WindowPlan{name: window.name, panes: panes}
}

func (p Project) planPane(window Window, pane Pane) PanePlan {
	return PanePlan{
		name:    pane.name,
		dir:     p.root.Resolve(pane.dir),
		split:   pane.split,
		splits:  pane.splits,
		env:     p.env.Overlay(pane.env).Vars(),
		note:    slices.Clone(pane.note),
		before:  slices.Concat(p.before, window.before, pane.before),
		startup: pane.startup,
	}
}
