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

// ProjectOverview is a registered project as the browser shows it: its
// config as written and, when the config is valid, the project, where it
// stands on disk and how its running session differs from it.
type ProjectOverview struct {
	status     ProjectStatus
	config     ConfigDocument
	problem    error
	project    domain.Project
	rootExists bool
	checkouts  []CheckoutStatus
	tree       domain.WorkingTree
	inGit      bool
	session    domain.RunningSession
	found      bool
	missing    []domain.WindowName
	warnings   []error
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

// Problem is why the project's config can't be read or isn't valid, if
// so; the rest of the overview is then empty, except Config when it
// could be read.
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

// Checkouts are the project's repositories, in clone order, and whether
// each is cloned.
func (o ProjectOverview) Checkouts() []CheckoutStatus {
	return slices.Clone(o.checkouts)
}

// WorkingTree is where the git repository the root is in stands, or
// false when it isn't in one.
func (o ProjectOverview) WorkingTree() (domain.WorkingTree, bool) {
	return o.tree, o.inGit
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
// project creates: a directory missing that no clone creates, wrapping
// ErrMissingDirectory, or a command whose program can't be found,
// wrapping ErrMissingProgram. Opening a stopped project creates every
// pane, a session weaver started the missing windows' panes, and one it
// didn't start none.
func (o ProjectOverview) Warnings() []error {
	return slices.Clone(o.warnings)
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

// CheckoutStatus is a project's repository and whether it is cloned.
type CheckoutStatus struct {
	checkout domain.Checkout
	cloned   bool
}

func (s CheckoutStatus) Checkout() domain.Checkout {
	return s.checkout
}

// Cloned reports whether the repository's directory exists.
func (s CheckoutStatus) Cloned() bool {
	return s.cloned
}

// BrowseProjects lets the user act on the registered projects from a
// browser until they open one or quit. After each action, the browser
// shows the projects again, with what happened.
type BrowseProjects struct {
	browser   ProjectBrowser
	confirm   Confirmer
	projects  domain.ProjectRepository
	documents ProjectDocuments
	workspace domain.Workspace
	sessions  domain.SessionFinder
	list      ListProjects
	open      OpenProject
	add       AddProject
	edit      EditProject
	kill      KillProject
}

// NewBrowseProjects constructs the use case against its ports and the use
// cases the browser offers.
func NewBrowseProjects(
	browser ProjectBrowser,
	confirm Confirmer,
	projects domain.ProjectRepository,
	documents ProjectDocuments,
	workspace domain.Workspace,
	sessions domain.SessionFinder,
	list ListProjects,
	open OpenProject,
	add AddProject,
	edit EditProject,
	kill KillProject,
) BrowseProjects {
	return BrowseProjects{
		browser: browser, confirm: confirm, projects: projects, documents: documents, workspace: workspace,
		sessions: sessions, list: list, open: open, add: add, edit: edit, kill: kill,
	}
}

// Handle shows the browser until the user opens a project or quits; a
// project added from it is rooted at workingDir.
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
		selected, notice, err = uc.act(choice, workingDir)
		switch {
		case err == nil && choice.Action == ActionOpen:
			return nil
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
	var checkouts []CheckoutStatus
	for _, checkout := range project.Checkouts().List() {
		cloned, err := uc.workspace.Exists(checkout.Path(project.Root()))
		if err != nil {
			overview.problem = err
			return overview
		}
		checkouts = append(checkouts, CheckoutStatus{checkout: checkout, cloned: cloned})
	}
	overview.project, overview.rootExists, overview.checkouts = project, rootExists, checkouts
	if rootExists {
		overview.tree, overview.inGit = uc.workspace.WorkingTree(project.Root().String())
	}
	opening := project.Plan().Windows()
	if status.Running() {
		opening = uc.restoration(&overview)
	}
	overview.warnings, overview.problem = uc.warnings(overview, opening, !status.Running())
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

// warnings are what would likely go wrong in the panes of windows, once
// overview's missing repositories are cloned when cloning, as starting a
// session does (restoring windows doesn't clone). Directories under a missing
// root aren't checked: the summary already says the root is gone. Nor are
// the programs of panes whose before commands or PATH may change what
// runs, as a version manager does.
func (uc BrowseProjects) warnings(overview ProjectOverview, windows []domain.WindowPlan, cloning bool) ([]error, error) {
	project := overview.project
	seen := map[string]bool{}
	var warnings []error
	for _, window := range windows {
		for _, pane := range window.Panes() {
			exists, err := uc.workspace.Exists(pane.Dir())
			if err != nil {
				return nil, err
			}
			if !exists {
				if !seen[pane.Dir()] && (!cloning || !overview.clones(pane.Dir())) &&
					(overview.rootExists || !project.Root().Contains(pane.Dir())) {
					warnings = append(warnings, fmt.Errorf("%s: %w: %s", where(window, pane), ErrMissingDirectory, pane.Dir()))
				}
				seen[pane.Dir()] = true
				continue
			}
			program, ok := runs(pane)
			if !ok {
				continue
			}
			key := program
			if strings.Contains(program, "/") {
				key = pane.Dir() + "\x00" + program
			}
			if !seen[key] && !uc.workspace.Executable(program, pane.Dir()) {
				warnings = append(warnings, fmt.Errorf("%s: %w: %s", where(window, pane), ErrMissingProgram, program))
			}
			seen[key] = true
		}
	}
	return warnings, nil
}

// clones reports whether opening the project clones a repository dir is
// in, which would create it.
func (o ProjectOverview) clones(dir string) bool {
	for _, status := range o.checkouts {
		if !status.cloned && status.checkout.Contains(o.project.Root(), dir) {
			return true
		}
	}
	return false
}

// runs is the program pane's command starts, run or left at the prompt,
// when weaver can tell what it is.
func runs(pane domain.PanePlan) (string, bool) {
	cmd, ok := pane.Startup().Command()
	if !ok || len(pane.Before()) > 0 || slices.ContainsFunc(pane.Env(), func(v domain.EnvVar) bool {
		return v.Name() == "PATH"
	}) {
		return "", false
	}
	return cmd.Program()
}

// act carries out choice and returns the project to select next and what
// to tell the user. Declining to confirm is ErrAborted.
func (uc BrowseProjects) act(choice Choice, workingDir string) (domain.ProjectName, string, error) {
	name := choice.Project
	switch choice.Action {
	case ActionOpen:
		return name, "", uc.open.Handle(name.String())
	case ActionAdd:
		project, err := uc.add.Handle(AddRequest{WorkingDir: workingDir})
		if err != nil {
			return name, "", err
		}
		return project.Name(), fmt.Sprintf("Saved %s.", project.Name()), nil
	case ActionEdit:
		changed, err := uc.edit.Handle(name.String())
		if err != nil || !changed {
			return name, "No changes.", err
		}
		return name, fmt.Sprintf("Saved %s.", name), nil
	case ActionKill:
		if err := uc.ask(fmt.Sprintf("Stop %s's session and everything running in it?", name)); err != nil {
			return name, "", err
		}
		return name, fmt.Sprintf("Stopped %s.", name), uc.kill.Handle(name.String())
	case ActionDelete:
		if err := uc.ask(fmt.Sprintf("Delete %s's config? A running session keeps running.", name)); err != nil {
			return name, "", err
		}
		return name, fmt.Sprintf("Deleted %s.", name), uc.projects.Delete(name)
	}
	return name, "", fmt.Errorf("unknown action %d", choice.Action)
}

// ask returns ErrAborted unless the user answers yes to question.
func (uc BrowseProjects) ask(question string) error {
	yes, err := uc.confirm.Confirm(question)
	if err == nil && !yes {
		return ErrAborted
	}
	return err
}
