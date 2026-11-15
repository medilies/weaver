package infra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	// Size reports the terminal's size in cells. Outside tmux, a new
	// session's window starts that big, less tmux's status lines, so its
	// splits come out as they will be seen; inside, it takes the client's
	// size instead, the pane weaver runs in being smaller. Without either,
	// tmux's default size applies.
	Size func() (cols, rows int, ok bool)
	// Limit is how long one tmux command may take before weaver gives up
	// on it, so a server that stops answering can't freeze weaver, read
	// as each command runs; nil, or zero, means callLimit.
	Limit func() time.Duration
	// Logs is the folder the panes that clear log what their before
	// commands print in, a folder per project; "" logs nothing.
	Logs string
}

// callLimit is how long a tmux command may take unless TmuxOptions says
// otherwise: enough for the slow calls that are fine, such as capturing a
// busy pane or starting a large project.
const callLimit = 5 * time.Second

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
	if absent(err) {
		return domain.RunningSession{}, false, nil
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

// Start implements domain.SessionLauncher. The session lands in its
// focused window.
func (t Tmux) Start(plan domain.SessionPlan) error {
	var session, previous string
	var focused string
	var l launch
	for i, window := range plan.Windows() {
		var windowID, paneID string
		var err error
		if i == 0 {
			session, windowID, paneID, l, err = t.newSession(plan.Name(), window)
			if err == nil {
				_, err = t.run("set-option", "-t", session, "@weaver_project", plan.Name().String())
			}
		} else {
			windowID, paneID, err = t.newWindow([]string{"-a", "-t", previous}, window, l)
		}
		if err == nil {
			err = t.fill(windowID, paneID, window, l)
		}
		if err != nil {
			if session != "" {
				_, _ = t.run("kill-session", "-t", session)
			}
			return err
		}
		previous = windowID
		if i == plan.FocusedWindow() {
			focused = windowID
		}
	}
	if plan.FocusedWindow() > 0 {
		if _, err := t.run("select-window", "-t", focused); err != nil {
			_, _ = t.run("kill-session", "-t", session)
			return err
		}
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
	l, err := t.launchOf(name)
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
		windowID, paneID, err := t.newWindow(target, window, l)
		if err == nil {
			if err = t.fill(windowID, paneID, window, l); err != nil {
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
// checks it first. The active window and panes are focused, so binding
// keeps where you were. A pane started with a command, rather than a shell,
// gets that command, as tmux holds it, in the directory it started in. The pane weaver runs in gets no
// command: its job is weaver. A pane with a job keeps its shell's
// directory, which the typed command will run from, rather than the
// job's, when /proc tells them apart.
func (t Tmux) Capture(session string) ([]domain.Window, error) {
	out, err := t.run("list-windows", "-t", "="+session, "-F",
		"#{?"+throwawayTag+",1,0} #{window_active} #{window_layout} "+sized("window_name"))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, session)
	}
	if err != nil {
		return nil, err
	}
	// Commands that start a shell like any other: tmux's default-command,
	// which a pane given no command holds as its start command once set,
	// and its default-shell, as a pane started with it explicitly holds.
	shells := make(map[string]bool)
	for _, option := range []string{"default-command", "default-shell"} {
		value, err := t.option("="+session, option)
		if err != nil {
			return nil, err
		}
		shells[value] = true
	}
	taken := make(map[string]bool)
	var windows []domain.Window
	for line := range strings.Lines(out) {
		throwaway, rest, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		if throwaway == "1" {
			continue
		}
		active, rest, _ := strings.Cut(rest, " ")
		layout, rawName, _ := strings.Cut(rest, " ")
		rawName = verbatim(rawName)
		cell, err := parseLayout(layout)
		if err != nil {
			return nil, err
		}
		panes, err := t.capturePanes(cell.layoutCell(cell.width).CreationOrder(), shells)
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
		if active == "1" && len(windows) > 0 { // the first is focused unless another is
			window = window.WithFocus()
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func (t Tmux) capturePanes(order []domain.CarvedPane, shells map[string]bool) ([]domain.Pane, error) {
	taken := make(map[string]bool)
	panes := make([]domain.Pane, 0, len(order))
	for i, p := range order {
		target := "%" + strconv.Itoa(p.Pane())
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
		activePid, err := t.run("display-message", "-p", "-t", target, "#{pane_active} #{pane_pid}")
		if err != nil {
			return nil, err
		}
		active, pid, _ := strings.Cut(activePid, " ")
		start, err := t.run("display-message", "-p", "-t", target, sized("pane_start_command"))
		if err != nil {
			return nil, err
		}
		pane := domain.NewPane()
		if active == "1" && i > 0 { // the first is focused unless another is
			pane = pane.WithFocus()
		}
		if target != t.opts.Pane {
			if cmd, ok := startedWith(startArgs(verbatim(start)), shells); ok {
				pane = pane.Prefilling(cmd)
				// It may have changed directory since, as `make -C sub`
				// does: it is typed where it started.
				started, err := t.run("display-message", "-p", "-t", target, sized("pane_start_path"))
				if err != nil {
					return nil, err
				}
				if started = verbatim(started); started != "" {
					path = started
				}
			}
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
		if split, ok := p.Split(); ok {
			pane = pane.Splitting(split)
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

// foreground is the command line of the job in the foreground of the
// terminal of the pane whose process is pid: none when that process is,
// either a shell idle at the prompt or the command the pane was started
// with. It is read best effort, as the job may end meanwhile.
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

// startedWith is the command a pane was started with, given its
// arguments, as it would be typed at a prompt: none when the pane started
// a shell: none at all, as tmux does given no command, one of shells, or
// weaver's startup script.
func startedWith(args []string, shells map[string]bool) (domain.Command, bool) {
	var line string
	switch {
	case len(args) == 0, wrapped(args):
		return domain.Command{}, false
	case len(args) == 1:
		line = args[0] // tmux has its default-shell run it, as a command line
	default:
		line = shellWords(args)
	}
	if shells[line] {
		return domain.Command{}, false
	}
	cmd, err := domain.NewCommand(line)
	return cmd, err == nil
}

// option is the value of a tmux option as target sees it.
func (t Tmux) option(target, name string) (string, error) {
	out, err := t.run("display-message", "-p", "-t", target, sized(name))
	return verbatim(out), err
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
	if absent(err) {
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
// Its zoomed windows are unzoomed first: tmux 3.4 crashes, ending every
// session, when it kills a session with a zoomed window.
func (t Tmux) Kill(name domain.ProjectName) error {
	session := "=" + name.String()
	out, err := t.run("list-windows", "-t", session, "-F", "#{window_zoomed_flag} #{window_id}")
	if absent(err) {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	if err != nil {
		return err
	}
	for line := range strings.Lines(out) {
		if id, zoomed := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "1 "); zoomed {
			// Only if it is still zoomed: resize-pane -Z zooms an unzoomed
			// window. A window closed meanwhile needs nothing.
			_, err := t.run("if-shell", "-F", "-t", id, "#{window_zoomed_flag}", "resize-pane -Z -t "+id)
			if err != nil && !lost(err) && !absent(err) {
				return err
			}
		}
	}
	_, err = t.run("kill-session", "-t", session)
	if absent(err) {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	return err
}

// InterruptPanes implements domain.SessionKiller. A pane in a mode, such
// as copy mode, leaves it first, for ctrl+c to reach what runs there.
func (t Tmux) InterruptPanes(name domain.ProjectName, within time.Duration) ([]string, error) {
	session := "=" + name.String()
	out, err := t.run("list-windows", "-t", session, "-F", "#{window_id} "+sized("window_name"))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	if err != nil {
		return nil, err
	}
	windows := map[string]string{} // names by ID
	for line := range strings.Lines(out) {
		id, printed, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		windows[id] = verbatim(printed)
	}
	out, err = t.run("list-panes", "-s", "-t", session, "-F", "#{window_id} #{pane_id} #{pane_pid} #{pane_in_mode} "+sized("@label"))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, name)
	}
	if err != nil {
		return nil, err
	}
	type pane struct{ id, pid, shown string }
	var panes []pane
	var pids []string
	modes := map[string]bool{}
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), " ", 5)
		if len(fields) < 5 || fields[1] == t.opts.Pane {
			continue
		}
		shown := windows[fields[0]]
		if label := verbatim(fields[4]); label != "" {
			shown += " › " + label
		}
		panes = append(panes, pane{id: fields[1], pid: fields[2], shown: shown})
		pids = append(pids, fields[2])
		modes[fields[1]] = fields[3] != "0"
	}
	idle, seen, err := t.idleShells(name, pids)
	if err != nil {
		return nil, err
	}
	var busy []pane
	for _, p := range panes {
		if idle[p.pid] || !seen[p.pid] {
			continue
		}
		if modes[p.id] {
			if _, err := t.run("send-keys", "-X", "-t", p.id, "cancel"); err != nil && !lost(err) && !absent(err) {
				return nil, err
			}
		}
		if _, err := t.run("send-keys", "-t", p.id, "C-c"); err != nil && !lost(err) && !absent(err) {
			return nil, err
		}
		busy = append(busy, p)
	}
	deadline := time.Now().Add(within)
	for len(busy) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		pids = pids[:0]
		for _, p := range busy {
			pids = append(pids, p.pid)
		}
		idle, seen, err := t.idleShells(name, pids)
		if errors.Is(err, domain.ErrSessionNotRunning) {
			return nil, nil // it ended meanwhile: nothing still runs
		}
		if err != nil {
			return nil, err
		}
		busy = slices.DeleteFunc(busy, func(p pane) bool { return idle[p.pid] || !seen[p.pid] })
	}
	var lingering []string
	for _, p := range busy {
		lingering = append(lingering, p.shown)
	}
	return lingering, nil
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

// Popup runs weave with args again in a tmux popup the size of the
// window, in dir, with env, and waits for it to end. The popup closes
// when weave succeeds, and stays open on a failure, showing why. Inside the popup,
// weave has no pane of its own, so it doesn't open another one. It
// waits as long as the popup is open: tmux's limit doesn't apply.
func (t Tmux) Popup(args []string, dir string, env []string) error {
	weave, err := os.Executable()
	if err != nil {
		return err
	}
	argv := append(slices.Clone(t.opts.Global), "display-popup", "-EE", "-w", "100%", "-h", "100%", "-d", dir)
	for _, v := range env {
		argv = append(argv, "-e", v)
	}
	argv = append(append(argv, "--", weave), args...)
	cmd := exec.Command("tmux", argv...)
	cmd.Env = t.opts.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// taskTag is the pane option holding the command Type typed into the
// pane, or RunApart into a throwaway window's, taskSinceTag the one
// holding when, in Unix nanoseconds, and taskOfTag the window option
// holding the ID of the pane a throwaway window's task is of.
const (
	taskTag      = "@weaver_task"
	taskSinceTag = "@weaver_task_since"
	taskOfTag    = "@weaver_task_of"
)

// throwawayTag is the window option marking a throwaway window, which
// Capture skips. Untagged as a project window, restoration ignores it.
const throwawayTag = "@weaver_throwaway"

// Busy implements domain.TaskRunner: a pane is busy when it is in a mode,
// such as copy mode, which would take typed keys as its commands, or when
// a job other than its shell is in its terminal's foreground, a command
// its shell was replaced with by exec included, or that ps can't tell.
// An error is the session's options failing to read.
func (t Tmux) Busy(project domain.ProjectName, window domain.WindowPlan, index int) (bool, error) {
	id, pid, err := t.paneOf(project, window, index)
	if err != nil {
		return false, err
	}
	if id == t.opts.Pane {
		return true, domain.ErrPaneRunsWeaver
	}
	inMode, err := t.run("display-message", "-p", "-t", id, "#{pane_in_mode}")
	if err != nil {
		return false, err
	}
	if inMode != "0" {
		return true, nil
	}
	idle, _, err := t.idleShells(project, []string{pid})
	if err != nil {
		return false, err
	}
	return !idle[pid], nil // a shell ps can't see may be gone: ask rather than type
}

// Type implements domain.TaskRunner. Whatever is at the prompt, as a
// prefill not run yet, is cut first, with C-e C-u, which shells keep to
// paste back with C-y, so the task's command isn't added to it. Landing,
// the pane and its window are selected first, so a failure types nothing.
func (t Tmux) Type(project domain.ProjectName, window domain.WindowPlan, index int, cmd domain.Command, landing bool) error {
	id, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	if landing {
		if _, err := t.run("select-window", "-t", id); err != nil {
			return err
		}
		if _, err := t.run("select-pane", "-t", id); err != nil {
			return err
		}
	}
	if _, err := t.run("send-keys", "-t", id, "C-e", "C-u"); err != nil {
		return err
	}
	if err := t.tagTask(id, cmd); err != nil {
		return err
	}
	return t.typeCommand(id, cmd.String(), true)
}

// RunApart implements domain.TaskRunner. The window takes the
// session's first free index, so it may sit between others, and is
// selected when landing.
func (t Tmux) RunApart(project domain.ProjectName, window domain.WindowPlan, index int, task domain.Task, landing bool) error {
	source, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	pane := window.Panes()[index]
	session := "=" + project.String()
	l, err := t.launchOf(project)
	if err != nil {
		return err
	}
	o := opening{env: pane.Env(), before: pane.Before(), clears: pane.Clears(), last: task.Command().String(), submit: true}
	log := l.logOf(task.ThrowawayWindow().String())
	args := []string{
		"new-window", "-t", session + ":",
		"-n", formatArgument(task.ThrowawayWindow().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	}
	if !landing {
		args = append(args, "-d")
	}
	ids, err := t.runIDs(2, append(args, l.script(o, log)...)...)
	if err != nil {
		return err
	}
	if _, err := t.run("set-option", "-w", "-t", ids[0], throwawayTag, "1"); err != nil {
		_, _ = t.run("kill-window", "-t", ids[0]) // untagged, bind would capture it
		return err
	}
	_, err = t.run("set-option", "-w", "-t", ids[0], taskOfTag, source)
	if err == nil {
		err = t.tagTask(ids[1], task.Command())
	}
	if err != nil {
		_, _ = t.run("kill-window", "-t", ids[0]) // nothing runs in it yet
		return err
	}
	return t.startUp(ids[1], l.typed(o, ids[1], log))
}

// tagTask tags the pane id with cmd, about to be typed into it, and with
// now.
func (t Tmux) tagTask(id string, cmd domain.Command) error {
	if _, err := t.run("set-option", "-p", "-t", id, taskTag, argument(cmd.String())); err != nil {
		return err
	}
	_, err := t.run("set-option", "-p", "-t", id, taskSinceTag, strconv.FormatInt(time.Now().UnixNano(), 10))
	return err
}

// Reach implements domain.TaskRunner: run's place is the ID of the pane
// it runs in.
func (t Tmux) Reach(project domain.ProjectName, run domain.RunningCommand) error {
	if err := t.ownedBy(project, run.Place()); err != nil {
		return err
	}
	if _, err := t.run("select-window", "-t", run.Place()); err != nil {
		return err
	}
	_, err := t.run("select-pane", "-t", run.Place())
	return err
}

// Interrupt implements domain.TaskRunner.
func (t Tmux) Interrupt(project domain.ProjectName, window domain.WindowPlan, index int) error {
	id, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	if id == t.opts.Pane {
		return domain.ErrPaneRunsWeaver
	}
	_, err = t.run("send-keys", "-t", id, "C-c")
	return err
}

// InterruptRun implements domain.TaskRunner: run's place is the ID of the
// pane it runs in.
func (t Tmux) InterruptRun(project domain.ProjectName, run domain.RunningCommand) error {
	if err := t.ownedBy(project, run.Place()); err != nil {
		return err
	}
	if run.Place() == t.opts.Pane {
		return domain.ErrPaneRunsWeaver
	}
	_, err := t.run("send-keys", "-t", run.Place(), "C-c")
	return err
}

// ownedBy returns an error wrapping ErrPaneNotRunning unless the pane id
// is in the project's session.
func (t Tmux) ownedBy(project domain.ProjectName, id string) error {
	session, err := t.run("display-message", "-p", "-t", id, "#{session_name}")
	if err != nil && !absent(err) {
		return err
	}
	if err != nil || session != project.String() {
		return fmt.Errorf("%w: it ended", domain.ErrPaneNotRunning)
	}
	return nil
}

// paneOf is the ID and process of the pane at index of window in the
// project's running session, found as pick finds it.
func (t Tmux) paneOf(project domain.ProjectName, window domain.WindowPlan, index int) (id, pid string, err error) {
	windows, err := t.projectPanes(project)
	if err != nil {
		return "", "", err
	}
	panes, ok := windows[window.Name().String()]
	if !ok {
		return "", "", fmt.Errorf("%w: its window is closed, open the project to restore it", domain.ErrPaneNotRunning)
	}
	pane, ok := pick(panes, window, index)
	if !ok && len(window.Panes()) == 1 {
		return "", "", fmt.Errorf("%w: no unlabelled pane in its window", domain.ErrPaneNotRunning)
	}
	if !ok {
		return "", "", fmt.Errorf("%w: no pane labelled %q in its window", domain.ErrPaneNotRunning, labelOf(window, index))
	}
	return pane.id, pane.pid, nil
}

// Idle implements domain.SessionInspector. Every pane is checked with one
// ps, as idleShells checks them.
func (t Tmux) Idle(project domain.ProjectName, windows []domain.WindowPlan) (map[domain.WindowName][]int, error) {
	listed, err := t.projectPanes(project)
	if err != nil {
		return nil, err
	}
	type at struct {
		window domain.WindowName
		index  int
	}
	found := map[string]at{} // by process
	for _, window := range windows {
		for index := range window.Panes() {
			if pane, ok := pick(listed[window.Name().String()], window, index); ok {
				found[pane.pid] = at{window: window.Name(), index: index}
			}
		}
	}
	shells, _, err := t.idleShells(project, slices.Collect(maps.Keys(found)))
	if err != nil {
		return nil, err
	}
	idle := map[domain.WindowName][]int{}
	for pid := range shells {
		pane := found[pid]
		idle[pane.window] = append(idle[pane.window], pane.index)
	}
	for window := range idle {
		slices.Sort(idle[window])
	}
	return idle, nil
}

// RunningCommands implements domain.SessionInspector. A command runs
// while the pane Type tagged with it, or the pane of the throwaway window
// RunApart tagged with it and with the pane it ran apart from, isn't idle
// as idleShells tells: it may be another job by then, which can't be told
// apart. A pane whose process ps doesn't report isn't counted: the mark
// is a hint, left out when it can't be told. Nor is the pane weaver runs
// in, which weaver keeps busy itself.
func (t Tmux) RunningCommands(project domain.ProjectName, windows []domain.WindowPlan) ([]domain.RunningCommand, error) {
	listed, err := t.projectPanes(project)
	if err != nil {
		return nil, err
	}
	type at struct {
		window domain.WindowName
		index  int
	}
	panes := map[string]at{} // by pane ID
	for _, window := range windows {
		for index := range window.Panes() {
			if pane, ok := pick(listed[window.Name().String()], window, index); ok {
				panes[pane.id] = at{window: window.Name(), index: index}
			}
		}
	}
	out, err := t.run("list-panes", "-s", "-t", "="+project.String(), "-F",
		"#{pane_id} #{pane_pid} #{?"+taskOfTag+",#{"+taskOfTag+"},-} #{?"+taskSinceTag+",#{"+taskSinceTag+"},0} "+sized(taskTag))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return nil, err
	}
	running := map[string]domain.RunningCommand{} // by process
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), " ", 5)
		if len(fields) != 5 || fields[0] == t.opts.Pane {
			continue
		}
		source, apart := fields[0], fields[2] != "-"
		if apart {
			source = fields[2]
		}
		pane, ok := panes[source]
		nanos, _ := strconv.ParseInt(fields[3], 10, 64) // 0, the oldest, when untold
		if command, err := domain.NewCommand(verbatim(fields[4])); ok && err == nil {
			running[fields[1]] = domain.NewRunningCommand(pane.window, pane.index, command).
				RunningIn(fields[0], apart, time.Unix(0, nanos))
		}
	}
	idle, seen, err := t.idleShells(project, slices.Collect(maps.Keys(running)))
	if err != nil {
		return nil, err
	}
	var commands []domain.RunningCommand
	for pid, run := range running {
		if seen[pid] && !idle[pid] {
			commands = append(commands, run)
		}
	}
	return commands, nil
}

// idleShells are those of pids, the processes of panes of the project's
// session, whose shell idles at its prompt, checked with one ps: its
// terminal's foreground process group is its own, and it is still the
// shell, not a command it was replaced with by exec. Seen are those ps
// reports at all.
func (t Tmux) idleShells(project domain.ProjectName, pids []string) (idle, seen map[string]bool, err error) {
	idle, seen = map[string]bool{}, map[string]bool{}
	if len(pids) == 0 {
		return idle, seen, nil
	}
	shells, err := t.shellNames(project)
	if err != nil {
		return nil, nil, err
	}
	// One -o per column: POSIX ps takes all after "=" as the header.
	out, err := exec.Command("ps", "-o", "pid=", "-o", "tpgid=", "-o", "comm=", "-p", strings.Join(pids, ",")).Output()
	if err != nil && len(out) == 0 {
		return idle, seen, nil // ps fails when every process is gone
	}
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			seen[fields[0]] = true
		}
		if len(fields) >= 3 && fields[0] == fields[1] && shells[programName(strings.Join(fields[2:], " "))] {
			idle[fields[0]] = true
		}
	}
	return idle, seen, nil
}

// shellNames are the names of the programs the panes of the project's
// session end their startup with: its default-shell, and the program of
// its default-command when set, past an exec and variable assignments.
// Linux's ps cuts a name at 15 characters, so a longer one never matches:
// its panes are never found idle.
func (t Tmux) shellNames(project domain.ProjectName) (map[string]bool, error) {
	names := map[string]bool{}
	for _, option := range []string{"default-shell", "default-command"} {
		value, err := t.option("="+project.String(), option)
		if absent(err) {
			return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
		}
		if err != nil {
			return nil, err
		}
		for _, word := range strings.Fields(value) {
			if word == "exec" || assigns(word) {
				continue
			}
			names[programName(word)] = true
			break
		}
	}
	return names, nil
}

// assigns reports whether word is a shell variable assignment, NAME=value.
func assigns(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, c := range name {
		letter := c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		digit := i > 0 && '0' <= c && c <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}

// programName is a program's name without its directory, or the "-" a
// login shell's name starts with, as ps may print it.
func programName(program string) string {
	return filepath.Base(strings.TrimPrefix(program, "-"))
}

// listedPane is a pane of a running session: its ID, its number in the
// server, its process and its label.
type listedPane struct {
	id, pid, label string
	number         int
}

// projectPanes are the panes of the project's running session, by the
// name each window is tagged with: the first window tagged with it, in
// order, which may have no pane listed if it closes meanwhile.
func (t Tmux) projectPanes(project domain.ProjectName) (map[string][]listedPane, error) {
	session := "=" + project.String()
	out, err := t.run("list-windows", "-t", session, "-F", "#{window_id} "+sized("@weaver_window"))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return nil, err
	}
	tags := map[string]string{} // by window ID
	windows := map[string][]listedPane{}
	for line := range strings.Lines(out) {
		id, tag, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		tag = verbatim(tag)
		if _, taken := windows[tag]; tag != "" && !taken {
			tags[id] = tag
			windows[tag] = nil
		}
	}
	out, err = t.run("list-panes", "-s", "-t", session, "-F", "#{window_id} #{pane_id} #{pane_pid} "+sized("@label"))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return nil, err
	}
	for line := range strings.Lines(out) {
		windowID, rest, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		paneID, rest, _ := strings.Cut(rest, " ")
		pid, label, _ := strings.Cut(rest, " ")
		tag, ok := tags[windowID]
		n, err := strconv.Atoi(strings.TrimPrefix(paneID, "%"))
		if !ok || err != nil {
			continue
		}
		windows[tag] = append(windows[tag], listedPane{id: paneID, pid: pid, label: verbatim(label), number: n})
	}
	return windows, nil
}

// Preview implements domain.SessionInspector. The panes are listed with
// their places through the first one found, which names their window, and
// each shows its visible screen, colours kept, its trailing blank lines
// dropped: a pane just cleared then shows its prompt at the bottom. The
// pane weaver runs in isn't captured. A zoomed window shows its zoomed
// pane alone, as tmux draws it.
func (t Tmux) Preview(project domain.ProjectName, window domain.WindowPlan) (domain.WindowPreview, error) {
	return readAgainIfLost(window.Name(), func() (domain.WindowPreview, error) { return t.preview(project, window) })
}

// readAgainIfLost is what read reads of the window named window, read
// again when a pane or window closed while it was read, and the window
// closed, ErrWindowNotRunning, when that happens twice.
func readAgainIfLost(window domain.WindowName, read func() (domain.WindowPreview, error)) (domain.WindowPreview, error) {
	preview, err := read()
	if lost(err) {
		preview, err = read()
	}
	if lost(err) {
		return domain.WindowPreview{}, fmt.Errorf("%w: %s", domain.ErrWindowNotRunning, window)
	}
	return preview, err
}

// preview is Preview, read once: an error lost reports is a pane or
// window closed meanwhile.
func (t Tmux) preview(project domain.ProjectName, window domain.WindowPlan) (domain.WindowPreview, error) {
	listed, err := t.projectPanes(project)
	if err != nil {
		return domain.WindowPreview{}, err
	}
	panes := listed[window.Name().String()]
	if len(panes) == 0 {
		return domain.WindowPreview{}, fmt.Errorf("%w: %s", domain.ErrWindowNotRunning, window.Name())
	}
	configured := map[string]int{} // by pane ID
	for index := range window.Panes() {
		if pane, ok := pick(panes, window, index); ok {
			configured[pane.id] = index
		}
	}
	gone := func(err error) error {
		if absent(err) {
			return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
		}
		return err
	}
	out, err := t.run("list-panes", "-t", panes[0].id, "-F", "#{window_width} #{window_height} #{window_zoomed_flag} "+
		"#{pane_active} #{pane_id} #{pane_left} #{pane_top} #{pane_width} #{pane_height} "+sized("@label"))
	if err != nil {
		return domain.WindowPreview{}, gone(err)
	}
	type paneRow struct {
		id, label string
		active    bool
		place     [4]int // left, top, width and height
	}
	var width, height int
	var zoomed bool
	var rows []paneRow
	for line := range strings.Lines(out) {
		// A line that doesn't parse, as the rest of a label holding a
		// newline set by hand, is skipped.
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), " ", 10)
		if len(fields) != 10 {
			continue
		}
		var numbers [8]int // the window's width, height and zoom, the pane's activity, left, top, width and height
		parsed := true
		for i, field := range slices.Concat(fields[:4], fields[5:9]) {
			if numbers[i], err = strconv.Atoi(field); err != nil {
				parsed = false
			}
		}
		if !parsed {
			continue
		}
		width, height, zoomed = numbers[0], numbers[1], numbers[2] == 1
		rows = append(rows, paneRow{id: fields[4], label: verbatim(fields[9]), active: numbers[3] == 1,
			place: [4]int{numbers[4], numbers[5], numbers[6], numbers[7]}})
	}
	active := func(row paneRow) bool { return row.active }
	if zoomed && slices.ContainsFunc(rows, active) { // the zoomed pane covers the others, still listed where they were
		rows = slices.DeleteFunc(rows, func(row paneRow) bool { return !row.active })
	}
	previews := make([]domain.PanePreview, 0, len(rows))
	for _, row := range rows {
		var lines []string
		if row.id != t.opts.Pane {
			shown, err := t.run("capture-pane", "-p", "-e", "-t", row.id)
			if err != nil {
				return domain.WindowPreview{}, gone(err)
			}
			lines = strings.Split(shown, "\n")
			for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
				lines = lines[:len(lines)-1]
			}
		}
		pane := domain.NewPanePreview(row.place[0], row.place[1], row.place[2], row.place[3], row.label, lines)
		if row.id == t.opts.Pane {
			pane = pane.RunningWeaver() // captured, it would hold its own capture, over and over
		}
		if index, ok := configured[row.id]; ok {
			pane = pane.Configured(index)
		}
		previews = append(previews, pane)
	}
	if len(previews) == 0 {
		return domain.WindowPreview{}, fmt.Errorf("%w: %s", domain.ErrWindowNotRunning, window.Name())
	}
	return domain.NewWindowPreview(width, height, previews...)
}

// lost reports whether err is tmux not finding a pane or window, closed
// since weaver found it.
func lost(err error) bool {
	failed, ok := errors.AsType[*tmuxError](err)
	if !ok {
		return false
	}
	if _, exited := errors.AsType[*exec.ExitError](failed.err); !exited {
		return false
	}
	return strings.HasPrefix(failed.stderr, "can't find pane") || strings.HasPrefix(failed.stderr, "can't find window")
}

// labelOf is the label the pane at index of window has in a running
// session: its name, or none for a window's only pane.
func labelOf(window domain.WindowPlan, index int) string {
	if len(window.Panes()) == 1 {
		return ""
	}
	return window.Panes()[index].Name().String()
}

// pick is the pane at index of window among panes, its window's in a
// running session: the one labelled with its name or, for a window's only
// pane, the first unlabelled one created, which panes split off it by
// hand leave in place.
func pick(panes []listedPane, window domain.WindowPlan, index int) (listedPane, bool) {
	name := labelOf(window, index)
	var picked listedPane
	found := false
	for _, pane := range panes {
		if pane.label == name && (!found || pane.number < picked.number) {
			picked, found = pane, true
		}
	}
	return picked, found
}

// newSession starts the session with window's first pane, and returns
// tmux's default-command too. tmux reads it from the user's config, which
// a server starting for this session only loads now, so a first pane
// needing a startup script, which ends by running that command, waits in
// a placeholder until it is read, then restarts with its script. Keys a
// hook of the user's config types into that pane, as on session-created,
// go to the placeholder and are lost.
func (t Tmux) newSession(name domain.ProjectName, window domain.WindowPlan) (session, windowID, paneID string, l launch, err error) {
	pane := window.Panes()[0]
	args := []string{
		"new-session", "-d", "-s", name.String(),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{session_id} #{window_id} #{pane_id}",
	}
	cols, rows, sized := t.terminalSize()
	guessed := t.statusLines("")
	if sized {
		args = append(args, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(max(rows-guessed, 1)))
	}
	// Whether the pane needs a script depends on the server's
	// default-command and default-shell, read once it runs.
	scripted := len(pane.Env()) > 0 || len(pane.Note()) > 0 || pane.Clears()
	if scripted {
		args = append(args, "--", "cat") // waits for input it never gets
	}
	ids, err := t.runIDs(3, args...)
	if err != nil {
		return "", "", "", launch{}, err
	}
	session, windowID, paneID = ids[0], ids[1], ids[2]
	if l, err = t.launchOf(name); err != nil {
		return "", "", "", launch{}, err
	}
	if actual := t.statusLines(session); sized && actual != guessed {
		// Started by new-session, the server only now read the user's
		// config, whose status lines the guess missed: the window takes
		// the right size, and goes back to following its clients.
		if _, err = t.run("resize-window", "-t", windowID, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(max(rows-actual, 1))); err == nil {
			_, err = t.run("set-option", "-w", "-u", "-t", windowID, "window-size")
		}
		if err != nil {
			return "", "", "", launch{}, err
		}
	}
	if scripted {
		_, err = t.run(append([]string{"respawn-pane", "-k", "-t", paneID, "-c", formatArgument(pane.Dir())},
			l.respawn(openingOf(pane), l.paneLog(window, 0))...)...)
	}
	return session, windowID, paneID, l, err
}

// launchOf is how the panes of the project's running session start.
func (t Tmux) launchOf(project domain.ProjectName) (launch, error) {
	session := "=" + project.String()
	command, err := t.option(session, "default-command")
	if err == nil {
		var shell string
		shell, err = t.option(session, "default-shell")
		l := launch{command: command, shell: shell, tmux: "tmux"}
		if path, lookErr := exec.LookPath("tmux"); lookErr == nil {
			l.tmux = path
		}
		if t.opts.Logs != "" {
			l.logs = filepath.Join(t.opts.Logs, project.String())
		}
		if err == nil {
			return l, nil
		}
	}
	if absent(err) {
		return launch{}, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	return launch{}, err
}

// terminalSize is the size of the terminal a new session is attached
// in: the client's, inside tmux, else the terminal's. Its window is that
// less the status lines, so that it keeps its size once attached: were it
// to change, tmux would share the difference out over the panes, a
// grid's columns or rows in cells too.
func (t Tmux) terminalSize() (cols, rows int, ok bool) {
	if t.opts.Inside {
		out, err := t.run("display-message", "-p", "#{client_width} #{client_height}")
		if _, scanErr := fmt.Sscan(out, &cols, &rows); err == nil && scanErr == nil && cols > 0 && rows > 0 {
			return cols, rows, true
		}
	}
	if t.opts.Size != nil {
		return t.opts.Size()
	}
	return 0, 0, false
}

// statusLines is how many lines tmux's status takes in session, or
// globally for none: read from a running server, else tmux's default of
// one line.
func (t Tmux) statusLines(session string) int {
	args := []string{"show-options", "-gv", "status"}
	if session != "" {
		args = []string{"display-message", "-p", "-t", session, "#{status}"}
	}
	out, err := t.run(args...)
	switch {
	case err != nil:
		return 1
	case out == "off":
		return 0
	case out == "on":
		return 1
	}
	if n, err := strconv.Atoi(out); err == nil {
		return n
	}
	return 1
}

// newWindow creates a window placed by target: "-a" (after) or "-b"
// (before), "-t", and a window.
func (t Tmux) newWindow(target []string, window domain.WindowPlan, l launch) (windowID, paneID string, err error) {
	pane := window.Panes()[0]
	args := append(append([]string{"new-window", "-d"}, target...),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	)
	ids, err := t.runIDs(2, append(args, l.script(openingOf(pane), l.paneLog(window, 0))...)...)
	if err != nil {
		return "", "", err
	}
	return ids[0], ids[1], nil
}

// fill tags the window as the project's, splits its first pane into the
// others, labels them in their borders, types their commands: those to
// run first, clear and the note when the pane clears, then the startup
// one, and selects its focused pane, without switching to the window.
func (t Tmux) fill(windowID, firstPaneID string, window domain.WindowPlan, l launch) error {
	if _, err := t.run("set-option", "-w", "-t", windowID, "@weaver_window", argument(window.Name().String())); err != nil {
		return err
	}
	if window.NeedsSize() {
		// A grid with a column or row in cells: its splits depend on the
		// window's size, which the session's terminal set.
		size, err := t.run("display-message", "-p", "-t", windowID, "#{window_width} #{window_height}")
		if err != nil {
			return err
		}
		var cols, rows int
		if _, err := fmt.Sscan(size, &cols, &rows); err != nil {
			return fmt.Errorf("tmux window size: unexpected output %q", size)
		}
		window = window.SizedTo(cols, rows)
	}
	panes := window.Panes()
	ids := []string{firstPaneID}
	for i, pane := range panes[1:] {
		split, _ := pane.Split()
		args := []string{
			"split-window", "-d", directionFlag(split.Direction()), "-t", ids[split.Of()],
			"-c", formatArgument(pane.Dir()),
			"-P", "-F", "#{pane_id}",
		}
		if !split.Size().IsDefault() {
			args = append(args, "-l", split.Size().String())
		}
		id, err := t.run(append(args, l.script(openingOf(pane), l.paneLog(window, i+1))...)...)
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
		if err := t.startUp(ids[i], l.typed(openingOf(pane), ids[i], l.paneLog(window, i))); err != nil {
			return err
		}
	}
	if focused := window.FocusedPane(); focused > 0 {
		_, err := t.run("select-pane", "-t", ids[focused])
		return err
	}
	return nil
}

// startUp types lines into the pane id, as typed says.
func (t Tmux) startUp(id string, lines []typedLine) error {
	for _, line := range lines {
		if err := t.typeCommand(id, line.text, line.submit); err != nil {
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
	// tmux reads #[…] and ## in a drawn border as styles and escapes;
	// q/h doubles each # of the label, so it shows as written.
	_, err := t.run("set-option", "-w", "-t", windowID, "pane-border-format", " #{q/h:@label} ")
	return err
}

// typeCommand types cmd at the pane's prompt, as if the user did, and
// presses Enter when submit: it stays in the shell history, to run again
// after Ctrl-C.
func (t Tmux) typeCommand(paneID, cmd string, submit bool) error {
	if _, err := t.run("send-keys", "-t", paneID, "-l", "--", argument(cmd)); err != nil {
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
	var limit time.Duration
	if t.opts.Limit != nil {
		limit = t.opts.Limit()
	}
	if limit == 0 {
		limit = callLimit
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", append(slices.Clone(t.opts.Global), args...)...)
	cmd.Env = t.opts.Env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("tmux %s: no answer within %s", args[0], limit)
	}
	if err != nil {
		return "", &tmuxError{command: args[0], err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// tmuxError is a tmux command that failed, with what tmux said.
type tmuxError struct {
	command string
	err     error
	stderr  string
}

func (e *tmuxError) Error() string {
	return "tmux " + e.command + ": " + e.err.Error() + ": " + e.stderr
}

func (e *tmuxError) Unwrap() error {
	return e.err
}

// absent reports whether err is tmux saying that the session it was given
// doesn't exist, or that no server runs to have one. Any other failure,
// such as a socket weave may not open, is an error: reading it as "not
// running" would hide the user's sessions.
func absent(err error) bool {
	failed, ok := errors.AsType[*tmuxError](err)
	if !ok {
		return false
	}
	if _, exited := errors.AsType[*exec.ExitError](failed.err); !exited {
		return false
	}
	said := failed.stderr
	return strings.HasPrefix(said, "can't find session") ||
		strings.HasPrefix(said, "no server running") || // a socket left by a server that's gone
		strings.HasPrefix(said, "error connecting to") && strings.HasSuffix(said, "(No such file or directory)")
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

// startArgs reads back the arguments a pane was started with from
// #{pane_start_command}, read verbatim: tmux joins them with spaces,
// quoting those that need it and escaping their characters as C does,
// "\\n" for a newline, "\\033" for an escape, and a "\\" before any
// character it could read otherwise.
func startArgs(start string) []string {
	var args []string
	var arg strings.Builder
	var quote byte
	started := false
	for i := 0; i < len(start); i++ {
		c := start[i]
		switch {
		case c == '\\' && i+1 < len(start):
			i++
			end := i
			for end < len(start) && end < i+3 && '0' <= start[end] && start[end] <= '7' {
				end++
			}
			if code, err := strconv.ParseUint(start[i:end], 8, 8); err == nil {
				arg.WriteByte(byte(code))
				i = end - 1
			} else {
				arg.WriteByte(unescaped(start[i]))
			}
			started = true
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote, started = c, true
		case quote == 0 && c == ' ':
			if started {
				args = append(args, arg.String())
				arg.Reset()
				started = false
			}
		default:
			arg.WriteByte(c)
			started = true
		}
	}
	if started {
		args = append(args, arg.String())
	}
	return args
}

// unescaped is the character that "\\" and c stand for in a C string.
func unescaped(c byte) byte {
	switch c {
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	}
	return c
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
