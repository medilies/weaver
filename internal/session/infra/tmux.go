package infra

import (
	"bytes"
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
	var session, previous, defaultCommand string
	var focused string
	for i, window := range plan.Windows() {
		var windowID, paneID string
		var err error
		if i == 0 {
			session, windowID, paneID, defaultCommand, err = t.newSession(plan.Name(), window)
			if err == nil {
				_, err = t.run("set-option", "-t", session, "@weaver_project", plan.Name().String())
			}
		} else {
			windowID, paneID, err = t.newWindow([]string{"-a", "-t", previous}, window, defaultCommand)
		}
		if err == nil {
			err = t.fill(windowID, paneID, window, defaultCommand)
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
	defaultCommand, err := t.option("="+name.String(), "default-command")
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
		windowID, paneID, err := t.newWindow(target, window, defaultCommand)
		if err == nil {
			if err = t.fill(windowID, paneID, window, defaultCommand); err != nil {
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
// checks it first. The active window and panes are focused, so freezing
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
		if active == "1" {
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
		pid, err := t.run("display-message", "-p", "-t", target, "#{pane_pid}")
		if err != nil {
			return nil, err
		}
		start, err := t.run("display-message", "-p", "-t", target, sized("pane_start_command"))
		if err != nil {
			return nil, err
		}
		active, err := t.run("display-message", "-p", "-t", target, "#{pane_active}")
		if err != nil {
			return nil, err
		}
		pane := domain.NewPane()
		if active == "1" {
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
func (t Tmux) Kill(name domain.ProjectName) error {
	_, err := t.run("kill-session", "-t", "="+name.String())
	if absent(err) {
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

// taskTag is the pane option holding the command Type typed into the
// pane, or RunApart into a throwaway window's, and taskOfTag the window
// option holding the ID of the pane a throwaway window's task is of.
const (
	taskTag   = "@weaver_task"
	taskOfTag = "@weaver_task_of"
)

// throwawayTag is the window option marking a throwaway window, which
// Capture skips. Untagged as a project window, restoration ignores it.
const throwawayTag = "@weaver_throwaway"

// Busy implements domain.TaskRunner: a pane is busy when it is in a mode,
// such as copy mode, which would take typed keys as its commands, or when
// a job other than its shell is in its terminal's foreground, or that
// can't be told.
func (t Tmux) Busy(project domain.ProjectName, window domain.WindowPlan, index int) (bool, error) {
	id, pid, err := t.paneOf(project, window, index)
	if err != nil {
		return false, err
	}
	inMode, err := t.run("display-message", "-p", "-t", id, "#{pane_in_mode}")
	if err != nil {
		return false, err
	}
	if inMode != "0" {
		return true, nil
	}
	group, err := exec.Command("ps", "-o", "tpgid=", "-p", pid).Output()
	if err != nil {
		return true, nil // the shell may be gone: ask rather than type
	}
	return strings.TrimSpace(string(group)) != pid, nil
}

// Type implements domain.TaskRunner. Whatever is at the prompt, as a
// prefill not run yet, is cut first, with C-e C-u, which shells keep to
// paste back with C-y, so the task's command isn't added to it. The pane
// and its window are selected first, so a failure types nothing.
func (t Tmux) Type(project domain.ProjectName, window domain.WindowPlan, index int, cmd domain.Command) error {
	id, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	if _, err := t.run("select-window", "-t", id); err != nil {
		return err
	}
	if _, err := t.run("select-pane", "-t", id); err != nil {
		return err
	}
	if _, err := t.run("send-keys", "-t", id, "C-e", "C-u"); err != nil {
		return err
	}
	if _, err := t.run("set-option", "-p", "-t", id, taskTag, argument(cmd.String())); err != nil {
		return err
	}
	return t.typeCommand(id, cmd.String(), true)
}

// RunApart implements domain.TaskRunner. The window goes after the
// session's last, and is selected.
func (t Tmux) RunApart(project domain.ProjectName, window domain.WindowPlan, index int, task domain.Task) error {
	source, _, err := t.paneOf(project, window, index)
	if err != nil {
		return err
	}
	pane := window.Panes()[index]
	session := "=" + project.String()
	defaultCommand, err := t.option(session, "default-command")
	if absent(err) {
		return fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return err
	}
	args := []string{
		"new-window", "-t", session + ":",
		"-n", formatArgument(task.ThrowawayWindow().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	}
	ids, err := t.runIDs(2, append(args, startScript(pane.Env(), nil, defaultCommand)...)...)
	if err != nil {
		return err
	}
	if _, err := t.run("set-option", "-w", "-t", ids[0], throwawayTag, "1"); err != nil {
		_, _ = t.run("kill-window", "-t", ids[0]) // untagged, freeze would capture it
		return err
	}
	if _, err := t.run("set-option", "-w", "-t", ids[0], taskOfTag, source); err != nil {
		return err
	}
	if _, err := t.run("set-option", "-p", "-t", ids[1], taskTag, argument(task.Command().String())); err != nil {
		return err
	}
	for _, cmd := range append(pane.Before(), task.Command()) {
		if err := t.typeCommand(ids[1], cmd.String(), true); err != nil {
			return err
		}
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
	shells, err := t.idleShells(project, slices.Collect(maps.Keys(found)))
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
// apart.
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
		"#{pane_id} #{pane_pid} #{?"+taskOfTag+",#{"+taskOfTag+"},-} "+sized(taskTag))
	if absent(err) {
		return nil, fmt.Errorf("%w: %s", domain.ErrSessionNotRunning, project)
	}
	if err != nil {
		return nil, err
	}
	type typed struct {
		at      at
		command string
	}
	running := map[string]typed{} // by process
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), " ", 4)
		if len(fields) != 4 {
			continue
		}
		source := fields[0]
		if fields[2] != "-" {
			source = fields[2]
		}
		pane, ok := panes[source]
		if command := verbatim(fields[3]); ok && command != "" {
			running[fields[1]] = typed{at: pane, command: command}
		}
	}
	idle, err := t.idleShells(project, slices.Collect(maps.Keys(running)))
	if err != nil {
		return nil, err
	}
	var commands []domain.RunningCommand
	for pid, run := range running {
		if !idle[pid] {
			commands = append(commands, domain.NewRunningCommand(run.at.window, run.at.index, run.command))
		}
	}
	return commands, nil
}

// idleShells are those of pids, the processes of panes of the project's
// session, whose shell idles at its prompt, checked with one ps: its
// terminal's foreground process group is its own, and it is still the
// shell, not a command it was replaced with by exec.
func (t Tmux) idleShells(project domain.ProjectName, pids []string) (map[string]bool, error) {
	idle := map[string]bool{}
	if len(pids) == 0 {
		return idle, nil
	}
	shells, err := t.shellNames(project)
	if err != nil {
		return nil, err
	}
	// One -o per column: POSIX ps takes all after "=" as the header.
	out, err := exec.Command("ps", "-o", "pid=", "-o", "tpgid=", "-o", "comm=", "-p", strings.Join(pids, ",")).Output()
	if err != nil && len(out) == 0 {
		return idle, nil // ps fails when every process is gone
	}
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == fields[1] && shells[programName(strings.Join(fields[2:], " "))] {
			idle[fields[0]] = true
		}
	}
	return idle, nil
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
func (t Tmux) newSession(name domain.ProjectName, window domain.WindowPlan) (session, windowID, paneID, defaultCommand string, err error) {
	pane := window.Panes()[0]
	args := []string{
		"new-session", "-d", "-s", name.String(),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{session_id} #{window_id} #{pane_id} " + sized("default-command"),
	}
	if t.opts.Size != nil {
		if cols, rows, ok := t.opts.Size(); ok {
			args = append(args, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
		}
	}
	scripted := shellCommand(pane, "") != nil
	if scripted {
		args = append(args, "--", "cat") // waits for input it never gets
	}
	out, err := t.run(args...)
	if err != nil {
		return "", "", "", "", err
	}
	fields := strings.SplitN(out, " ", 4)
	if len(fields) != 4 {
		return "", "", "", "", fmt.Errorf("tmux new-session: unexpected output %q", out)
	}
	session, windowID, paneID, defaultCommand = fields[0], fields[1], fields[2], verbatim(fields[3])
	if scripted {
		_, err = t.run(append([]string{"respawn-pane", "-k", "-t", paneID, "-c", formatArgument(pane.Dir())},
			shellCommand(pane, defaultCommand)...)...)
	}
	return session, windowID, paneID, defaultCommand, err
}

// newWindow creates a window placed by target: "-a" (after) or "-b"
// (before), "-t", and a window.
func (t Tmux) newWindow(target []string, window domain.WindowPlan, defaultCommand string) (windowID, paneID string, err error) {
	pane := window.Panes()[0]
	args := append(append([]string{"new-window", "-d"}, target...),
		"-n", formatArgument(window.Name().String()),
		"-c", formatArgument(pane.Dir()),
		"-P", "-F", "#{window_id} #{pane_id}",
	)
	ids, err := t.runIDs(2, append(args, shellCommand(pane, defaultCommand)...)...)
	if err != nil {
		return "", "", err
	}
	return ids[0], ids[1], nil
}

// fill tags the window as the project's, splits its first pane into the
// others, labels them in their borders, types their commands: those to
// run first, clear and the note when the pane clears, then the startup
// one, and selects its focused pane, without switching to the window.
func (t Tmux) fill(windowID, firstPaneID string, window domain.WindowPlan, defaultCommand string) error {
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
		id, err := t.run(append(args, shellCommand(pane, defaultCommand)...)...)
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
			if err := t.typeCommand(ids[i], cmd.String(), true); err != nil {
				return err
			}
		}
		if pane.Clears() {
			if err := t.typeCommand(ids[i], clearLine(pane), true); err != nil {
				return err
			}
		}
		if cmd, ok := pane.Startup().Command(); ok {
			if err := t.typeCommand(ids[i], cmd.String(), pane.Startup().Submits()); err != nil {
				return err
			}
		}
	}
	if focused := window.FocusedPane(); focused > 0 {
		_, err := t.run("select-pane", "-t", ids[focused])
		return err
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
	cmd := exec.Command("tmux", append(slices.Clone(t.opts.Global), args...)...)
	cmd.Env = t.opts.Env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
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
