// Command weave drives tmux sessions, windows and panes from a
// per-project YAML config.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// application is the use cases the commands run.
type application struct {
	addProject     app.AddProject
	browseProjects app.BrowseProjects
	editProject    app.EditProject
	openProject    app.OpenProject
	killProject    app.KillProject
	listProjects   app.ListProjects
	// documents reads configs as stored, for `show`.
	documents app.ProjectDocuments
	// sessions lists running tmux sessions, to complete `add --from`.
	sessions domain.SessionLister
}

// wire builds the use cases against this machine: configs under the user
// config directory and the user's tmux server.
func wire() (application, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return application{}, err
	}

	projects := infra.NewFileProjectRepository(filepath.Join(configDir, "weaver", "projects"))
	tmux := infra.NewTmux(infra.TmuxOptions{Inside: os.Getenv("TMUX") != "", Size: infra.TerminalSize})
	workspace := infra.NewGitWorkspace(os.Stderr)

	forms := infra.NewHuhForms()

	weaver := application{
		addProject:   app.NewAddProject(projects, workspace, tmux, forms, forms),
		editProject:  app.NewEditProject(projects, infra.NewEditor(), forms),
		sessions:     tmux,
		openProject:  app.NewOpenProject(projects, tmux, workspace),
		killProject:  app.NewKillProject(tmux),
		listProjects: app.NewListProjects(projects, tmux),
		documents:    projects,
	}
	weaver.browseProjects = app.NewBrowseProjects(infra.NewTeaBrowser(), forms, projects, projects,
		weaver.listProjects, weaver.openProject, weaver.addProject, weaver.editProject, weaver.killProject)
	return weaver, nil
}

func main() {
	weaver, err := wire()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	root := newRootCommand(weaver)
	root.Version = buildVersion()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
