//go:build terminal

// Package terminal tests the weave binary as a user runs it: in a real
// terminal emulator, attached to a private tmux server.
package terminal

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	tuitest "github.com/microsoft/tui-test/bindings/go"
)

const cols, rows = 160, 45

var (
	weaveDir = flag.String("weave", "", "test DIR/weave instead of building ./cmd/weave")
	keep     = flag.Bool("keep", false, "keep a failed test's directory and tmux server")
	shots    = flag.String("shots", "", "write a PNG of the screen each test ends on to DIR")
	record   = flag.String("record", "", "write a GIF of each test's screen, from start to end, to DIR")
	demo     = flag.String("demo", "", "run TestDemo, writing the README's GIF to FILE, with ffmpeg")
	traces   = flag.String("traces", "", "write a trace of each failed test to DIR")
)

// weave is the binary under test, set by TestMain.
var weave string

func TestMain(m *testing.M) {
	if err := tolerateDanglingPointers(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	flag.Parse()
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(code)
}

// tolerateDanglingPointers re-runs the tests with GODEBUG=invalidptr=0. The
// tui-test binding (v0.1.1) hands an empty Rust string to Go as the pointer
// 0x1, as in a cell without a link, and the runtime aborts when it finds one
// while moving a goroutine's stack. The runtime reads invalidptr only as the
// process starts, so setting it here needs a fresh exec.
func tolerateDanglingPointers() error {
	godebug := os.Getenv("GODEBUG")
	if strings.Contains(godebug, "invalidptr=0") {
		return nil
	}
	if godebug != "" {
		godebug += ","
	}
	if err := os.Setenv("GODEBUG", godebug+"invalidptr=0"); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(self, os.Args, os.Environ())
}

func run(m *testing.M) (int, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return 0, fmt.Errorf("tmux not found")
	}
	if *weaveDir != "" {
		weave = filepath.Join(*weaveDir, "weave")
		return m.Run(), nil
	}
	dir, err := os.MkdirTemp("", "wvt-bin-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	weave = filepath.Join(dir, "weave")
	build := exec.Command("go", "build", "-o", weave, "./cmd/weave")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("building weave: %v\n%s", err, out)
	}
	return m.Run(), nil
}

// harness is one test's world: a directory, a tmux server in it, and a
// terminal emulator whose client is attached to that server's term
// session, where weave runs.
type harness struct {
	t      *testing.T
	dir    string
	sock   string
	env    []string
	screen *tuitest.Client
	// roots are the projects' roots the settings list, in order, and
	// preferences the rest of the settings' lines.
	roots, preferences []string
}

// world is what a test's terminal and tmux server start with, when it is
// not the usual. Only the demo asks for more.
type world struct {
	cols, rows uint16
	tmuxConf   string            // appended to the test's tmux config
	tools      map[string]string // programs on PATH, by name, with their sh script
	record     bool              // start recording at once, with -record
}

// start sets up a test's world, running beside the others, and tears it
// down when the test ends.
func start(t *testing.T) *harness {
	t.Helper()
	return startIn(t, world{cols: cols, rows: rows, record: true})
}

// startIn is start in a world of its own.
func startIn(t *testing.T, w world) *harness {
	t.Helper()
	t.Parallel()
	dir := testDir(t)
	h := &harness{t: t, dir: dir, sock: filepath.Join(dir, "tmux.sock")}
	tmuxBin, _ := exec.LookPath("tmux")
	// Every tmux command and the client run in the test's environment,
	// not the caller's: tmux gives the panes a command creates the
	// environment of the client sending it, PATH included.
	h.env = []string{
		"HOME=" + filepath.Join(dir, "home"), "HISTFILE=", "LANG=C.UTF-8", "LC_ALL=C.UTF-8",
		"TERM=tmux-256color", "TMUX_TMPDIR=" + filepath.Join(dir, "tmux"),
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "cfg"),
		"PATH=" + filepath.Dir(weave) + ":" + filepath.Dir(tmuxBin) + ":/usr/bin:/bin",
	}
	h.files()
	h.add(w)
	t.Cleanup(h.clean)

	h.tmux("-f", filepath.Join(dir, "tmux.conf"), "new-session", "-d", "-s", "term",
		"-x", fmt.Sprint(w.cols), "-y", fmt.Sprint(w.rows), "bash", "--norc", "--noprofile")
	options := tuitest.ClientOptions{}
	if *traces != "" {
		options.Trace = &tuitest.TraceOptions{Mode: tuitest.TraceOnFailure, Directory: *traces}
	}
	screen, err := tuitest.Ephemeral("weave", options)
	if err != nil {
		t.Fatal(err)
	}
	h.screen = screen
	// The client sees the terminal the emulator is, not tmux's.
	client := append([]string{"-i"}, h.env...)
	client = append(client, "TERM=xterm-256color", "tmux", "-S", h.sock, "attach", "-t", "term")
	if _, err := screen.Run("env", client, tuitest.SpawnOptions{
		Cols: tuitest.Ptr(w.cols), Rows: tuitest.Ptr(w.rows), WaitReady: tuitest.Ptr(false),
	}); err != nil {
		t.Fatal(err)
	}
	h.eventually("a client attaches to term", func() bool { return h.clientOn() == "term" })
	if *record != "" && w.record {
		if err := screen.StartRecording(filepath.Join(*record, t.Name()+".gif"), tuitest.RecordingOptions{}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	h.promptIn("term")
	h.typed("command -v weave")
	h.sees(weave)
	return h
}

// add sets a world's tools and tmux config up, before the server starts.
func (h *harness) add(w world) {
	h.t.Helper()
	if w.tmuxConf != "" {
		conf, err := os.OpenFile(filepath.Join(h.dir, "tmux.conf"), os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			h.t.Fatal(err)
		}
		defer func() { _ = conf.Close() }()
		if _, err := conf.WriteString(w.tmuxConf); err != nil {
			h.t.Fatal(err)
		}
	}
	if len(w.tools) == 0 {
		return
	}
	bin := filepath.Join(h.dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		h.t.Fatal(err)
	}
	for name, script := range w.tools {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			h.t.Fatal(err)
		}
	}
	for i, kv := range h.env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			h.env[i] = "PATH=" + bin + ":" + path
		}
	}
}

// testDir makes the test's directory, /tmp/wvt- and 8 hex digits: not
// t.TempDir(), whose long paths can pass the 108 bytes a socket's path is
// limited to, and of one length, so that snapshots can hide it without
// moving what is aligned after it.
func testDir(t *testing.T) string {
	t.Helper()
	for range 100 {
		dir := fmt.Sprintf("/tmp/wvt-%08x", rand.Uint32())
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return dir
		}
		if !errors.Is(err, fs.ErrExist) {
			t.Fatal(err)
		}
	}
	t.Fatal("no free test directory under /tmp")
	return ""
}

// files writes the home, the sample projects, each its config in its
// root, the settings listing them, and tmux's config.
func (h *harness) files() {
	h.t.Helper()
	for _, d := range []string{"home", "tmux", "cfg/weaver", "shop/back", "blog", "notes"} {
		if err := os.MkdirAll(filepath.Join(h.dir, d), 0o700); err != nil {
			h.t.Fatal(err)
		}
	}
	samples, err := filepath.Glob("testdata/projects/*.yml")
	if err != nil {
		h.t.Fatal(err)
	}
	for _, sample := range samples {
		config, err := os.ReadFile(sample)
		if err != nil {
			h.t.Fatal(err)
		}
		config = bytes.ReplaceAll(config, []byte("ROOT"), []byte(h.dir))
		h.register(strings.TrimSuffix(filepath.Base(sample), ".yml"), string(config))
	}
	// The panes weave creates run bash whatever the login shell is, with
	// a plain prompt and no login banner.
	h.write("home/.bashrc", "PS1='\\$ '\n")
	h.write("home/.bash_profile", ". ~/.bashrc\n")
	h.write("home/.hushlogin", "")
	bash, _ := exec.LookPath("bash")
	h.write("tmux.conf", "set -g default-terminal tmux-256color\nset -g default-shell "+bash+"\nset -g mouse on\nset -g status-right ''\nset -g automatic-rename off\n")
}

func (h *harness) write(name, content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, name), []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) clean() {
	if *record != "" && h.screen != nil {
		if _, err := h.screen.StopRecording(); err != nil {
			h.t.Logf("recording: %v", err)
		}
	}
	if *shots != "" && h.screen != nil {
		name := filepath.Join(*shots, h.t.Name()+".png")
		if _, err := h.screen.Screenshot(name, tuitest.ScreenshotOptions{}); err != nil {
			h.t.Logf("screenshot: %v", err)
		}
	}
	if h.screen != nil {
		// A failed test's trace is kept, with -traces.
		if err := h.screen.Close(tuitest.CloseOptions{Failed: tuitest.Ptr(h.t.Failed())}); err != nil {
			h.t.Logf("closing the terminal: %v", err)
		}
	}
	if h.t.Failed() && *keep {
		h.t.Logf("kept %s; look with: tmux -S %s attach", h.dir, h.sock)
		return
	}
	_ = exec.Command("tmux", "-S", h.sock, "kill-server").Run()
	_ = os.RemoveAll(h.dir)
}

// tmux runs a tmux command on the test's server, in the test's
// environment, and returns its output.
func (h *harness) tmux(args ...string) string {
	h.t.Helper()
	out, err := h.tmuxErr(args...)
	if err != nil {
		h.t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func (h *harness) tmuxErr(args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-S", h.sock}, args...)...)
	cmd.Env, cmd.Dir = h.env, h.dir
	out, err := cmd.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// clientOn is the session the client is on, or "" without a client.
func (h *harness) clientOn() string {
	out, _ := h.tmuxErr("list-clients", "-F", "#{client_session}")
	return out
}

// --- Keys, mouse and screen -------------------------------------------------

// keys presses keys in the terminal, as a user does: "Enter", "Ctrl+s",
// "Down", or text.
func (h *harness) keys(keys ...string) {
	h.t.Helper()
	if err := h.screen.Press(keys...); err != nil {
		h.t.Fatalf("pressing %q: %v", keys, err)
	}
}

// types types text, as a user does.
func (h *harness) types(text string) {
	h.t.Helper()
	if err := h.screen.Type(text); err != nil {
		h.t.Fatalf("typing %q: %v", text, err)
	}
}

// typed types text then presses Enter.
func (h *harness) typed(text string) {
	h.t.Helper()
	h.types(text)
	h.keys("Enter")
}

// clicks clicks the middle of the one place text shows.
func (h *harness) clicks(text string) {
	h.t.Helper()
	if err := h.screen.GetByText(text, tuitest.TextSelectorOptions{}).Click(tuitest.LocatorClickOptions{}); err != nil {
		h.fatal("clicking %q: %v", text, err)
	}
}

// sees waits until text shows.
func (h *harness) sees(text string) {
	h.t.Helper()
	if err := h.at(text).Expect(tuitest.LocatorExpectOptions{}); err != nil {
		h.fatal("%q never showed", text)
	}
}

// seesWithin waits until text shows, for up to wait: for what weave
// takes longer than sees waits to do.
func (h *harness) seesWithin(text string, wait time.Duration) {
	h.t.Helper()
	if err := h.at(text).Expect(tuitest.LocatorExpectOptions{Timeout: &wait}); err != nil {
		h.fatal("%q never showed within %s", text, wait)
	}
}

// at is where text shows, once or more.
func (h *harness) at(text string) *tuitest.Locator {
	return h.screen.GetByText(text, tuitest.TextSelectorOptions{}).Any()
}

// text is the screen, less its blank lines.
func (h *harness) text() string {
	screen, err := h.screen.Text(tuitest.TextOptions{})
	if err != nil {
		return fmt.Sprintf("(no screen: %v)", err)
	}
	var lines []string
	for _, line := range strings.Split(screen, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, "  | "+strings.TrimRight(line, " "))
		}
	}
	return strings.Join(lines, "\n")
}

// fatal fails the test with what was found, then the screen.
func (h *harness) fatal(format string, args ...any) {
	h.t.Helper()
	h.t.Fatalf(format+"\n%s", append(args, h.text())...)
}

// eventually waits until done, polling every 100 ms for 5 s.
func (h *harness) eventually(what string, done func() bool) {
	h.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if done() {
			return
		}
	}
	h.fatal("%s: never happened", what)
}

// promptIn waits until the pane runs its shell.
func (h *harness) promptIn(target string) {
	h.t.Helper()
	h.waitRuns(target, "bash", 5*time.Second)
}

// browse opens the browser in the pane the client shows, on the project the filter selects
// unless project is empty.
func (h *harness) browse(project string) {
	h.t.Helper()
	h.promptIn(h.here())
	h.typed("clear; weave")
	h.sees("enter open")
	if project == "" {
		return
	}
	h.keys("/")
	h.types(project)
	h.keys("Enter")
	// The filter kept, not still typed: the key line is the browser's.
	h.sees("/" + project + "  enter open")
	h.sees(project + " · weaver.yml")
}

// gone waits until text no longer shows.
func (h *harness) gone(text string) {
	h.t.Helper()
	if err := h.at(text).Expect(tuitest.LocatorExpectOptions{Not: true}); err != nil {
		h.fatal("%q stayed", text)
	}
}

// screenShows waits until one capture of the screen satisfies shows:
// a redraw can match half-made when its facts are read apart.
func (h *harness) screenShows(what string, shows func(screen string) bool) {
	h.t.Helper()
	h.eventually(what, func() bool {
		screen, err := h.screen.Text(tuitest.TextOptions{})
		return err == nil && shows(screen)
	})
}

// --- What tmux holds --------------------------------------------------------

// paneOf is the ID of the pane named label in a session's window.
func (h *harness) paneOf(window, label string) string {
	h.t.Helper()
	for _, line := range strings.Split(h.tmux("list-panes", "-t", "="+window, "-F", "#{pane_id} #{@label}"), "\n") {
		if id, name, _ := strings.Cut(line, " "); name == label {
			return id
		}
	}
	h.t.Fatalf("%s has no pane %s", window, label)
	return ""
}

// waitRuns waits until the pane runs the program, for up to wait.
func (h *harness) waitRuns(pane, program string, wait time.Duration) {
	h.t.Helper()
	var runs string
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if runs, _ = h.tmuxErr("display", "-p", "-t", pane, "#{pane_current_command}"); runs == program {
			return
		}
	}
	h.fatal("%s runs %q, not %q", pane, runs, program)
}

// activePane is the name of the pane a session's current window selects.
func (h *harness) activePane(session string) string {
	return h.tmux("display", "-p", "-t", "="+session+":", "#{@label}")
}

// logsIdle is shop's logs pane, once its sleep 3 exited.
func (h *harness) logsIdle() string {
	h.t.Helper()
	logs := h.paneOf("w/shop:api", "logs")
	h.waitRuns(logs, "bash", 8*time.Second)
	return logs
}

// --- Rendering --------------------------------------------------------------

// laidOut fails the test when the screen's cards aren't closed and
// aligned, or a line is wider than the terminal.
func (h *harness) laidOut() {
	h.t.Helper()
	screen, err := h.screen.Text(tuitest.TextOptions{})
	if err != nil {
		h.t.Fatal(err)
	}
	if problem := layoutProblem(screen); problem != "" {
		h.fatal("the layout is broken: %s", problem)
	}
}

// layoutProblem is the first thing wrong with the screen's cards: a ╭
// with no ╮ on its line, no │ or ╰ under it, or a line wider than the
// terminal; "" when there is none.
func layoutProblem(screen string) string {
	lines := strings.Split(screen, "\n")
	grid := make([][]rune, len(lines))
	for i, line := range lines {
		grid[i] = []rune(line)
		if len(grid[i]) > cols {
			return fmt.Sprintf("line %d is %d cells wide", i+1, len(grid[i]))
		}
	}
	at := func(y, x int) rune {
		if y < len(grid) && x < len(grid[y]) {
			return grid[y][x]
		}
		return ' '
	}
	for y, line := range grid {
		for x, r := range line {
			if r != '╭' {
				continue
			}
			right := x + 1
			for right < len(line) && line[right] != '╮' {
				right++
			}
			if right == len(line) {
				return fmt.Sprintf("the card at line %d, column %d has no top-right corner", y+1, x+1)
			}
			bottom := y + 1
			for ; bottom < len(grid); bottom++ {
				if at(bottom, x) == '╰' {
					break
				}
				if at(bottom, x) != '│' || at(bottom, right) != '│' {
					break
				}
			}
			if bottom < len(grid) && at(bottom, x) != '╰' {
				if cutOff(grid, bottom, x, right) {
					continue
				}
				return fmt.Sprintf("the card at line %d, column %d has a broken side at line %d", y+1, x+1, bottom+1)
			}
			if bottom == len(grid) || at(bottom, right) != '╯' {
				return fmt.Sprintf("the card at line %d, column %d isn't closed under its top", y+1, x+1)
			}
		}
	}
	return ""
}

// colourOf is the colour text is drawn in where it shows once: a
// palette index such as "6", a name or an RGB colour.
func (h *harness) colourOf(text string) string {
	h.t.Helper()
	match, err := h.screen.GetByText(text, tuitest.TextSelectorOptions{}).Location()
	if err != nil {
		h.fatal("%q isn't on the screen once: %v", text, err)
	}
	cells, err := h.screen.Cells(uint16(match.Start.Column), uint16(match.Start.Row), 1, 1)
	if err != nil || len(cells) == 0 {
		h.fatal("no cell under %q: %v", text, err)
	}
	return string(cells[0].FG)
}

// started starts a project from term, with weave <project> --detached,
// and waits for weave to be done with it, back at term's prompt.
func (h *harness) started(project string) {
	h.t.Helper()
	h.promptIn("term")
	h.typed("clear; weave " + project + " --detached")
	h.eventually(project+" starts", func() bool { return h.running("w/" + project) })
	h.promptIn("term")
}

// running is whether the session exists.
func (h *harness) running(session string) bool {
	_, err := h.tmuxErr("has-session", "-t", "="+session)
	return err == nil
}

// switchTo puts the client on a session's window, as a user's tmux
// keys would.
func (h *harness) switchTo(session, window string) {
	h.t.Helper()
	h.tmux("switch-client", "-t", "="+session)
	h.tmux("select-window", "-t", "="+session+":"+window)
	h.eventually("the client switches to "+session, func() bool { return h.clientOn() == session })
}

// here is the pane the client shows.
func (h *harness) here() string {
	out, _ := h.tmuxErr("list-clients", "-F", "#{pane_id}")
	return out
}

// --- Forms, files and panes -------------------------------------------------

// step presses keys and waits until the screen changes.
func (h *harness) step(keys ...string) {
	h.t.Helper()
	before, _ := h.screen.Text(tuitest.TextOptions{})
	h.keys(keys...)
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if now, _ := h.screen.Text(tuitest.TextOptions{}); now != before {
			return
		}
	}
	h.fatal("%q changed nothing", keys)
}

// cursorTo presses Down until the form's › is on a row matching pattern.
func (h *harness) cursorTo(pattern string) {
	h.t.Helper()
	row := regexp.MustCompile(`› +` + pattern)
	for range 60 {
		if screen, _ := h.screen.Text(tuitest.TextOptions{}); row.MatchString(screen) {
			return
		}
		h.step("Down")
	}
	h.fatal("the cursor never reached %q", pattern)
}

// edit opens the project's config form from the browser's e.
func (h *harness) edit(project string) {
	h.t.Helper()
	h.browse(project)
	h.keys("e")
	h.sees("↑↓ move")
}

// settings writes settings.yml: lines, then the projects' roots.
func (h *harness) settings(lines ...string) {
	h.t.Helper()
	h.preferences = lines
	h.writeSettings()
}

// register writes config as weaver.yml in the root project, in the test's
// directory, and lists the root last in the settings.
func (h *harness) register(project, config string) {
	h.t.Helper()
	h.registerIn(project, config)
}

// registerIn is register for the root dir, in the test's directory.
func (h *harness) registerIn(dir, config string) {
	h.t.Helper()
	h.mkdir(dir)
	h.write(filepath.Join(dir, "weaver.yml"), config)
	h.roots = append(h.roots, filepath.Join(h.dir, dir))
	h.writeSettings()
}

// unregister takes the root project off the settings' list.
func (h *harness) unregister(project string) {
	h.t.Helper()
	h.roots = slices.DeleteFunc(h.roots, func(root string) bool { return root == filepath.Join(h.dir, project) })
	h.writeSettings()
}

func (h *harness) writeSettings() {
	h.t.Helper()
	lines := slices.Clone(h.preferences)
	if len(h.roots) > 0 {
		lines = append(lines, "projects:")
		for _, root := range h.roots {
			lines = append(lines, "  - "+root)
		}
	}
	h.write("cfg/weaver/settings.yml", strings.Join(lines, "\n")+"\n")
}

// config is the project's config as stored, in its root.
func (h *harness) config(project string) string {
	h.t.Helper()
	config, err := os.ReadFile(filepath.Join(h.dir, project, "weaver.yml"))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(config)
}

// zoom zooms the pane named label in a session's window.
func (h *harness) zoom(window, label string) {
	h.t.Helper()
	h.tmux("resize-pane", "-Z", "-t", h.paneOf(window, label))
}

// paneShows waits until a pane, shown or not, holds a line containing text.
func (h *harness) paneShows(pane, text string) {
	h.t.Helper()
	h.eventually(fmt.Sprintf("%s shows %q", pane, text), func() bool {
		out, _ := h.tmuxErr("capture-pane", "-p", "-t", pane)
		return strings.Contains(out, text)
	})
}

// paneHasLine waits until a pane holds the line, whole.
func (h *harness) paneHasLine(pane, line string) {
	h.t.Helper()
	h.eventually(fmt.Sprintf("%s has the line %q", pane, line), func() bool {
		out, _ := h.tmuxErr("capture-pane", "-p", "-t", pane)
		return slices.Contains(strings.Split(out, "\n"), line)
	})
}

// drawnIn is whether a cell on the screen is drawn in the colour.
func (h *harness) drawnIn(colour string) bool {
	cells, err := h.screen.Cells(0, 0, cols, rows)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(cells, func(c tuitest.Cell) bool { return string(c.FG) == colour && strings.TrimSpace(c.Char) != "" })
}

// seesLoosely waits until text shows, any run of spaces in it matching
// any other: for columns aligned to the longest value, such as a root
// under the test's directory.
func (h *harness) seesLoosely(text string) {
	h.t.Helper()
	loose := h.screen.GetByText(text, tuitest.TextSelectorOptions{Whitespace: tuitest.Normalize}).Any()
	if err := loose.Expect(tuitest.LocatorExpectOptions{}); err != nil {
		h.fatal("%q never showed", text)
	}
}

// cutOff is whether a card whose sides stop at line bottom is cut by a
// scrolled screen's edge: both sides stop there, and nothing below
// carries on either.
func cutOff(grid [][]rune, bottom, left, right int) bool {
	for y := bottom; y < len(grid); y++ {
		for _, x := range []int{left, right} {
			if x < len(grid[y]) && strings.ContainsRune("│╰╯", grid[y][x]) {
				return false
			}
		}
	}
	return true
}

// --- Snapshots --------------------------------------------------------------

var update = flag.Bool("update", false, "rewrite the snapshots in testdata/snapshots")

// testDirShown stands for the test's directory in snapshots, as long as it.
const testDirShown = "/tmp/wvt-XXXXXXXX"

// matchesSnapshot fails the test when the screen isn't the one in
// testdata/snapshots/<name>.txt, the test's directory hidden; -update
// rewrites it. The layout is checked too.
func (h *harness) matchesSnapshot(name string) {
	h.t.Helper()
	h.laidOut()
	screen, err := h.screen.Text(tuitest.TextOptions{})
	if err != nil {
		h.t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(screen, h.dir, testDirShown), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	got := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	path := filepath.Join("testdata", "snapshots", name+".txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			h.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("no snapshot %s: run with -update to write it", path)
	}
	if string(want) != got {
		h.t.Fatalf("the screen isn't snapshot %s; run with -update to accept it\n%s", path, lineDiff(string(want), got))
	}
}

// lineDiff is the lines of want and got that differ, by number.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var diff []string
	for i := range max(len(w), len(g)) {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			diff = append(diff, fmt.Sprintf("line %d\n  want | %s\n  got  | %s", i+1, a, b))
		}
	}
	return strings.Join(diff, "\n")
}
