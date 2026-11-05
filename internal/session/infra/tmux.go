package infra

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/medilies/weaver/internal/session/domain"
)

// TmuxOptions configures how weaver talks to tmux.
type TmuxOptions struct {
	// Global flags go before every tmux command, e.g. -S <socket> to talk
	// to another server than the user's.
	Global []string
	// Env is the environment tmux runs with; nil inherits weaver's.
	Env []string
	// Inside reports that weaver runs inside tmux ($TMUX is set), where
	// attaching switches the current client to the session instead.
	Inside bool
	// Size reports the terminal's size in cells. A new session starts that
	// big, so its percentage splits come out as they will be seen; without
	// it, tmux's default size applies.
	Size func() (cols, rows int, ok bool)
}

// Tmux drives a tmux server through the tmux command. Sessions are named
// after projects and targeted with "=name", which matches exactly; panes
// and windows are targeted by the IDs tmux prints when creating them, so
// user options such as base-index never matter.
type Tmux struct {
	opts TmuxOptions
}

// NewTmux constructs a Tmux.
func NewTmux(opts TmuxOptions) Tmux {
	return Tmux{opts: opts}
}

// IsRunning implements domain.SessionLauncher.
func (t Tmux) IsRunning(name domain.ProjectName) (bool, error) {
	_, err := t.run("has-session", "-t", "="+name.String())
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return false, nil // no such session, or no server at all
	}
	return err == nil, err
}

// Start implements domain.SessionLauncher.
func (t Tmux) Start(plan domain.SessionPlan) error {
	var session, previous string
	for i, window := range plan.Windows() {
		var windowID, paneID string
		var err error
		if i == 0 {
			session, windowID, paneID, err = t.newSession(plan.Name(), window)
		} else {
			windowID, paneID, err = t.newWindow(previous, window)
		}
		if err == nil {
			err = t.fill(windowID, paneID, window)
		}
		if err != nil {
			if session != "" {
				_, _ = t.run("kill-session", "-t", session)
			}
			return err
		}
		previous = windowID
	}
	return nil
}

// Running implements domain.SessionLister.
func (t Tmux) Running() ([]string, error) {
	out, err := t.run("list-sessions", "-F", "#{session_name}")
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return nil, nil // no server, so no sessions
	}
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Kill implements domain.SessionKiller.
func (t Tmux) Kill(name domain.ProjectName) error {
	_, err := t.run("kill-session", "-t", "="+name.String())
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	return err
}

// Attach implements domain.SessionLauncher. Outside tmux it replaces
// weaver with `tmux attach-session`, like the shell scripts it replaces.
func (t Tmux) Attach(name domain.ProjectName) error {
	target := "=" + name.String()
	if t.opts.Inside {
		_, err := t.run("switch-client", "-t", target)
		return err
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	env := t.opts.Env
	if env == nil {
		env = os.Environ()
	}
	argv := append([]string{"tmux"}, t.opts.Global...)
	return syscall.Exec(path, append(argv, "attach-session", "-t", target), env)
}

func (t Tmux) newSession(name domain.ProjectName, window domain.WindowPlan) (session, windowID, paneID string, err error) {
	pane := window.Panes()[0]
	args := []string{
		"new-session", "-d", "-s", name.String(),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{session_id} #{window_id} #{pane_id}",
	}
	if t.opts.Size != nil {
		if cols, rows, ok := t.opts.Size(); ok {
			args = append(args, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
		}
	}
	ids, err := t.runIDs(3, append(args, shellCommand(pane)...)...)
	if err != nil {
		return "", "", "", err
	}
	return ids[0], ids[1], ids[2], nil
}

func (t Tmux) newWindow(after string, window domain.WindowPlan) (windowID, paneID string, err error) {
	pane := window.Panes()[0]
	args := []string{
		"new-window", "-d", "-a", "-t", after,
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	}
	ids, err := t.runIDs(2, append(args, shellCommand(pane)...)...)
	if err != nil {
		return "", "", err
	}
	return ids[0], ids[1], nil
}

// fill splits the window's first pane into the others, labels them in
// their borders, then types their commands.
func (t Tmux) fill(windowID, firstPaneID string, window domain.WindowPlan) error {
	panes := window.Panes()
	ids := []string{firstPaneID}
	for _, pane := range panes[1:] {
		split, _ := pane.Split()
		args := []string{
			"split-window", "-d", directionFlag(split.Direction()), "-t", ids[split.Of()],
			"-c", formatArgument(pane.Dir()),
			"-P", "-F", "#{pane_id}",
		}
		if !split.Size().IsDefault() {
			args = append(args, "-l", split.Size().String())
		}
		id, err := t.run(append(args, shellCommand(pane)...)...)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if len(panes) > 1 {
		if err := t.label(windowID, ids, panes); err != nil {
			return err
		}
	}
	for i, pane := range panes {
		if err := t.typeStartup(ids[i], pane.Startup()); err != nil {
			return err
		}
	}
	return nil
}

func (t Tmux) label(windowID string, ids []string, panes []domain.PanePlan) error {
	for i, pane := range panes {
		if _, err := t.run("set-option", "-p", "-t", ids[i], "@label", argument(pane.Name().String())); err != nil {
			return err
		}
	}
	if _, err := t.run("set-option", "-w", "-t", windowID, "pane-border-status", "top"); err != nil {
		return err
	}
	_, err := t.run("set-option", "-w", "-t", windowID, "pane-border-format", " #{@label} ")
	return err
}

// typeStartup types the pane's command at its prompt, as if the user did:
// it stays in the shell history, to run again after Ctrl-C.
func (t Tmux) typeStartup(paneID string, startup domain.Startup) error {
	cmd, ok := startup.Command()
	if !ok {
		return nil
	}
	if _, err := t.run("send-keys", "-t", paneID, "-l", "--", argument(cmd.String())); err != nil {
		return err
	}
	if !startup.Submits() {
		return nil
	}
	_, err := t.run("send-keys", "-t", paneID, "Enter")
	return err
}

func directionFlag(direction domain.Direction) string {
	if direction == domain.Below {
		return "-v"
	}
	return "-h"
}

// run runs one tmux command and returns its output without the final
// newline.
func (t Tmux) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", append(slices.Clone(t.opts.Global), args...)...)
	cmd.Env = t.opts.Env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// runIDs runs a tmux command printing n space-separated IDs. (tmux prints
// a tab as "_".)
func (t Tmux) runIDs(n int, args ...string) ([]string, error) {
	out, err := t.run(args...)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(out)
	if len(ids) != n {
		return nil, fmt.Errorf("tmux %s: unexpected output %q", args[0], out)
	}
	return ids, nil
}

// argument protects a trailing ";", which tmux reads as a command
// separator even in arguments passed without a shell: "\;" stands for a
// literal ";".
func argument(s string) string {
	if strings.HasSuffix(s, ";") {
		return s[:len(s)-1] + `\;`
	}
	return s
}

// formatArgument is argument for values tmux expands as formats, such as
// window names and start directories: "##" stands for a literal "#".
func formatArgument(s string) string {
	return argument(strings.ReplaceAll(s, "#", "##"))
}
