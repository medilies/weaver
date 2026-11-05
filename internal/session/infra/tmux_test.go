package infra_test

import (
	"errors"
	"fmt"
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

func TestTmuxStartPassesTextVerbatim(t *testing.T) {
	server := newTmuxServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "x#Sy"))
	plan := planOf(t, "verbatim", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "a#Sb", "panes": [
			{"name": "semicolon", "dir": "x#Sy", "command": "printf '%%s%%s\\n' semi colon\\;"},
			{"name": "dash", "split": {"direction": "right"}, "prefill": "-n leading-dash"}
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

	t.Run("keeps a hash in a directory", func(t *testing.T) {
		pane := server.paneLabelled(t, "verbatim", "semicolon")
		if got := server.query(t, "display-message", "-p", "-t", pane, "#{pane_current_path}"); got != filepath.Join(root, "x#Sy") {
			t.Fatalf("got %q, want %q", got, filepath.Join(root, "x#Sy"))
		}
	})
}

func TestTmuxStartLeavesNothingWhenAStepFails(t *testing.T) {
	server := newTmuxServer(t)
	// Halving a 50-line pane twelve times runs out of lines.
	panes := []string{`{"name": "p0"}`}
	for i := 1; i <= 12; i++ {
		panes = append(panes, fmt.Sprintf(`{"name": "p%d", "split": {"direction": "below"}}`, i))
	}
	plan := planOf(t, "broken", fmt.Sprintf(`{"root": %q, "windows": [{"name": "main", "panes": [%s]}]}`,
		t.TempDir(), strings.Join(panes, ",")))
	name, _ := domain.NewProjectName("broken")

	if err := server.tmux().Start(plan); err == nil {
		t.Fatal("got no error, want a split to run out of space")
	}

	if server.running(t, name) {
		t.Fatal("got the session running, want the half-built session gone")
	}
}

func TestTmuxKill(t *testing.T) {
	server := newTmuxServer(t)
	name, _ := domain.NewProjectName("doomed")

	t.Run("reports a session that isn't running", func(t *testing.T) {
		if err := server.tmux().Kill(name); !errors.Is(err, domain.ErrSessionNotRunning) {
			t.Fatalf("got %v, want ErrSessionNotRunning", err)
		}
	})

	t.Run("stops a running session", func(t *testing.T) {
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
func layoutOf(windows []domain.Window, root string) []string {
	var out []string
	for _, w := range windows {
		var panes []string
		for _, p := range w.Panes() {
			text := p.Name().String() + "@" + strings.TrimPrefix(strings.TrimPrefix(p.Dir().String(), root), "/")
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
		if got := layoutOf(windows, root); !slices.Equal(got, want) {
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
		if got, want := layoutOf(windows, root)[0], "pane 1@ | pane 2@app right of #1 25%"; !strings.HasSuffix(got, want) {
			t.Fatalf("got %q, want it to end with %q", got, want)
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
