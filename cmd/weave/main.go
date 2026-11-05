// Command weave drives tmux sessions, windows and panes from a
// per-project JSON or YAML config.
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
	addProject   app.AddProject
	editProject  app.EditProject
	openProject  app.OpenProject
	killProject  app.KillProject
	listProjects app.ListProjects
	loadProject  app.LoadProject
	// sessions lists running tmux sessions, to complete `add --from`.
	sessions domain.SessionLister
}

// wire builds the use cases against this machine: configs under the user
// config directory, the user's tmux server, PHP from /usr/bin and Node.js
// from nvm.
func wire() (application, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return application{}, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return application{}, err
	}
	nvmDir := os.Getenv("NVM_DIR")
	if nvmDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return application{}, err
		}
		nvmDir = filepath.Join(home, ".nvm")
	}

	projects := infra.NewFileProjectRepository(filepath.Join(configDir, "weaver", "projects"))
	tmux := infra.NewTmux(infra.TmuxOptions{Inside: os.Getenv("TMUX") != "", Size: infra.TerminalSize})
	workspace := infra.NewGitWorkspace(os.Stderr)
	runtimes := infra.NewInstalledRuntimes("/usr/bin", nvmDir, filepath.Join(cacheDir, "weaver", "php"))

	forms := infra.NewHuhForms()

	return application{
		addProject:   app.NewAddProject(projects, workspace, runtimes, tmux, forms, forms),
		editProject:  app.NewEditProject(projects, infra.NewEditor(), forms),
		sessions:     tmux,
		openProject:  app.NewOpenProject(projects, tmux, workspace, runtimes),
		killProject:  app.NewKillProject(tmux),
		listProjects: app.NewListProjects(projects, tmux),
		loadProject:  app.NewLoadProject(projects),
	}, nil
}

func main() {
	weaver, err := wire()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := newRootCommand(weaver).Execute(); err != nil {
		os.Exit(1)
	}
}
