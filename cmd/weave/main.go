// Command weave drives tmux sessions, windows and panes from a
// per-project YAML config.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// application is the use cases the commands run.
type application struct {
	bindProject    app.BindProject
	browseProjects app.BrowseProjects
	editProject    app.EditProject
	openProject    app.OpenProject
	killProject    app.KillProject
	listProjects   app.ListProjects
	// sessions lists running tmux sessions, to complete `bind`.
	sessions domain.SessionLister
	// settings reads the settings, once.
	settings func() (infra.Settings, error)
}

// readSettings stops a command showing the confirmation dialog, the
// config form or the browser before it does anything, while the settings
// are invalid, rather than once the user has typed what would be lost.
func (a application) readSettings() error {
	if a.settings == nil {
		return nil
	}
	_, err := a.settings()
	return err
}

// wire builds the use cases against this machine: settings and configs
// under the user config directory, and the user's tmux server. Invalid
// settings stop only what shows the confirmation dialog, the config form
// or the browser.
func wire() (application, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return application{}, err
	}

	weaverDir := filepath.Join(configDir, "weaver")
	settings := sync.OnceValues(func() (infra.Settings, error) { return infra.LoadSettings(weaverDir, os.Stderr) })
	projects := infra.NewFileProjectRegistry(filepath.Join(weaverDir, "projects")).WithWarnings(os.Stderr)
	// TMUX= weave, to attach from inside tmux, keeps $TMUX_PANE: it only
	// counts with $TMUX, which names the server it belongs to. A popup or
	// run-shell has $TMUX but no pane.
	inside := os.Getenv("TMUX") != ""
	var pane string
	if inside {
		pane = os.Getenv("TMUX_PANE")
	}
	tmux := infra.NewTmux(infra.TmuxOptions{Inside: inside, Pane: pane, Size: infra.TerminalSize})
	workspace := infra.NewLocalWorkspace()
	commands := infra.NewShellRunner(os.Stdin, os.Stdout, os.Stderr)

	confirm := lazyConfirmer(builtOnce(settings, infra.NewTeaConfirmer))
	configForm := lazyConfigForm(builtOnce(settings, func(s infra.Settings) infra.TeaConfigForm {
		return infra.NewTeaConfigForm(workspace, projects, s)
	}))

	weaver := application{
		bindProject:  app.NewBindProject(projects, tmux, workspace, configForm),
		editProject:  app.NewEditProject(projects, projects, configForm, infra.NewEditor(), confirm),
		sessions:     tmux,
		openProject:  app.NewOpenProject(projects, tmux, workspace, commands),
		killProject:  app.NewKillProject(tmux, tmux, projects, commands, infra.NewLineAcknowledger(os.Stdin, os.Stderr)),
		listProjects: app.NewListProjects(projects, tmux),
		settings:     settings,
	}
	// The browser asks its own questions, in its status line while on screen.
	browser := lazyBrowser(builtOnce(settings, infra.NewTeaBrowser))
	weaver.browseProjects = app.NewBrowseProjects(browser, projects, projects, workspace, tmux,
		weaver.listProjects, weaver.openProject, weaver.bindProject, weaver.editProject, weaver.killProject,
		app.NewRunTask(projects, tmux, browser), app.NewRerunCommand(projects, tmux))
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
