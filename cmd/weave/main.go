// Command weave drives tmux sessions, windows and panes from a
// per-project YAML config.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// application is the use cases the commands run.
type application struct {
	bindProject    app.BindProject
	browseProjects app.BrowseProjects
	editProject    app.EditProject
	findProject    app.FindProject
	openProject    app.OpenProject
	killProject    app.KillProject
	listProjects   app.ListProjects
	// sessions lists running tmux sessions, to complete `bind`.
	sessions domain.SessionLister
	// settings reads the settings, once.
	settings func() (infra.Settings, error)
	// popup runs weave again in a tmux popup, when the settings ask for
	// one and weave runs in a tmux pane, and reports whether it did.
	popup func() (bool, error)
}

// inPopup runs weave again in a tmux popup, as popup does; never without
// one.
func (a application) inPopup() (bool, error) {
	if a.popup == nil {
		return false, nil
	}
	return a.popup()
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

// wire builds the use cases against this machine: the settings, listing
// the projects' roots, under the user config directory, the configs in
// those roots, and the user's tmux server. Invalid
// settings stop only what shows the confirmation dialog, the config form
// or the browser.
func wire() (application, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return application{}, err
	}

	weaverDir := filepath.Join(configDir, "weaver")
	settings := sync.OnceValues(func() (infra.Settings, error) { return infra.LoadSettings(weaverDir, os.Stderr) })
	// Warnings wait while the browser is on screen, which they would
	// show over.
	warnings := infra.NewHeldOutput(os.Stderr)
	projects := infra.NewFileProjectRegistry(infra.SettingsFile(weaverDir)).
		WithWarnings(warnings, filepath.Join(weaverDir, "projects")).
		WithSchema(infra.SchemaURL(buildVersion()))
	// TMUX= weave, to attach from inside tmux, keeps $TMUX_PANE: it only
	// counts with $TMUX, which names the server it belongs to. A popup or
	// run-shell has $TMUX but no pane.
	inside := os.Getenv("TMUX") != ""
	var pane string
	if inside {
		pane = os.Getenv("TMUX_PANE")
	}
	// Invalid settings leave tmux's limit as it is, rather than stop
	// commands that show no screen.
	limit := func() time.Duration {
		if s, err := settings(); err == nil {
			return s.TmuxTimeout()
		}
		return 0
	}
	tmux := infra.NewTmux(infra.TmuxOptions{Inside: inside, Pane: pane, Size: infra.TerminalSize, Limit: limit})
	workspace := infra.NewLocalWorkspace()
	commands := infra.NewShellRunner(os.Stdin, os.Stdout, os.Stderr)

	confirm := lazyConfirmer(builtOnce(settings, infra.NewTeaConfirmer))
	configForm := lazyConfigForm(builtOnce(settings, func(s infra.Settings) infra.TeaConfigForm {
		return infra.NewTeaConfigForm(workspace, projects, s)
	}))

	popup := func() (bool, error) {
		if s, err := settings(); err != nil || !s.Popup() || pane == "" {
			return false, nil
		}
		dir, err := os.Getwd()
		if err != nil {
			return false, err
		}
		// The popup's shell starts from tmux's environment: weave takes
		// its own along, but for what tells it which pane it runs in.
		env := slices.DeleteFunc(os.Environ(), func(v string) bool {
			return strings.HasPrefix(v, "TMUX=") || strings.HasPrefix(v, "TMUX_PANE=")
		})
		return true, tmux.Popup(os.Args[1:], dir, env)
	}

	weaver := application{
		popup:       popup,
		bindProject: app.NewBindProject(projects, projects, tmux, workspace, configForm),
		editProject: app.NewEditProject(projects, projects, configForm, infra.NewEditor().Preferring(func() string {
			if s, err := settings(); err == nil {
				return s.Editor()
			}
			return ""
		}), confirm),
		sessions:     tmux,
		findProject:  app.NewFindProject(projects, workspace),
		openProject:  app.NewOpenProject(projects, tmux, workspace, commands),
		killProject:  app.NewKillProject(tmux, tmux, projects, commands, infra.NewLineAcknowledger(os.Stdin, os.Stderr)),
		listProjects: app.NewListProjects(projects, tmux),
		settings:     settings,
	}
	// The browser asks its own questions, in its status line while on screen.
	browser := lazyBrowser(builtOnce(settings, func(s infra.Settings) infra.TeaBrowser { return infra.NewTeaBrowser(s, warnings) }))
	weaver.browseProjects = app.NewBrowseProjects(browser, projects, projects, workspace, tmux,
		weaver.listProjects, weaver.openProject, weaver.bindProject, weaver.editProject, weaver.killProject,
		app.NewRunTask(projects, tmux, browser), app.NewRerunCommand(projects, tmux), app.NewInterrupt(projects, tmux), app.NewReload(projects, tmux))
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
