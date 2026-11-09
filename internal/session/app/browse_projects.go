package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrMissingProgram is warned about when a pane's command starts a
// program that can't be found.
var ErrMissingProgram = errors.New("program not found")

// ErrUnclosedQuote is warned about when a command run in a pane leaves a
// quote open, so the shell waits for the rest of it instead of running it.
var ErrUnclosedQuote = errors.New("unclosed quote")

// ProjectOverview is a registered project as the browser shows it: its
// config as written and, when the config is valid, the project, where it
// stands on disk and how its running session differs from it.
type ProjectOverview struct {
	status     ProjectStatus
	config     ConfigDocument
	problem    error
	project    domain.Project
	rootExists bool
	session    domain.RunningSession
	found      bool
	missing    []domain.WindowName
	warnings   []Warning
}

// Name is the project's name.
func (o ProjectOverview) Name() domain.ProjectName {
	return o.status.Name()
}

// Running reports whether the project's session runs.
func (o ProjectOverview) Running() bool {
	return o.status.Running()
}

// Config is the project's config as written; empty when it can't be
// read.
func (o ProjectOverview) Config() ConfigDocument {
	return o.config
}

// Problem is why the project's config can't be read or isn't valid, or
// why its directories couldn't be checked, if so. In the first two cases
// the rest of the overview is empty, except Config when it could be read.
func (o ProjectOverview) Problem() error {
	return o.problem
}

// Project is the project the config describes.
func (o ProjectOverview) Project() domain.Project {
	return o.project
}

// RootExists reports whether the project's root is a directory.
func (o ProjectOverview) RootExists() bool {
	return o.rootExists
}

// Session is the project's running session, or false when none runs.
func (o ProjectOverview) Session() (domain.RunningSession, bool) {
	return o.session, o.found
}

// MissingWindows are the configured windows the running session lacks,
// in config order, which opening the project restores. Empty for a
// session weaver didn't start, which opening leaves as it is.
func (o ProjectOverview) MissingWindows() []domain.WindowName {
	return slices.Clone(o.missing)
}

// Warnings are what would likely go wrong in the panes opening the
// project creates: a directory missing, wrapping ErrMissingDirectory, a
// command whose program can't be found, wrapping ErrMissingProgram, or a
// command run that leaves a quote open, wrapping ErrUnclosedQuote; and in
// any pane, a task whose program can't be found or that leaves a quote
// open; each once, however many panes it concerns. Opening a project that
// isn't running creates every pane, an owned session the missing windows'
// panes, and a session weaver didn't start none.
func (o ProjectOverview) Warnings() []Warning {
	return slices.Clone(o.warnings)
}

// Warning is something opening a project would likely fail on, and the
// panes it concerns. Its message names the first of them.
type Warning struct {
	err   error
	panes []paneAt
}

// paneAt is a pane by its window and its index in it, from 0.
type paneAt struct {
	window domain.WindowName
	pane   int
}

func (w Warning) Error() string {
	if w.err == nil {
		return ""
	}
	return w.err.Error()
}

// Unwrap is ErrMissingDirectory, ErrMissingProgram or ErrUnclosedQuote.
func (w Warning) Unwrap() error {
	return errors.Unwrap(w.err)
}

// Concerns reports whether the warning is about the pane at index (from
// 0) of window.
func (w Warning) Concerns(window domain.WindowName, pane int) bool {
	return slices.Contains(w.panes, paneAt{window: window, pane: pane})
}

// WindowCount is how many of the configured windows the running session
// has, and how many are configured; both zero unless weaver started the
// session, since only then are its windows compared with the config.
func (o ProjectOverview) WindowCount() (present, configured int) {
	if !o.found || !o.session.Owned() {
		return 0, 0
	}
	configured = len(o.project.Windows())
	return configured - len(o.missing), configured
}

// BrowseProjects lets the user act on the registered projects from a
// browser until they open one or quit. After each action, the browser
// shows the projects again, with what happened.
type BrowseProjects struct {
	browser   ProjectBrowser
	confirm   Confirmer
	projects  domain.ProjectRegistry
	documents ProjectDocuments
	workspace domain.Workspace
	sessions  domain.SessionFinder
	list      ListProjects
	open      OpenProject
	freeze    FreezeProject
	edit      EditProject
	kill      KillProject
	run       RunTask
}

// NewBrowseProjects constructs the use case against its ports and the use
// cases the browser offers.
func NewBrowseProjects(
	browser ProjectBrowser,
	confirm Confirmer,
	projects domain.ProjectRegistry,
	documents ProjectDocuments,
	workspace domain.Workspace,
	sessions domain.SessionFinder,
	list ListProjects,
	open OpenProject,
	freeze FreezeProject,
	edit EditProject,
	kill KillProject,
	run RunTask,
) BrowseProjects {
	return BrowseProjects{
		browser: browser, confirm: confirm, projects: projects, documents: documents, workspace: workspace,
		sessions: sessions, list: list, open: open, freeze: freeze, edit: edit, kill: kill, run: run,
	}
}

// Handle shows the browser until the user opens a project, runs a task,
// or quits; a project frozen from it is rooted at workingDir.
func (uc BrowseProjects) Handle(workingDir string) error {
	var selected domain.ProjectName
	notice := ""
	for {
		overviews, err := uc.overviews()
		if err != nil {
			return err
		}
		choice, err := uc.browser.Browse(overviews, selected, notice)
		if errors.Is(err, ErrAborted) {
			return nil
		}
		if err != nil {
			return err
		}
		selected, notice, err = uc.act(choice, workingDir, overviews)
		switch {
		case err == nil && (choice.Action == ActionOpen || choice.Action == ActionRunTask):
			return nil // the user is in the session
		case errors.Is(err, ErrAborted):
			notice = "Nothing done."
		case err != nil:
			notice = err.Error()
		}
	}
}

// overviews are the registered projects, with their configs, where they
// stand on disk and how their running sessions differ from them.
func (uc BrowseProjects) overviews() ([]ProjectOverview, error) {
	statuses, err := uc.list.Handle()
	if err != nil {
		return nil, err
	}
	overviews := make([]ProjectOverview, 0, len(statuses))
	for _, status := range statuses {
		overviews = append(overviews, uc.overview(status))
	}
	return overviews, nil
}

// overview is the project status shows, its config and, when the config
// is valid, where it stands on disk and how its running session differs
// from it.
func (uc BrowseProjects) overview(status ProjectStatus) ProjectOverview {
	overview := ProjectOverview{status: status}
	overview.config, overview.problem = uc.documents.Document(status.Name())
	if overview.problem != nil {
		return overview
	}
	project, err := uc.projects.FindByName(status.Name())
	if err != nil {
		overview.problem = err
		return overview
	}
	rootExists, err := uc.workspace.Exists(project.Root().String())
	if err != nil {
		overview.problem = err
		return overview
	}
	overview.project, overview.rootExists = project, rootExists
	opening := project.Plan().Windows()
	if status.Running() {
		opening = uc.restoration(&overview)
	}
	overview.warnings, overview.problem = uc.warnings(overview, opening, project.Plan().Windows())
	return overview
}

// restoration finds the running session of overview's project and the
// windows it lacks, and returns the ones opening the project creates.
func (uc BrowseProjects) restoration(overview *ProjectOverview) []domain.WindowPlan {
	project := overview.project
	var err error
	// The session only adds to the summary: if tmux can't describe it,
	// the browser shows the project as running, without the comparison.
	overview.session, overview.found, err = uc.sessions.Find(project.Name())
	if err != nil {
		overview.found = false
		return nil
	}
	if !overview.found || !overview.session.Owned() {
		return nil
	}
	var opening []domain.WindowPlan
	for _, placement := range project.Restoration(overview.session.Windows()) {
		overview.missing = append(overview.missing, placement.Window().Name())
		opening = append(opening, placement.Window())
	}
	return opening
}

// warnings are what would likely go wrong in the panes of opening, and
// in the tasks of the panes of all, each warned once with every pane it
// concerns. Directories under a missing root aren't checked: the summary
// already says the root is gone. Nor are the programs of panes whose
// before commands or PATH may change what runs, as a version manager
// does, nor those of tasks in a missing directory, which opening warns
// about.
func (uc BrowseProjects) warnings(overview ProjectOverview, opening, all []domain.WindowPlan) ([]Warning, error) {
	project := overview.project
	// What is checked, a directory or a program, and the warning about it,
	// or -1 when it is fine.
	checked := map[string]int{}
	var warnings []Warning
	warn := func(key string, at paneAt, failed func() bool, err func() error) {
		i, seen := checked[key]
		switch {
		case !seen && failed():
			checked[key] = len(warnings)
			warnings = append(warnings, Warning{err: err(), panes: []paneAt{at}})
		case !seen:
			checked[key] = -1
		case i >= 0:
			warnings[i].panes = append(warnings[i].panes, at)
		}
	}
	program := func(at paneAt, pane domain.PanePlan, cmd domain.Command, whose string) {
		program, ok := runs(pane, cmd)
		if !ok {
			return
		}
		key := "program\x00" + program
		if strings.Contains(program, "/") {
			key += "\x00" + pane.Dir()
		}
		warn(key, at, func() bool {
			return !uc.workspace.Executable(program, pane.Dir())
		}, func() error {
			return fmt.Errorf("%s: %w: %s", whose, ErrMissingProgram, program)
		})
	}
	quote := func(at paneAt, cmd domain.Command, whose string) {
		warn("quote\x00"+cmd.String(), at, cmd.Unclosed, func() error {
			return fmt.Errorf("%s: %w: %s", whose, ErrUnclosedQuote, cmd)
		})
	}
	for _, window := range opening {
		for index, pane := range window.Panes() {
			at := paneAt{window: window.Name(), pane: index}
			for _, cmd := range pane.Before() {
				quote(at, cmd, where(window, pane))
			}
			if cmd, ok := pane.Startup().Command(); ok && pane.Startup().Submits() {
				quote(at, cmd, where(window, pane))
			}
			exists, err := uc.workspace.Exists(pane.Dir())
			if err != nil {
				return nil, err
			}
			if !exists {
				warn("dir\x00"+pane.Dir(), at, func() bool {
					return overview.rootExists || !project.Root().Contains(pane.Dir())
				}, func() error {
					return fmt.Errorf("%s: %w: %s", where(window, pane), ErrMissingDirectory, pane.Dir())
				})
				continue
			}
			if cmd, ok := pane.Startup().Command(); ok {
				program(at, pane, cmd, where(window, pane))
			}
		}
	}
	for _, window := range all {
		for index, pane := range window.Panes() {
			tasks := pane.Tasks()
			if len(tasks) == 0 {
				continue
			}
			at := paneAt{window: window.Name(), pane: index}
			for _, task := range tasks {
				quote(at, task.Command(), fmt.Sprintf("%s, task %q", where(window, pane), task.Label()))
			}
			if exists, err := uc.workspace.Exists(pane.Dir()); err != nil {
				return nil, err
			} else if !exists {
				continue
			}
			for _, task := range tasks {
				whose := fmt.Sprintf("%s, task %q", where(window, pane), task.Label())
				program(at, pane, task.Command(), whose)
			}
		}
	}
	return warnings, nil
}

// runs is the program cmd starts in pane, when weaver can tell what it
// is: not when the pane's before commands or PATH may change what runs.
func runs(pane domain.PanePlan, cmd domain.Command) (string, bool) {
	if len(pane.Before()) > 0 || slices.ContainsFunc(pane.Env(), func(v domain.EnvVar) bool {
		return v.Name() == "PATH"
	}) {
		return "", false
	}
	return cmd.Program()
}

// act carries out choice on one of overviews, and returns the project to
// select next and what to tell the user. Declining to confirm is ErrAborted.
func (uc BrowseProjects) act(choice Choice, workingDir string, overviews []ProjectOverview) (domain.ProjectName, string, error) {
	name := choice.Project
	switch choice.Action {
	case ActionOpen:
		return name, "", uc.open.Handle(name.String())
	case ActionFreeze:
		project, err := uc.freeze.Handle(FreezeRequest{WorkingDir: workingDir})
		if err != nil {
			return name, "", err
		}
		return project.Name(), fmt.Sprintf("Saved %s.", project.Name()), nil
	case ActionEdit:
		if err := uc.edit.Handle(name.String()); err != nil {
			return name, "", err
		}
		return name, fmt.Sprintf("Saved %s.", name), nil
	case ActionEditRaw:
		changed, err := uc.edit.HandleRaw(name.String())
		if err != nil || !changed {
			return name, "No changes.", err
		}
		return name, fmt.Sprintf("Saved %s.", name), nil
	case ActionKill:
		// Don't ask to kill what isn't running. When finding the session
		// fails, ask anyway: killing it then reports the failure.
		if _, found, err := uc.sessions.Find(name); err == nil && !found {
			return name, "", fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
		}
		if err := uc.ask(fmt.Sprintf("Kill %s's session and everything running in it?", name)); err != nil {
			return name, "", err
		}
		return name, fmt.Sprintf("Killed %s.", name), uc.kill.Handle(name.String())
	case ActionDelete:
		if err := uc.ask(fmt.Sprintf("Delete %s's config? A running session keeps running.", name)); err != nil {
			return name, "", err
		}
		if err := uc.projects.Delete(name); err != nil {
			return name, "", err
		}
		return neighbour(overviews, name), fmt.Sprintf("Deleted %s.", name), nil
	case ActionRunTask:
		return name, "", uc.run.Handle(name, choice.Task)
	}
	return name, "", fmt.Errorf("unknown action %d", choice.Action)
}

// neighbour is the project after name in overviews, or before it when
// name was the last, so the selection stays where a deleted project was.
func neighbour(overviews []ProjectOverview, name domain.ProjectName) domain.ProjectName {
	for i, overview := range overviews {
		if overview.Name() != name {
			continue
		}
		if i+1 < len(overviews) {
			return overviews[i+1].Name()
		}
		if i > 0 {
			return overviews[i-1].Name()
		}
	}
	return name
}

// ask returns ErrAborted unless the user answers yes to question.
func (uc BrowseProjects) ask(question string) error {
	yes, err := uc.confirm.Confirm(question)
	if err == nil && !yes {
		return ErrAborted
	}
	return err
}
