package app

import (
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// ProjectOverview is a registered project as the browser shows it.
type ProjectOverview struct {
	status  ProjectStatus
	config  ConfigDocument
	problem error
}

// Name is the project's name.
func (o ProjectOverview) Name() domain.ProjectName {
	return o.status.Name()
}

// Running reports whether the project's session runs.
func (o ProjectOverview) Running() bool {
	return o.status.Running()
}

// Config is the project's config as written.
func (o ProjectOverview) Config() ConfigDocument {
	return o.config
}

// Problem is why the project's config can't be read, if it can't.
func (o ProjectOverview) Problem() error {
	return o.problem
}

// BrowseProjects lets the user act on the registered projects from a
// browser until they open one or quit. After each action, the browser
// shows the projects again, with what happened.
type BrowseProjects struct {
	browser   ProjectBrowser
	confirm   Confirmer
	projects  domain.ProjectRepository
	documents ProjectDocuments
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
	list ListProjects,
	open OpenProject,
	add AddProject,
	edit EditProject,
	kill KillProject,
) BrowseProjects {
	return BrowseProjects{
		browser: browser, confirm: confirm, projects: projects, documents: documents,
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

// overviews are the registered projects, with their configs.
func (uc BrowseProjects) overviews() ([]ProjectOverview, error) {
	statuses, err := uc.list.Handle()
	if err != nil {
		return nil, err
	}
	overviews := make([]ProjectOverview, 0, len(statuses))
	for _, status := range statuses {
		config, err := uc.documents.Document(status.Name())
		overviews = append(overviews, ProjectOverview{status: status, config: config, problem: err})
	}
	return overviews, nil
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
