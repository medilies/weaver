package app

import (
	"errors"
	"fmt"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// ProjectOverview is a registered project as the browser shows it: its
// config as written and, when the config is valid, the project and where
// it stands on disk.
type ProjectOverview struct {
	status     ProjectStatus
	config     ConfigDocument
	problem    error
	project    domain.Project
	rootExists bool
	checkouts  []CheckoutStatus
	tree       domain.WorkingTree
	inGit      bool
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
	list ListProjects,
	open OpenProject,
	add AddProject,
	edit EditProject,
	kill KillProject,
) BrowseProjects {
	return BrowseProjects{
		browser: browser, confirm: confirm, projects: projects, documents: documents, workspace: workspace,
		list: list, open: open, add: add, edit: edit, kill: kill,
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

// overviews are the registered projects, with their configs and where
// they stand on disk.
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
// is valid, where it stands on disk.
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
	return overview
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
