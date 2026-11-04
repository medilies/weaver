package infra_test

import (
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

// stubRuntimes is a fake domain.RuntimeResolver: "kind version" → dir.
type stubRuntimes map[string]string

func (s stubRuntimes) Resolve(runtime domain.Runtime) (string, error) {
	if dir, ok := s[runtime.String()]; ok {
		return dir, nil
	}
	return "", fmt.Errorf("%w: %s", domain.ErrRuntimeNotInstalled, runtime)
}

// planOf loads doc as project name and resolves it for tmux.
func planOf(t *testing.T, name, doc string, runtimes stubRuntimes) domain.SessionPlan {
	t.Helper()
	project, err := load(t, name, doc)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	plan, err := project.Plan(runtimes)
	if err != nil {
		t.Fatalf("planning %s: %v", name, err)
	}
	return plan
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
	probes := t.TempDir()
	if err := os.WriteFile(filepath.Join(probes, "weaver-probe"), []byte("#!/bin/sh\necho probe-ok\n"), 0o755); err != nil {
		t.Fatalf("writing probe: %v", err)
	}
	plan := planOf(t, "start", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "api", "panes": [
			{"name": "shell"},
			{"name": "side", "dir": "sub", "split": {"direction": "right", "size": "30%%"}, "command": "echo w-$((40+2))"},
			{"name": "below", "split": {"direction": "below"}, "prefill": "echo p-$((40+2))"}
		]},
		{"name": "env", "panes": [
			{"note": ["it's a note"], "env": {"GREETING": "hi $HOME"}, "runtimes": {"node": "20"},
			 "command": "echo \"[$GREETING]\" && weaver-probe"}
		]},
		{"name": "repo"}
	]}`, root), stubRuntimes{"node 20": probes})

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

	t.Run("puts the pane's runtimes first on PATH", func(t *testing.T) {
		server.waitFor(t, "=start:env", "probe-ok")
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
	]}`, root), nil)

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
		t.TempDir(), strings.Join(panes, ",")), nil)
	name, _ := domain.NewProjectName("broken")

	if err := server.tmux().Start(plan); err == nil {
		t.Fatal("got no error, want a split to run out of space")
	}

	if running, err := server.tmux().IsRunning(name); err != nil || running {
		t.Fatalf("got running=%v (%v), want the half-built session gone", running, err)
	}
}

func TestTmuxIsRunning(t *testing.T) {
	server := newTmuxServer(t)
	shop, _ := domain.NewProjectName("shop")

	t.Run("reports no session when no server runs", func(t *testing.T) {
		if running, err := server.tmux().IsRunning(shop); err != nil || running {
			t.Fatalf("got running=%v (%v), want not running", running, err)
		}
	})

	if err := server.tmux().Start(planOf(t, "shop-api", fmt.Sprintf(`{"root": %q}`, t.TempDir()), nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("reports a session it started", func(t *testing.T) {
		name, _ := domain.NewProjectName("shop-api")
		if running, err := server.tmux().IsRunning(name); err != nil || !running {
			t.Fatalf("got running=%v (%v), want running", running, err)
		}
	})

	t.Run("matches session names exactly, not by prefix", func(t *testing.T) {
		if running, err := server.tmux().IsRunning(shop); err != nil || running {
			t.Fatalf("got running=%v (%v), want shop not running", running, err)
		}
	})
}
