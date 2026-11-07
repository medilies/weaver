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
	// Inside reports whether weaver runs inside tmux ($TMUX): attaching
	// then switches the current client to the session instead.
	Inside bool
	// Pane is the pane weaver runs in ($TMUX_PANE): empty outside tmux,
	// and inside a popup or run-shell, which have no pane of their own.
	Pane string
	// Size reports the terminal's size in cells. A new session starts that
	// big, so its percentage splits come out as they will be seen; without
	// it, tmux's default size applies.
	Size func() (cols, rows int, ok bool)
}

// Tmux drives a tmux server through the tmux command. Sessions are named
// after projects and targeted with "=name", which matches exactly; panes
// and windows are targeted by the IDs tmux prints when creating them, so
// user options such as base-index never matter.
//
// weaver tags the sessions it starts with the @weaver_project option and
// their windows with @weaver_window, so it recognises them later, even
// once renamed.
type Tmux struct {
	opts TmuxOptions
}

// NewTmux constructs a Tmux.
func NewTmux(opts TmuxOptions) Tmux {
	return Tmux{opts: opts}
}

// Find implements domain.SessionFinder. A session is owned when it
// carries its project's tag.
func (t Tmux) Find(name domain.ProjectName) (domain.RunningSession, bool, error) {
	// A window's format sees its session's options too. "/" can't be in a
	// project name, so it ends the session's tag.
	out, err := t.run("list-windows", "-t", "="+name.String(), "-F", "#{@weaver_project}/"+sized("@weaver_window"))
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return domain.RunningSession{}, false, nil // no such session, or no server at all
	}
	if err != nil {
		return domain.RunningSession{}, false, err
	}
	owned := false
	var windows []domain.WindowName
	for line := range strings.Lines(out) {
		project, tag, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "/")
		owned = owned || project == name.String()
		if window, err := domain.NewWindowName(verbatim(tag)); err == nil {
			windows = append(windows, window)
		}
	}
	return domain.NewRunningSession(owned, windows...), true, nil
}

// Start implements domain.SessionLauncher.
func (t Tmux) Start(plan domain.SessionPlan) error {
	var session, previous string
	for i, window := range plan.Windows() {
		var windowID, paneID string
		var err error
		if i == 0 {
			session, windowID, paneID, err = t.newSession(plan.Name(), window)
			if err == nil {
				_, err = t.run("set-option", "-t", session, "@weaver_project", plan.Name().String())
			}
		} else {
			windowID, paneID, err = t.newWindow([]string{"-a", "-t", previous}, window)
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

// Restore implements domain.SessionLauncher. A window whose anchor is
// gone too goes at the end.
func (t Tmux) Restore(name domain.ProjectName, placements []domain.WindowPlacement) error {
	out, err := t.run("list-windows", "-t", "="+name.String(), "-F", "#{window_id} "+sized("@weaver_window"))
	if err != nil {
		return err
	}
	ids := make(map[string]string)
	var last string
	for line := range strings.Lines(out) {
		id, tag, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		if tag = verbatim(tag); tag != "" {
			ids[tag] = id
		}
		last = id
	}
	for _, placement := range placements {
		window := placement.Window()
		target, atEnd := []string{"-a", "-t", last}, true
		if after, ok := placement.After(); ok && ids[after.String()] != "" {
			anchor := ids[after.String()]
			target, atEnd = []string{"-a", "-t", anchor}, anchor == last
		} else if before, ok := placement.Before(); ok && ids[before.String()] != "" {
			target, atEnd = []string{"-b", "-t", ids[before.String()]}, false
		}
		windowID, paneID, err := t.newWindow(target, window)
		if err == nil {
			if err = t.fill(windowID, paneID, window); err != nil {
				// Tagged already, it would count as restored.
				_, _ = t.run("kill-window", "-t", windowID)
			}
		}
		if err != nil {
			return err
		}
		ids[window.Name().String()] = windowID
		if atEnd {
			last = windowID
		}
	}
	return nil
}

// Capture implements domain.SessionCapturer. Panes are named after their
// labels, else "pane N" in a window with several; a name already taken in
// its window or session gets a number. The job in the foreground of a
// pane's terminal, if any, is typed at its prompt but not run: ps rebuilds
// it from its arguments, losing quotes, pipes and variables, so the user
// checks it first. The pane weaver runs in gets no command: its job is
// weaver. A pane with a job keeps its shell's directory, which the typed
// command will run from, rather than the job's, when /proc tells them
// apart.
func (t Tmux) Capture(session string) ([]domain.Window, error) {
	out, err := t.run("list-windows", "-t", "="+session, "-F", "#{window_layout} "+sized("window_name"))
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, session)
	}
	if err != nil {
		return nil, err
	}
	taken := make(map[string]bool)
	var windows []domain.Window
	for line := range strings.Lines(out) {
		layout, rawName, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		rawName = verbatim(rawName)
		cell, err := parseLayout(layout)
		if err != nil {
			return nil, err
		}
		panes, err := t.capturePanes(creationOrder(cell))
		if err != nil {
			return nil, err
		}
		name, err := domain.NewWindowName(unique(strings.TrimSpace(rawName), taken))
		if err != nil {
			name, _ = domain.NewWindowName(unique("window "+strconv.Itoa(len(windows)+1), taken))
		}
		window, err := domain.NewWindow(name, panes...)
		if err != nil {
			return nil, fmt.Errorf("window %q: %w", name, err)
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func (t Tmux) capturePanes(order []layoutPane) ([]domain.Pane, error) {
	taken := make(map[string]bool)
	panes := make([]domain.Pane, 0, len(order))
	for i, p := range order {
		target := "%" + strconv.Itoa(p.pane)
		path, err := t.run("display-message", "-p", "-t", target, sized("pane_current_path"))
		if err != nil {
			return nil, err
		}
		path = verbatim(path)
		label, err := t.run("display-message", "-p", "-t", target, sized("@label"))
		if err != nil {
			return nil, err
		}
		label = verbatim(label)
		pid, err := t.run("display-message", "-p", "-t", target, "#{pane_pid}")
		if err != nil {
			return nil, err
		}
		pane := domain.NewPane()
		if target != t.opts.Pane {
			if cmd, ok := foreground(pid); ok {
				pane = pane.Prefilling(cmd)
				// tmux reports the job's directory, but the prefill runs
				// from the shell's. Linux marks a removed one, and tmux
				// drops the mark, so it is dropped here too.
				if cwd, err := os.Readlink("/proc/" + pid + "/cwd"); err == nil {
					path = strings.TrimSuffix(cwd, " (deleted)")
				}
			}
		}
		if dir, err := domain.NewPaneDir(path); err == nil {
			pane = pane.In(dir)
		}
		label = strings.TrimSpace(label)
		if _, err := domain.NewPaneName(label); err != nil && len(order) > 1 {
			label = domain.DefaultPaneName(i).String()
		}
		if name, err := domain.NewPaneName(unique(label, taken)); err == nil {
			pane = pane.Named(name)
		}
		if p.splits {
			size, err := domain.Percent(p.percent)
			if err != nil {
				return nil, err
			}
			split, err := domain.NewSplit(p.of, p.direction, size)
			if err != nil {
				return nil, err
			}
			pane = pane.Splitting(split)
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

// foreground is the command line of the job in the foreground of the
// terminal of the pane whose process is pid: none when that process, its
// shell, is idle at the prompt. It is read best effort, as the job may end
// meanwhile.
func foreground(pid string) (domain.Command, bool) {
	group, err := exec.Command("ps", "-o", "tpgid=", "-p", pid).Output()
	leader := strings.TrimSpace(string(group))
	if err != nil || leader == pid || strings.HasPrefix(leader, "-") {
		return domain.Command{}, false // -1: no terminal
	}
	// -ww: macOS's ps cuts the line at the terminal's width otherwise.
	args, err := exec.Command("ps", "-ww", "-o", "args=", "-p", leader).Output()
	if err != nil {
		return domain.Command{}, false
	}
	cmd, err := domain.NewCommand(string(args))
	return cmd, err == nil
}

// Current implements domain.SessionLocator. Without a pane of its own, as
// in a popup or run-shell, it is the session tmux finds current: the most
// recently active one, which the key that ran weaver just made so.
func (t Tmux) Current() (string, error) {
	args := []string{"display-message", "-p"}
	switch {
	case t.opts.Pane != "":
		args = append(args, "-t", t.opts.Pane)
	case !t.opts.Inside:
		return "", domain.ErrNotInsideTmux
	}
	out, err := t.run(append(args, sized("session_name"))...)
	return verbatim(out), err
}

// unique is name, or name with the first free number added, and marks
// the result taken. An empty name stays empty.
func unique(name string, taken map[string]bool) string {
	if name == "" {
		return ""
	}
	candidate := name
	for n := 2; taken[candidate]; n++ {
		candidate = name + " " + strconv.Itoa(n)
	}
	taken[candidate] = true
	return candidate
}

// Running implements domain.SessionLister.
func (t Tmux) Running() ([]string, error) {
	out, err := t.run("list-sessions", "-F", sized("session_name"))
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return nil, nil // no server, so no sessions
	}
	if err != nil || out == "" {
		return nil, err
	}
	var sessions []string
	for line := range strings.Lines(out) {
		sessions = append(sessions, verbatim(strings.TrimSuffix(line, "\n")))
	}
	return sessions, nil
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

// newWindow creates a window placed by target: "-a" (after) or "-b"
// (before), "-t", and a window.
func (t Tmux) newWindow(target []string, window domain.WindowPlan) (windowID, paneID string, err error) {
	pane := window.Panes()[0]
	args := append(append([]string{"new-window", "-d"}, target...),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	)
	ids, err := t.runIDs(2, append(args, shellCommand(pane)...)...)
	if err != nil {
		return "", "", err
	}
	return ids[0], ids[1], nil
}

// fill tags the window as the project's, splits its first pane into the
// others, labels them in their borders, then types their commands: those
// to run first, then the startup one.
func (t Tmux) fill(windowID, firstPaneID string, window domain.WindowPlan) error {
	if _, err := t.run("set-option", "-w", "-t", windowID, "@weaver_window", argument(window.Name().String())); err != nil {
		return err
	}
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
		for _, cmd := range pane.Before() {
			if err := t.typeCommand(ids[i], cmd, true); err != nil {
				return err
			}
		}
		if cmd, ok := pane.Startup().Command(); ok {
			if err := t.typeCommand(ids[i], cmd, pane.Startup().Submits()); err != nil {
				return err
			}
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
	// tmux reads #[…] and ## in a drawn border as styles and escapes;
	// q/h doubles each # of the label, so it shows as written.
	_, err := t.run("set-option", "-w", "-t", windowID, "pane-border-format", " #{q/h:@label} ")
	return err
}

// typeCommand types cmd at the pane's prompt, as if the user did, and
// presses Enter when submit: it stays in the shell history, to run again
// after Ctrl-C.
func (t Tmux) typeCommand(paneID string, cmd domain.Command, submit bool) error {
	if _, err := t.run("send-keys", "-t", paneID, "-l", "--", argument(cmd.String())); err != nil {
		return err
	}
	if !submit {
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

// sized is a format printing variable's size in bytes, a space and its
// value, for verbatim to read once whatever the format prints before it
// is cut off.
func sized(variable string) string {
	return "#{n:" + variable + "} #{" + variable + "}"
}

// verbatim reads a value printed by sized as tmux holds it. tmux 3.4
// prints a "\" before each "$" that a letter, "_" or "{" follows, though
// it doesn't hold one, so a value printed longer than its size has them
// taken out. One printed at its size, as by a tmux that doesn't add them,
// is read as is. A client without UTF-8 prints each multibyte character
// as one "_", which can hide the added "\"; such a value is lost anyway.
func verbatim(printed string) string {
	size, value, _ := strings.Cut(printed, " ")
	if n, err := strconv.Atoi(size); err != nil || len(value) <= n {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+2 < len(value) && value[i+1] == '$' && startsName(value[i+2]) {
			continue
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

// startsName reports whether c, after a "$", makes tmux 3.4 print a "\"
// before the "$".
func startsName(c byte) bool {
	return c == '_' || c == '{' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
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
