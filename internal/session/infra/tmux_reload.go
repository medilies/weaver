package infra

import (
	"fmt"
	"slices"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// ReloadPane implements domain.SessionReloader: tmux respawns the pane,
// keeping its ID, size and place, with its start script, or else a plain
// shell, then types what opening types into it. Its tags of a task
// typed into it go, with the task.
func (t Tmux) ReloadPane(project domain.ProjectName, window domain.WindowPlan, index int) error {
	id, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	if id == t.opts.Pane {
		return domain.ErrPaneRunsWeaver
	}
	defaultCommand, err := t.option("="+project.String(), "default-command")
	if absent(err) {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return err
	}
	pane := window.Panes()[index]
	args := []string{"respawn-pane", "-k", "-t", id, "-c", formatArgument(pane.Dir())}
	if _, err := t.run(append(args, respawnCommand(pane, defaultCommand)...)...); err != nil {
		return err
	}
	for _, tag := range []string{taskTag, taskSinceTag} {
		if _, err := t.run("set-option", "-p", "-u", "-t", id, tag); err != nil {
			return err
		}
	}
	return t.startUp(id, pane)
}

// respawnCommand is what a respawned pane starts with: its start script,
// or a plain shell, which the session's panes start as when none is
// given, since respawn-pane would run the pane's first command again.
func respawnCommand(pane domain.PanePlan, defaultCommand string) []string {
	if script := shellCommand(pane, defaultCommand); script != nil {
		return script
	}
	if defaultCommand != "" {
		return []string{"--", "/bin/sh", "-c", shellLine + "-c " + shellQuote(defaultCommand)}
	}
	return []string{"--", "/bin/sh", "-c", shellLine + "-l"}
}

// ReloadWindow implements domain.SessionReloader. The new window is built
// at the session's first free index, then swapped with the old one, so no
// other window moves, and the old one killed. Throwaway windows of the
// old panes' tasks are tied to the new panes in their places.
func (t Tmux) ReloadWindow(project domain.ProjectName, window domain.WindowPlan) error {
	session := "=" + project.String()
	panes, err := t.projectPanes(project)
	if err != nil {
		return err
	}
	old, listed, err := t.windowTagged(project, window.Name())
	if err != nil {
		return err
	}
	if slices.Contains(listed, t.opts.Pane) {
		return domain.ErrPaneRunsWeaver
	}
	active, err := t.run("display-message", "-p", "-t", old, "#{window_active}")
	if err != nil {
		return err
	}
	defaultCommand, err := t.option(session, "default-command")
	if err != nil {
		return err
	}
	oldPanes := make(map[string]int) // index in window, by old pane ID
	for index := range window.Panes() {
		if pane, ok := pick(panes[window.Name().String()], window, index); ok {
			oldPanes[pane.id] = index
		}
	}
	created, firstPane, err := t.newWindow([]string{"-t", session + ":"}, window, defaultCommand)
	if err != nil {
		return err
	}
	if err := t.fill(created, firstPane, window, defaultCommand); err != nil {
		_, _ = t.run("kill-window", "-t", created)
		return err
	}
	if _, err := t.run("swap-window", "-d", "-s", created, "-t", old); err != nil {
		_, _ = t.run("kill-window", "-t", created)
		return err
	}
	if _, err := t.run("kill-window", "-t", old); err != nil {
		return err
	}
	if active == "1" {
		if _, err := t.run("select-window", "-t", created); err != nil {
			return err
		}
	}
	return t.retie(project, window, oldPanes)
}

// windowTagged is the ID of the first window of the project's session
// tagged with name, and its panes' IDs; an error wrapping
// ErrWindowNotRunning when there is none.
func (t Tmux) windowTagged(project domain.ProjectName, name domain.WindowName) (string, []string, error) {
	out, err := t.run("list-panes", "-s", "-t", "="+project.String(), "-F", "#{window_id} #{pane_id} "+sized("@weaver_window"))
	if absent(err) {
		return "", nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return "", nil, err
	}
	var window string
	var panes []string
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), " ", 3)
		if len(fields) != 3 || verbatim(fields[2]) != name.String() || window != "" && fields[0] != window {
			continue
		}
		window = fields[0]
		panes = append(panes, fields[1])
	}
	if window == "" {
		return "", nil, fmt.Errorf("%w: %s", domain.ErrWindowNotRunning, name)
	}
	return window, panes, nil
}

// retie points the throwaway windows of the tasks of oldPanes, the panes
// window had by their IDs, at the panes now in their places.
func (t Tmux) retie(project domain.ProjectName, window domain.WindowPlan, oldPanes map[string]int) error {
	if len(oldPanes) == 0 {
		return nil
	}
	out, err := t.run("list-windows", "-t", "="+project.String(), "-F", "#{window_id} #{"+taskOfTag+"}")
	if err != nil {
		return err
	}
	panes, err := t.projectPanes(project)
	if err != nil {
		return err
	}
	for line := range strings.Lines(out) {
		id, source, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		index, ok := oldPanes[source]
		if !ok {
			continue
		}
		pane, ok := pick(panes[window.Name().String()], window, index)
		if !ok {
			continue
		}
		if _, err := t.run("set-option", "-w", "-t", id, taskOfTag, pane.id); err != nil {
			return err
		}
	}
	return nil
}
