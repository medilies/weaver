// Command weaver drives tmux sessions, windows and panes from a
// per-project JSON config.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/infra"
)

// application is the use cases the commands run.
type application struct {
	openProject app.OpenProject
	loadProject app.LoadProject
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

	projects := infra.NewJSONProjectRepository(filepath.Join(configDir, "weaver", "projects"))
	tmux := infra.NewTmux(infra.TmuxOptions{Inside: os.Getenv("TMUX") != "", Size: infra.TerminalSize})
	workspace := infra.NewGitWorkspace(os.Stderr)
	runtimes := infra.NewInstalledRuntimes("/usr/bin", nvmDir, filepath.Join(cacheDir, "weaver", "php"))

	return application{
		openProject: app.NewOpenProject(projects, tmux, workspace, runtimes),
		loadProject: app.NewLoadProject(projects),
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
