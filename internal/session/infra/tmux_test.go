package infra_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// tmuxServer is a private tmux server for one test: its own socket, no
// user config, and /bin/sh panes in an empty home. Tests never touch the
// user's own server.
type tmuxServer struct {
	global []string
	env    []string
}

func newTmuxServer(t *testing.T) tmuxServer {
	t.Helper()
	requireTool(t, "tmux")
	// Not t.TempDir(): its long path would overflow a socket address.
	dir, err := os.MkdirTemp("", "wv")
	if err != nil {
		t.Fatalf("creating socket dir: %v", err)
	}
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return tmuxServer{
		global: []string{"-S", socket, "-f", "/dev/null"},
		env: []string{
			"HOME=" + t.TempDir(),
			"PATH=" + os.Getenv("PATH"),
			"SHELL=/bin/sh",
			"TERM=xterm-256color",
		},
	}
}

// tmux is the adapter under test, talking to this server in a 200x50
// terminal.
func (s tmuxServer) tmux() infra.Tmux {
	return infra.NewTmux(infra.TmuxOptions{
		Global: s.global,
		Env:    s.env,
		Size:   func() (int, int, bool) { return 200, 50, true },
	})
}

// query runs a tmux command against the server and returns its output.
func (s tmuxServer) query(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("tmux", append(slices.Clone(s.global), args...)...)
	cmd.Env = s.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// waitFor waits until pane shows text and returns what it shows.
func (s tmuxServer) waitFor(t *testing.T, pane, text string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		shown := s.query(t, "capture-pane", "-p", "-t", pane)
		if strings.Contains(shown, text) {
			return shown
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %s never showed %q; it shows:\n%s", pane, text, shown)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// busy waits until the first pane of session runs a command starting
// with program, as Capture sees it from outside tmux, and returns its
// startup. A login shell may run others first, such as update-motd.
func (s tmuxServer) busy(t *testing.T, session, program string) domain.Startup {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		windows, err := s.tmux().Capture(session)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		startup := windows[0].Panes()[0].Startup()
		if cmd, ok := startup.Command(); ok && strings.HasPrefix(cmd.String(), program) {
			return startup
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s never ran %s", session, program)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// firstStartup is what Capture types in session's first pane.
func firstStartup(t *testing.T, s tmuxServer, session string) domain.Startup {
	t.Helper()
	windows, err := s.tmux().Capture(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return windows[0].Panes()[0].Startup()
}

// attached is another private server, and a pane of it holding a client
// attached to session. capture-pane leaves out pane borders: reading that
// pane shows what tmux draws, borders included.
func (s tmuxServer) attached(t *testing.T, session string) (tmuxServer, string) {
	t.Helper()
	viewer := newTmuxServer(t)
	attach := []string{"unset TMUX; exec tmux"}
	for _, arg := range append(slices.Clone(s.global), "attach-session", "-t", "="+session) {
		attach = append(attach, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	viewer.query(t, "new-session", "-d", "-s", "viewer", "-x", "200", "-y", "50", strings.Join(attach, " "))
	return viewer, "=viewer:"
}

// paneLabelled is the ID of the pane labelled label in session.
func (s tmuxServer) paneLabelled(t *testing.T, session, label string) string {
	t.Helper()
	for line := range strings.Lines(s.query(t, "list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{@label}")) {
		if id, got, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " "); got == label {
			return id
		}
	}
	t.Fatalf("no pane labelled %q in session %s", label, session)
	return ""
}

// planOf loads doc as project name and resolves it for tmux.
func planOf(t *testing.T, name, doc string) domain.SessionPlan {
	t.Helper()
	project, err := load(t, name, doc)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return project.Plan()
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	return path
}

func TestTmuxStart(t *testing.T) {
	server := newTmuxServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "sub"))
	plan := planOf(t, "start", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "api", "panes": [
			{"name": "shell"},
			{"name": "side", "dir": "sub", "split": {"direction": "right", "size": "30%%"}, "command": "echo w-$((40+2))"},
			{"name": "below", "split": {"direction": "below"}, "prefill": "echo p-$((40+2))"}
		]},
		{"name": "env", "before": ["W=window"], "panes": [
			{"note": ["it's a note"], "env": {"GREETING": "hi $HOME"}, "before": ["P=$W-pane"],
			 "command": "echo \"[$GREETING]\" first-$P"}
		]},
		{"name": "repo"}
	]}`, root))

	if err := server.tmux().Start(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("creates the windows in order", func(t *testing.T) {
		got := server.query(t, "list-windows", "-t", "=start", "-F", "#{window_name}")
		if want := "api\nenv\nrepo"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps the first window current", func(t *testing.T) {
		got := server.query(t, "list-windows", "-t", "=start", "-F", "#{window_active} #{window_name}")
		if want := "1 api\n0 env\n0 repo"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("labels the panes of a split window", func(t *testing.T) {
		got := server.query(t, "list-panes", "-t", "=start:api", "-F", "#{@label}")
		if want := "shell\nside\nbelow"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows the labels in the top border", func(t *testing.T) {
		if got := server.query(t, "show-options", "-w", "-v", "-t", "=start:api", "pane-border-status"); got != "top" {
			t.Fatalf("got %q, want %q", got, "top")
		}
	})

	t.Run("leaves a single-pane window's border alone", func(t *testing.T) {
		if got := server.query(t, "show-options", "-w", "-t", "=start:repo"); strings.Contains(got, "pane-border-status") {
			t.Fatalf("got window options %q, want no pane-border-status", got)
		}
	})

	t.Run("sizes a split as a share of the split pane", func(t *testing.T) {
		side := server.paneLabelled(t, "start", "side")
		if got := server.query(t, "display-message", "-p", "-t", side, "#{pane_width}"); got != "60" {
			t.Fatalf("got %s columns, want 60 (30%% of 200)", got)
		}
	})

	t.Run("starts a pane in its directory", func(t *testing.T) {
		side := server.paneLabelled(t, "start", "side")
		if got := server.query(t, "display-message", "-p", "-t", side, "#{pane_current_path}"); got != filepath.Join(root, "sub") {
			t.Fatalf("got %q, want %q", got, filepath.Join(root, "sub"))
		}
	})

	t.Run("runs a pane's command", func(t *testing.T) {
		server.waitFor(t, server.paneLabelled(t, "start", "side"), "w-42")
	})

	t.Run("leaves a prefilled command at the prompt until Enter", func(t *testing.T) {
		below := server.paneLabelled(t, "start", "below")
		server.waitFor(t, below, "echo p-$((40+2))")
		time.Sleep(300 * time.Millisecond)
		if shown := server.query(t, "capture-pane", "-p", "-t", below); strings.Contains(shown, "p-42") {
			t.Fatalf("the prefilled command ran by itself:\n%s", shown)
		}
		server.query(t, "send-keys", "-t", below, "Enter")
		server.waitFor(t, below, "p-42")
	})

	t.Run("prints a pane's note", func(t *testing.T) {
		server.waitFor(t, "=start:env", "it's a note")
	})

	t.Run("exports a pane's env literally", func(t *testing.T) {
		server.waitFor(t, "=start:env", "[hi $HOME]")
	})

	t.Run("runs the window's and the pane's commands before the pane's own", func(t *testing.T) {
		server.waitFor(t, "=start:env", "first-window-pane")
	})
}

func TestTmuxStartClearing(t *testing.T) {
	server := newTmuxServer(t)
	plan := planOf(t, "cleared", fmt.Sprintf(`{"root": %q, "clear": true, "windows": [
		{"name": "w", "panes": [
			{"before": ["echo hidden-$((1+1))"], "note": ["kept note"], "command": "echo shown-$((2+2))"}
		]}
	]}`, t.TempDir()))

	if err := server.tmux().Start(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	server.waitFor(t, "=cleared:w", "shown-4")

	t.Run("hides the before commands once they ran", func(t *testing.T) {
		if shown := server.query(t, "capture-pane", "-p", "-t", "=cleared:w"); strings.Contains(shown, "hidden") {
			t.Fatalf("the before command still shows:\n%s", shown)
		}
	})

	t.Run("prints the note after clearing", func(t *testing.T) {
		server.waitFor(t, "=cleared:w", "kept note")
	})
}

func TestTmuxStartEndsAPaneScriptWithTheDefaultCommand(t *testing.T) {
	server := newTmuxServer(t)
	// In the config, read by the server Start starts: there is none before.
	config := filepath.Join(t.TempDir(), "tmux.conf")
	if err := os.WriteFile(config, []byte(`set -g default-command "echo by-default; exec /bin/sh"`+"\n"), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
	server.global[3] = config
	project, err := load(t, "dflt", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "first", "panes": [{"note": ["first note"]}]},
		{"name": "second", "panes": [{"name": "a", "env": {"X": "1"}}, {"name": "b", "env": {"X": "1"}, "split": {"direction": "right"}}]},
		{"name": "third", "panes": [{"note": ["third note"]}]}
	]}`, t.TempDir()))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if err := server.tmux().Start(project.Plan()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("in the session's first pane", func(t *testing.T) {
		server.waitFor(t, "=dflt:first", "first note")
		server.waitFor(t, "=dflt:first", "by-default")
	})

	t.Run("in a later window's first pane", func(t *testing.T) {
		server.waitFor(t, server.paneLabelled(t, "dflt", "a"), "by-default")
	})

	t.Run("in a split pane", func(t *testing.T) {
		server.waitFor(t, server.paneLabelled(t, "dflt", "b"), "by-default")
	})

	t.Run("in a restored window", func(t *testing.T) {
		server.query(t, "kill-window", "-t", "=dflt:third")
		session, _, err := server.tmux().Find(project.Name())
		if err != nil {
			t.Fatalf("finding: %v", err)
		}
		if err := server.tmux().Restore(project.Name(), project.Restoration(session.Windows())); err != nil {
			t.Fatalf("restoring: %v", err)
		}
		server.waitFor(t, "=dflt:third", "third note")
		server.waitFor(t, "=dflt:third", "by-default")
	})
}

func TestTmuxStartPassesTextVerbatim(t *testing.T) {
	server := newTmuxServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "x#Sy"))
	plan := planOf(t, "verbatim", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "a#Sb", "panes": [
			{"name": "semicolon", "dir": "x#Sy", "command": "printf '%%s%%s\\n' semi colon\\;"},
			{"name": "dash", "split": {"direction": "right"}, "prefill": "-n leading-dash"},
			{"name": "a##b #[reverse]c", "split": {"direction": "below"}}
		]}
	]}`, root))

	if err := server.tmux().Start(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("keeps a trailing semicolon", func(t *testing.T) {
		server.waitFor(t, server.paneLabelled(t, "verbatim", "semicolon"), "semicolon;")
	})

	t.Run("keeps a leading dash", func(t *testing.T) {
		server.waitFor(t, server.paneLabelled(t, "verbatim", "dash"), "-n leading-dash")
	})

	t.Run("keeps a hash in a window name", func(t *testing.T) {
		if got := server.query(t, "list-windows", "-t", "=verbatim", "-F", "#{window_name}"); got != "a#Sb" {
			t.Fatalf("got %q, want %q", got, "a#Sb")
		}
	})

	t.Run("draws a hash in a pane name as written", func(t *testing.T) {
		viewer, pane := server.attached(t, "verbatim")
		viewer.waitFor(t, pane, " a##b #[reverse]c ")
	})

	t.Run("keeps a hash in a directory", func(t *testing.T) {
		pane := server.paneLabelled(t, "verbatim", "semicolon")
		if got := server.query(t, "display-message", "-p", "-t", pane, "#{pane_current_path}"); got != filepath.Join(root, "x#Sy") {
			t.Fatalf("got %q, want %q", got, filepath.Join(root, "x#Sy"))
		}
	})
}

// tooManyPanes is the panes of a window that can't be laid out: halving
// a 50-line pane twelve times runs out of lines.
func tooManyPanes() string {
	panes := []string{`{"name": "p0"}`}
	for i := 1; i <= 12; i++ {
		panes = append(panes, fmt.Sprintf(`{"name": "p%d", "split": {"direction": "below"}}`, i))
	}
	return strings.Join(panes, ",")
}

func TestTmuxStartLeavesNothingWhenAStepFails(t *testing.T) {
	server := newTmuxServer(t)
	plan := planOf(t, "broken", fmt.Sprintf(`{"root": %q, "windows": [{"name": "main", "panes": [%s]}]}`,
		t.TempDir(), tooManyPanes()))
	name, _ := domain.NewProjectName("broken")

	if err := server.tmux().Start(plan); err == nil {
		t.Fatal("got no error, want a split to run out of space")
	}

	if server.running(t, name) {
		t.Fatal("got the session running, want the half-built session gone")
	}
}

func TestTmuxStartLeavesNothingWhenALaterWindowFails(t *testing.T) {
	server := newTmuxServer(t)
	plan := planOf(t, "broken", fmt.Sprintf(`{"root": %q, "windows": [{"name": "fine"}, {"name": "main", "panes": [%s]}]}`,
		t.TempDir(), tooManyPanes()))
	name, _ := domain.NewProjectName("broken")

	if err := server.tmux().Start(plan); err == nil {
		t.Fatal("got no error, want a split in the second window to run out of space")
	}

	if server.running(t, name) {
		t.Fatal("got the session running, want the half-built session gone")
	}
}

// unreachable is a running server whose socket weave may not open, as
// one another user started.
func unreachable(t *testing.T) tmuxServer {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root opens any socket")
	}
	server := newTmuxServer(t)
	server.query(t, "new-session", "-d", "-s", "shop")
	socket := server.global[1]
	if err := os.Chmod(socket, 0); err != nil {
		t.Fatalf("closing the socket: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(socket, 0o700) })
	return server
}

// stale is a server's socket left behind once it is gone, so connecting
// is refused.
func stale(t *testing.T) tmuxServer {
	t.Helper()
	server := newTmuxServer(t)
	listener, err := net.Listen("unix", server.global[1])
	if err != nil {
		t.Fatalf("leaving a socket: %v", err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = listener.Close()
	return server
}

func TestTmuxUnreachableServer(t *testing.T) {
	server := unreachable(t)
	name, _ := domain.NewProjectName("shop")

	t.Run("listing fails rather than finding nothing running", func(t *testing.T) {
		if sessions, err := server.tmux().Running(); err == nil {
			t.Fatalf("got %q, want an error", sessions)
		}
	})

	t.Run("finding fails rather than finding nothing", func(t *testing.T) {
		if _, _, err := server.tmux().Find(name); err == nil {
			t.Fatal("got no error, want one")
		}
	})

	t.Run("killing says why it failed", func(t *testing.T) {
		err := server.tmux().Kill(name)
		if err == nil || errors.Is(err, domain.ErrSessionNotRunning) || !strings.Contains(err.Error(), "Permission denied") {
			t.Fatalf("got %v, want tmux's permission error", err)
		}
	})
}

func TestTmuxStaleSocket(t *testing.T) {
	server := stale(t)

	t.Run("lists no sessions", func(t *testing.T) {
		if sessions, err := server.tmux().Running(); err != nil || len(sessions) != 0 {
			t.Fatalf("got %q (%v), want none running", sessions, err)
		}
	})

	t.Run("finds no session", func(t *testing.T) {
		name, _ := domain.NewProjectName("shop")
		if _, running, err := server.tmux().Find(name); err != nil || running {
			t.Fatalf("got running=%v (%v), want not running", running, err)
		}
	})
}

func TestTmuxKill(t *testing.T) {
	server := newTmuxServer(t)
	name, _ := domain.NewProjectName("doomed")

	t.Run("reports a session that isn't running", func(t *testing.T) {
		if err := server.tmux().Kill(name); !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("kills a running session", func(t *testing.T) {
		if err := server.tmux().Start(planOf(t, "doomed", fmt.Sprintf(`{"root": %q}`, t.TempDir()))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := server.tmux().Kill(name); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if server.running(t, name) {
			t.Fatal("got the session running, want it gone")
		}
	})
}

func TestTmuxRunning(t *testing.T) {
	server := newTmuxServer(t)

	t.Run("lists no sessions when no server runs", func(t *testing.T) {
		if sessions, err := server.tmux().Running(); err != nil || len(sessions) != 0 {
			t.Fatalf("got %q (%v), want none", sessions, err)
		}
	})

	t.Run("lists the running sessions", func(t *testing.T) {
		for _, name := range []string{"api", "front"} {
			if err := server.tmux().Start(planOf(t, name, fmt.Sprintf(`{"root": %q}`, t.TempDir()))); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}

		sessions, err := server.tmux().Running()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slices.Sort(sessions)
		if want := []string{"api", "front"}; !slices.Equal(sessions, want) {
			t.Fatalf("got %q, want %q", sessions, want)
		}
	})

	t.Run("lists a session with a dollar so it can be captured", func(t *testing.T) {
		server := newTmuxServer(t)
		server.query(t, "new-session", "-d", "-s", "a$b") // tmux names it a\$b
		sessions, err := server.tmux().Running()
		if err != nil || len(sessions) != 1 {
			t.Fatalf("got %q (%v), want one session", sessions, err)
		}
		if _, err := server.tmux().Capture(sessions[0]); err != nil {
			t.Fatalf("capturing %q: %v", sessions[0], err)
		}
	})
}

func TestTmuxFind(t *testing.T) {
	server := newTmuxServer(t)
	shop, _ := domain.NewProjectName("shop")

	t.Run("reports no session when no server runs", func(t *testing.T) {
		if server.running(t, shop) {
			t.Fatal("got shop running, want no session")
		}
	})

	if err := server.tmux().Start(planOf(t, "shop-api", fmt.Sprintf(`{"root": %q, "windows": [{"name": "api"}, {"name": "app"}]}`, t.TempDir()))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shopAPI, _ := domain.NewProjectName("shop-api")

	t.Run("reports a session it started as owned", func(t *testing.T) {
		if session, running, err := server.tmux().Find(shopAPI); err != nil || !running || !session.Owned() {
			t.Fatalf("got running=%v owned=%v (%v), want an owned session", running, session.Owned(), err)
		}
	})

	t.Run("reports the project windows the session has", func(t *testing.T) {
		server.query(t, "rename-window", "-t", "shop-api:app", "renamed by hand")
		session, _, err := server.tmux().Find(shopAPI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, w := range session.Windows() {
			got = append(got, w.String())
		}
		if want := []string{"api", "app"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reads a window name with a dollar as written", func(t *testing.T) {
		if err := server.tmux().Start(planOf(t, "dollar", fmt.Sprintf(`{"root": %q, "windows": [{"name": "run $HOME ${X}"}]}`, t.TempDir()))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dollar, _ := domain.NewProjectName("dollar")
		session, _, err := server.tmux().Find(dollar)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := session.Windows(); len(got) != 1 || got[0].String() != "run $HOME ${X}" {
			t.Fatalf("got %q, want %q", got, "run $HOME ${X}")
		}
	})

	t.Run("matches session names exactly, not by prefix", func(t *testing.T) {
		if server.running(t, shop) {
			t.Fatal("got shop running, want only shop-api")
		}
	})

	t.Run("reports a session started otherwise as not owned", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "plain")
		plain, _ := domain.NewProjectName("plain")
		if session, running, err := server.tmux().Find(plain); err != nil || !running || session.Owned() {
			t.Fatalf("got running=%v owned=%v (%v), want a running session not owned", running, session.Owned(), err)
		}
	})
}

func TestTmuxRestore(t *testing.T) {
	doc := fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "a"},
		{"name": "b", "panes": [
			{"name": "left"},
			{"name": "right", "split": {"direction": "right"}, "command": "echo restored-$((40+2))"}
		]},
		{"name": "c"}
	]}`, t.TempDir())

	// restore starts the project, adds a window by hand, closes windows,
	// lets weaver restore them, and returns the window names.
	restore := func(t *testing.T, closed ...string) (tmuxServer, string) {
		t.Helper()
		server := newTmuxServer(t)
		project, err := load(t, "restore", doc)
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		if err := server.tmux().Start(project.Plan()); err != nil {
			t.Fatalf("starting: %v", err)
		}
		server.query(t, "new-window", "-d", "-t", "restore:", "-n", "manual")
		for _, window := range closed {
			server.query(t, "kill-window", "-t", "restore:"+window)
		}

		session, _, err := server.tmux().Find(project.Name())
		if err != nil {
			t.Fatalf("finding: %v", err)
		}
		if err := server.tmux().Restore(project.Name(), project.Restoration(session.Windows())); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return server, server.query(t, "list-windows", "-t", "=restore", "-F", "#{window_name}")
	}

	t.Run("puts closed windows back in place", func(t *testing.T) {
		if _, got := restore(t, "a", "b"); got != "a\nb\nc\nmanual" {
			t.Fatalf("got windows %q, want a, b, c, manual", got)
		}
	})

	t.Run("appends them when none of the project's is left", func(t *testing.T) {
		if _, got := restore(t, "a", "b", "c"); got != "manual\na\nb\nc" {
			t.Fatalf("got windows %q, want manual, a, b, c", got)
		}
	})

	t.Run("lays out and starts a restored window's panes", func(t *testing.T) {
		server, _ := restore(t, "b")
		server.waitFor(t, server.paneLabelled(t, "restore", "right"), "restored-42")
	})

	t.Run("puts a window back between ones named with a dollar", func(t *testing.T) {
		server := newTmuxServer(t)
		project, err := load(t, "dollar", fmt.Sprintf(`{"root": %q, "windows": [{"name": "$a"}, {"name": "b"}, {"name": "$c"}]}`, t.TempDir()))
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		if err := server.tmux().Start(project.Plan()); err != nil {
			t.Fatalf("starting: %v", err)
		}
		server.query(t, "kill-window", "-t", "dollar:b")
		server.query(t, "new-window", "-d", "-a", "-t", "dollar:$c", "-n", "manual")

		session, _, err := server.tmux().Find(project.Name())
		if err != nil {
			t.Fatalf("finding: %v", err)
		}
		if err := server.tmux().Restore(project.Name(), project.Restoration(session.Windows())); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// tmux 3.4 prints a "\" before each "$" here.
		got := strings.ReplaceAll(server.query(t, "list-windows", "-t", "=dollar", "-F", "#{window_name}"), `\$`, "$")
		if want := "$a\nb\n$c\nmanual"; got != want {
			t.Fatalf("got windows %q, want %q", got, want)
		}
	})

	t.Run("removes a window it fails to lay out", func(t *testing.T) {
		server := newTmuxServer(t)
		root := t.TempDir()
		if err := server.tmux().Start(planOf(t, "shrunk", fmt.Sprintf(`{"root": %q, "windows": [{"name": "a"}]}`, root))); err != nil {
			t.Fatalf("starting: %v", err)
		}
		project, err := load(t, "shrunk", fmt.Sprintf(`{"root": %q, "windows": [{"name": "a"}, {"name": "tall", "panes": [%s]}]}`,
			root, tooManyPanes()))
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		session, _, err := server.tmux().Find(project.Name())
		if err != nil {
			t.Fatalf("finding: %v", err)
		}

		if err := server.tmux().Restore(project.Name(), project.Restoration(session.Windows())); err == nil {
			t.Fatal("got no error, want a split to run out of space")
		}
		if got := server.query(t, "list-windows", "-t", "=shrunk", "-F", "#{window_name}"); got != "a" {
			t.Fatalf("got windows %q, want only a", got)
		}
	})
}

// running reports whether a session named after the project runs.
func (s tmuxServer) running(t *testing.T, name domain.ProjectName) bool {
	t.Helper()
	_, running, err := s.tmux().Find(name)
	if err != nil {
		t.Fatalf("finding %s: %v", name, err)
	}
	return running
}

// layoutOf describes windows as "window: pane@dir | pane@dir right of #1 40%".
func layoutOf(t *testing.T, windows []domain.Window, root string) []string {
	t.Helper()
	var out []string
	for _, w := range windows {
		var panes []string
		for _, p := range w.Panes() {
			text := p.Name().String() + "@" + strings.TrimPrefix(strings.TrimPrefix(ownDir(t, p), root), "/")
			if s, ok := p.Split(); ok {
				text += fmt.Sprintf(" %s of #%d %s", s.Direction(), s.Of()+1, s.Size())
			}
			panes = append(panes, text)
		}
		out = append(out, w.Name().String()+": "+strings.Join(panes, " | "))
	}
	return out
}

func TestTmuxCapture(t *testing.T) {
	server := newTmuxServer(t)
	root := t.TempDir()
	for _, dir := range []string{"api", "app"} {
		mkdir(t, filepath.Join(root, dir))
	}

	t.Run("reads back the layout weaver started", func(t *testing.T) {
		if err := server.tmux().Start(planOf(t, "capture", fmt.Sprintf(`{"root": %q, "windows": [
			{"name": "api", "panes": [
				{"name": "api shell", "dir": "api"},
				{"name": "task dev", "dir": "api", "split": {"direction": "right", "size": "40%%"}},
				{"name": "psql", "split": {"direction": "below", "size": "30%%"}}
			]},
			{"name": "app", "panes": [
				{"name": "side", "dir": "app"},
				{"name": "main", "split": {"of": "side", "direction": "right", "size": "70%%"}},
				{"name": "under side", "split": {"of": "side", "direction": "below", "size": "20%%"}}
			]}
		]}`, root))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		windows, err := server.tmux().Capture("capture")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{
			"api: api shell@api | task dev@api right of #1 40% | psql@ below of #2 30%",
			"app: side@app | main@ right of #1 70% | under side@ below of #1 20%",
		}
		if got := layoutOf(t, windows, root); !slices.Equal(got, want) {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("reads a session weaver didn't start", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "raw", "-x", "200", "-y", "50", "-c", root)
		server.query(t, "split-window", "-h", "-l", "25%", "-t", "raw:", "-c", filepath.Join(root, "app"))

		windows, err := server.tmux().Capture("raw")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := layoutOf(t, windows, root)[0], "pane 1@ | pane 2@app right of #1 25%"; !strings.HasSuffix(got, want) {
			t.Fatalf("got %q, want it to end with %q", got, want)
		}
	})

	t.Run("reads names and directories with a dollar as written", func(t *testing.T) {
		dir := filepath.Join(root, "d $HOME")
		mkdir(t, dir)
		server.query(t, "new-session", "-d", "-s", "dollar", "-n", "w ${X}", "-c", dir)
		server.query(t, "set-option", "-p", "-t", "dollar:", "@label", "run $HOME")
		server.waitFor(t, "dollar:", "$") // the shell is in its directory

		windows, err := server.tmux().Capture("dollar")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := layoutOf(t, windows, root), []string{"w ${X}: run $HOME@d $HOME"}; !slices.Equal(got, want) {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("types the command in a pane's foreground without running it", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "busy")
		server.query(t, "send-keys", "-t", "busy:", "sleep 300", "Enter")

		startup := server.busy(t, "busy", "sleep")
		if cmd, _ := startup.Command(); cmd.String() != "sleep 300" || startup.Submits() {
			t.Fatalf("got command %q (run: %t), want sleep 300 only typed", cmd, startup.Submits())
		}
	})

	t.Run("keeps the directory of the shell a job left", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "moved", "-c", root)
		server.query(t, "send-keys", "-t", "moved:", "(cd api && exec sleep 300)", "Enter")
		server.busy(t, "moved", "sleep")

		windows, err := server.tmux().Capture("moved")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ownDir(t, windows[0].Panes()[0]); got != root {
			t.Fatalf("got dir %q, want the shell's %q", got, root)
		}
	})

	t.Run("keeps the directory of a busy shell as written once removed", func(t *testing.T) {
		gone := filepath.Join(root, "gone")
		mkdir(t, gone)
		server.query(t, "new-session", "-d", "-s", "gone", "-c", gone)
		server.query(t, "send-keys", "-t", "gone:", "sleep 300", "Enter")
		server.busy(t, "gone", "sleep")
		if err := os.Remove(gone); err != nil {
			t.Fatalf("removing %s: %v", gone, err)
		}

		windows, err := server.tmux().Capture("gone")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ownDir(t, windows[0].Panes()[0]); got != gone {
			t.Fatalf("got dir %q, want %q", got, gone)
		}
	})

	t.Run("gives the pane weaver runs in no command", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "self")
		pane := server.query(t, "display-message", "-p", "-t", "self:", "#{pane_id}")
		server.query(t, "send-keys", "-t", pane, "sleep 300", "Enter")
		server.busy(t, "self", "sleep")
		tmux := infra.NewTmux(infra.TmuxOptions{Global: server.global, Env: server.env, Pane: pane})

		windows, err := tmux.Capture("self")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd, ok := windows[0].Panes()[0].Startup().Command(); ok {
			t.Fatalf("got command %q, want none: it would be weaver itself", cmd)
		}
	})

	t.Run("gives a pane idle at its prompt no command", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "idle")
		server.waitFor(t, "idle:", "$")

		windows, err := server.tmux().Capture("idle")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd, ok := windows[0].Panes()[0].Startup().Command(); ok {
			t.Fatalf("got command %q, want none", cmd)
		}
	})

	t.Run("types the command a pane was started with without running it", func(t *testing.T) {
		started := `echo "it's $HOME" ~ '#x'; sleep 300`
		server.query(t, "new-session", "-d", "-s", "started", started)

		startup := firstStartup(t, server, "started")
		if cmd, _ := startup.Command(); cmd.String() != started || startup.Submits() {
			t.Fatalf("got command %q (run: %t), want %q only typed", cmd, startup.Submits(), started)
		}
	})

	t.Run("quotes the arguments a pane was started with", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "argv", "--",
			"/bin/sh", "-c", "sleep 300", "it's", "~", "a=b", "=x", "", `back\slash`, `"`, "a#$z")

		cmd, _ := firstStartup(t, server, "argv").Command()
		if want := `/bin/sh -c 'sleep 300' 'it'\''s' '~' a=b '=x' '' 'back\slash' '"' 'a#$z'`; cmd.String() != want {
			t.Fatalf("got command %q, want %q", cmd, want)
		}
	})

	t.Run("keeps the directory a started command changed from", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "cd", "-c", root, "cd api && sleep 300")
		server.busy(t, "cd", "cd api")

		windows, err := server.tmux().Capture("cd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ownDir(t, windows[0].Panes()[0]); got != root {
			t.Fatalf("got dir %q, want where the command started, %q", got, root)
		}
	})

	t.Run("gives a pane started with tmux's default-command no command", func(t *testing.T) {
		server.query(t, "set-option", "-g", "default-command", "exec /bin/sh")
		t.Cleanup(func() { server.query(t, "set-option", "-gu", "default-command") })
		server.query(t, "new-session", "-d", "-s", "default")
		server.waitFor(t, "default:", "$")

		if cmd, ok := firstStartup(t, server, "default").Command(); ok {
			t.Fatalf("got command %q, want none", cmd)
		}
	})

	t.Run("gives a pane weaver started with a note no command", func(t *testing.T) {
		if err := server.tmux().Start(planOf(t, "noted", fmt.Sprintf(`{"root": %q, "windows": [
			{"name": "w", "panes": [{"note": ["hello"], "env": {"A": "1"}}]}
		]}`, root))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server.waitFor(t, "=noted:", "hello")

		if cmd, ok := firstStartup(t, server, "noted").Command(); ok {
			t.Fatalf("got command %q, want none", cmd)
		}
	})

	t.Run("reports a session that isn't running", func(t *testing.T) {
		if _, err := server.tmux().Capture("nosuch"); !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("numbers pane names that differ only by spaces", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "spaced")
		server.query(t, "split-window", "-d", "-h", "-t", "spaced:")
		server.query(t, "set-option", "-p", "-t", "spaced:.0", "@label", "x")
		server.query(t, "set-option", "-p", "-t", "spaced:.1", "@label", "x ")

		windows, err := server.tmux().Capture("spaced")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var names []string
		for _, p := range windows[0].Panes() {
			names = append(names, p.Name().String())
		}
		if want := []string{"x", "x 2"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("numbers window names taken already", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "twins", "-n", "bash")
		server.query(t, "new-window", "-d", "-t", "twins:", "-n", "bash")

		windows, err := server.tmux().Capture("twins")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var names []string
		for _, w := range windows {
			names = append(names, w.Name().String())
		}
		if want := []string{"bash", "bash 2"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})
}

func TestTmuxCurrent(t *testing.T) {
	server := newTmuxServer(t)

	t.Run("names the session of the pane weaver runs in", func(t *testing.T) {
		server.query(t, "new-session", "-d", "-s", "first")
		server.query(t, "new-session", "-d", "-s", "second")
		tmux := infra.NewTmux(infra.TmuxOptions{
			Global: server.global,
			Env:    server.env,
			Pane:   server.query(t, "display-message", "-p", "-t", "first:", "#{pane_id}"),
		})

		got, err := tmux.Current()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "first" {
			t.Fatalf("got %q, want first", got)
		}
	})

	t.Run("names a session with a dollar so it can be captured", func(t *testing.T) {
		tmux := infra.NewTmux(infra.TmuxOptions{
			Global: server.global,
			Env:    server.env,
			Pane:   server.query(t, "new-session", "-d", "-s", "a$b", "-P", "-F", "#{pane_id}"),
		})

		// tmux itself names the session a\$b.
		got, err := tmux.Current()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := tmux.Capture(got); err != nil {
			t.Fatalf("capturing %q: %v", got, err)
		}
	})

	t.Run("names the most recently active session without a pane, as in a popup", func(t *testing.T) {
		server := newTmuxServer(t)
		server.query(t, "new-session", "-d", "-s", "only")
		tmux := infra.NewTmux(infra.TmuxOptions{Global: server.global, Env: server.env, Inside: true})

		got, err := tmux.Current()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "only" {
			t.Fatalf("got %q, want only", got)
		}
	})

	t.Run("reports running outside tmux", func(t *testing.T) {
		if _, err := server.tmux().Current(); !errors.Is(err, domain.ErrNotInsideTmux) {
			t.Fatalf("got %v, want ErrNotInsideTmux", err)
		}
	})
}

// TestTmuxTasks' cases run in order on one session: the pane they type
// into gets busy along the way.
func TestTmuxTasks(t *testing.T) {
	server := newTmuxServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "back"))
	plan := planOf(t, "tasks", fmt.Sprintf(`{"root": %q, "before": ["B=before"], "windows": [
		{"name": "api", "panes": [
			{"name": "shell"},
			{"name": "back", "dir": "back", "env": {"GREETING": "hi"}, "split": {"direction": "right"},
			 "tasks": [{"name": "greet", "command": "echo \"$GREETING-$B\" task-$((40+2))"}]}
		]},
		{"name": "logs", "panes": [{"tasks": [{"command": "echo logs-$((40+2))"}]}]}
	]}`, root))
	if err := server.tmux().Start(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	project := plan.Name()
	api, logs := plan.Windows()[0], plan.Windows()[1]
	greet := api.Panes()[1].Tasks()[0]
	back := server.paneLabelled(t, "tasks", "back")
	server.waitFor(t, back, "$")

	t.Run("finds an idle pane's shell not busy", func(t *testing.T) {
		busy, err := server.tmux().Busy(project, api, 1)
		if err != nil || busy {
			t.Fatalf("got busy %t (err %v), want idle", busy, err)
		}
	})

	t.Run("types a task into the pane labelled with its name", func(t *testing.T) {
		if err := server.tmux().Type(project, api, 1, greet); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server.waitFor(t, back, "hi-before task-42")
	})

	t.Run("cuts what is at the prompt before typing a task", func(t *testing.T) {
		server.query(t, "send-keys", "-t", back, "-l", "echo prefilled")
		if err := server.tmux().Type(project, api, 1, greet); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shown := server.waitFor(t, back, "$ echo \"$GREETING-$B\" task-$((40+2))\nhi-before task-42")
		if strings.Contains(shown, "prefilled") {
			t.Fatalf("the task was typed after what was at the prompt:\n%s", shown)
		}
	})

	t.Run("finds a pane in copy mode busy", func(t *testing.T) {
		server.query(t, "copy-mode", "-t", back)
		defer server.query(t, "send-keys", "-t", back, "-X", "cancel")
		if busy, err := server.tmux().Busy(project, api, 1); err != nil || !busy {
			t.Fatalf("got busy %t (err %v), want busy", busy, err)
		}
	})

	t.Run("reports a pane its window doesn't have", func(t *testing.T) {
		renamed := planOf(t, "tasks", fmt.Sprintf(`{"root": %q, "windows": [
			{"name": "api", "panes": [{"name": "shell"}, {"name": "gone", "split": {"direction": "right"}}]}
		]}`, root)).Windows()[0]
		if _, err := server.tmux().Busy(project, renamed, 1); !errors.Is(err, domain.ErrPaneNotRunning) {
			t.Fatalf("got %v, want ErrPaneNotRunning", err)
		}
	})

	t.Run("types a task into a window's only pane", func(t *testing.T) {
		if err := server.tmux().Type(project, logs, 0, logs.Panes()[0].Tasks()[0]); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server.waitFor(t, "=tasks:logs", "logs-42")
	})

	t.Run("finds a pane running a command busy", func(t *testing.T) {
		server.query(t, "send-keys", "-t", back, "sleep 300", "Enter")
		deadline := time.Now().Add(5 * time.Second)
		for {
			busy, err := server.tmux().Busy(project, api, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if busy {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the pane never got busy")
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("runs a task apart in a throwaway window, in the pane's dir with its env and before commands", func(t *testing.T) {
		if err := server.tmux().RunApart(project, api.Panes()[1], greet); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		shown := server.waitFor(t, "=tasks:greet", "hi-before task-42")
		if got := server.query(t, "display-message", "-p", "-t", "=tasks:greet", "#{pane_current_path}"); got != filepath.Join(root, "back") {
			t.Fatalf("got the throwaway window in %q, want %q; it shows:\n%s", got, filepath.Join(root, "back"), shown)
		}
	})

	t.Run("keeps the user's window current", func(t *testing.T) {
		if got := server.query(t, "display-message", "-p", "-t", "=tasks:", "#{window_name}"); got != "api" {
			t.Fatalf("got %q current, want api", got)
		}
	})

	t.Run("skips the throwaway window when capturing", func(t *testing.T) {
		windows, err := server.tmux().Capture("tasks")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(windows) != 2 {
			t.Fatalf("got %d windows, want api and logs only", len(windows))
		}
	})

	t.Run("doesn't count the throwaway window as the project's", func(t *testing.T) {
		session, _, err := server.tmux().Find(project)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := session.Windows(); len(got) != 2 {
			t.Fatalf("got windows %v, want api and logs", got)
		}
	})

	t.Run("reports a pane whose window is closed", func(t *testing.T) {
		server.query(t, "kill-window", "-t", "=tasks:logs")
		if _, err := server.tmux().Busy(project, logs, 0); !errors.Is(err, domain.ErrPaneNotRunning) {
			t.Fatalf("got %v, want ErrPaneNotRunning", err)
		}
	})
}
