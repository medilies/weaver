package domain

import (
	"fmt"
	"slices"
)

// RuntimeResolver finds where a pinned toolchain version is installed.
// Infrastructure provides the implementation.
type RuntimeResolver interface {
	// Resolve returns the directory to put first on PATH for runtime, or
	// an error wrapping ErrRuntimeNotInstalled.
	Resolve(runtime Runtime) (string, error)
}

// SessionPlan is a project resolved for tmux: every directory absolute,
// every pane's environment merged over its project's, and every pinned
// version located. Starting a session follows it without decisions.
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
	path    []string
	env     []EnvVar
	note    []string
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

// Path is the directories to put first on the pane's PATH, in order.
func (p PanePlan) Path() []string {
	return slices.Clone(p.path)
}

// Env is the pane's variables merged over its project's, sorted by name.
func (p PanePlan) Env() []EnvVar {
	return slices.Clone(p.env)
}

// Note is the lines printed before the pane's shell starts.
func (p PanePlan) Note() []string {
	return slices.Clone(p.note)
}

func (p PanePlan) Startup() Startup {
	return p.startup
}

// Plan resolves the project for tmux. It locates every pinned version
// through resolver first, so a missing one fails before anything starts.
func (p Project) Plan(resolver RuntimeResolver) (SessionPlan, error) {
	windows := make([]WindowPlan, 0, len(p.windows))
	for _, window := range p.windows {
		planned, err := p.planWindow(window, resolver)
		if err != nil {
			return SessionPlan{}, err
		}
		windows = append(windows, planned)
	}
	return SessionPlan{name: p.name, windows: windows}, nil
}

func (p Project) planWindow(window Window, resolver RuntimeResolver) (WindowPlan, error) {
	panes := make([]PanePlan, 0, len(window.panes))
	for i, pane := range window.panes {
		planned, err := p.planPane(pane, resolver)
		if err != nil {
			return WindowPlan{}, fmt.Errorf("window %q: pane %s: %w", window.name.value, paneRef(i, pane), err)
		}
		panes = append(panes, planned)
	}
	return WindowPlan{name: window.name, panes: panes}, nil
}

func (p Project) planPane(pane Pane, resolver RuntimeResolver) (PanePlan, error) {
	var path []string
	for _, runtime := range p.runtimes.Overlay(pane.runtimes).List() {
		dir, err := resolver.Resolve(runtime)
		if err != nil {
			return PanePlan{}, err
		}
		path = append(path, dir)
	}
	return PanePlan{
		name:    pane.name,
		dir:     p.root.Resolve(pane.dir),
		split:   pane.split,
		splits:  pane.splits,
		path:    path,
		env:     p.env.Overlay(pane.env).Vars(),
		note:    slices.Clone(pane.note),
		startup: pane.startup,
	}, nil
}
