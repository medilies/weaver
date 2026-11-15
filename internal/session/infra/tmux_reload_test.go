package infra_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

func TestTmuxReload(t *testing.T) {
	server := newTmuxServer(t)
	plan := planOf(t, "reload", fmt.Sprintf(`{"root": %q, "windows": [
		{"name": "first"},
		{"name": "api", "panes": [
			{"name": "shell", "tasks": [{"command": "sleep 300"}]},
			{"name": "back", "split": {"direction": "right", "size": "30%%"}, "env": {"GREETING": "hello"}, "command": "echo $GREETING-again"}
		]},
		{"name": "last"}
	]}`, t.TempDir()))
	if err := server.tmux().Start(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	project, api := plan.Name(), plan.Windows()[1]
	server.waitFor(t, server.paneLabelled(t, "reload", "back"), "hello-again")
	windows := func() string {
		return server.query(t, "list-windows", "-t", "=reload", "-F", "#{window_index}:#{window_name}")
	}

	t.Run("restarts a pane in place, as opening starts it", func(t *testing.T) {
		back := server.paneLabelled(t, "reload", "back")
		before := server.query(t, "display-message", "-p", "-t", back, "#{pane_pid} #{pane_width} #{pane_left}")
		server.query(t, "send-keys", "-t", back, "clear", "Enter")
		if err := server.tmux().ReloadPane(project, api, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server.waitFor(t, back, "hello-again")
		after := server.query(t, "display-message", "-p", "-t", back, "#{pane_pid} #{pane_width} #{pane_left}")
		pid, place, _ := strings.Cut(before, " ")
		newPid, newPlace, _ := strings.Cut(after, " ")
		if pid == newPid || place != newPlace {
			t.Fatalf("pane was %q, now %q; want a new process in the same place", before, after)
		}
	})

	t.Run("replaces a window where it is, the others staying, its task's throwaway windows tied to its new panes", func(t *testing.T) {
		server.query(t, "select-window", "-t", "=reload:api")
		if err := server.tmux().RunApart(project, api, 0, api.Panes()[0].Tasks()[0], false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		before := windows()
		old := server.query(t, "display-message", "-p", "-t", "=reload:api", "#{window_id}")
		if err := server.tmux().ReloadWindow(project, api); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if after := windows(); after != before {
			t.Fatalf("windows were %q, now %q", before, after)
		}
		now := server.query(t, "display-message", "-p", "-t", "=reload:", "#{window_id} #{window_name}")
		if now == old+" api" || !strings.HasSuffix(now, " api") {
			t.Fatalf("the current window is %q, want a new api", now)
		}
		server.waitFor(t, server.paneLabelled(t, "reload", "back"), "hello-again")
		shell := server.paneLabelled(t, "reload", "shell")
		if tie := server.query(t, "display-message", "-p", "-t", "=reload:sleep", "#{@weaver_task_of}"); tie != shell {
			t.Fatalf("the throwaway window is tied to %q, want the new shell %q", tie, shell)
		}
	})

	t.Run("leaves the current window current when reloading another", func(t *testing.T) {
		server.query(t, "select-window", "-t", "=reload:first")
		if err := server.tmux().ReloadWindow(project, api); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if current := server.query(t, "display-message", "-p", "-t", "=reload:", "#{window_name}"); current != "first" {
			t.Fatalf("the current window is %q, want first", current)
		}
	})

	t.Run("leaves the session's last window as it was", func(t *testing.T) {
		server.query(t, "select-window", "-t", "=reload:last")
		server.query(t, "select-window", "-t", "=reload:first")
		if err := server.tmux().ReloadWindow(project, api); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if last := server.query(t, "display-message", "-p", "-t", "=reload:last", "#{window_last_flag}"); last != "1" {
			t.Fatalf("last's last-window flag is %q, want it still the last window", last)
		}
	})

	t.Run("restarts a pane without a command or env as a plain shell", func(t *testing.T) {
		first := plan.Windows()[0]
		pane := server.query(t, "display-message", "-p", "-t", "=reload:first", "#{pane_id}")
		if err := server.tmux().ReloadPane(project, first, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		server.waitFor(t, pane, "$")
	})

	t.Run("refuses the window weaver runs in", func(t *testing.T) {
		inside := infra.NewTmux(infra.TmuxOptions{Global: server.global, Env: server.env, Inside: true, Pane: server.paneLabelled(t, "reload", "back")})
		if err := inside.ReloadWindow(project, api); !errors.Is(err, domain.ErrPaneRunsWeaver) {
			t.Fatalf("got %v, want ErrPaneRunsWeaver", err)
		}
		if err := inside.ReloadPane(project, api, 1); !errors.Is(err, domain.ErrPaneRunsWeaver) {
			t.Fatalf("got %v, want ErrPaneRunsWeaver", err)
		}
	})

	t.Run("reports a window the session lacks", func(t *testing.T) {
		server.query(t, "kill-window", "-t", "=reload:last")
		if err := server.tmux().ReloadWindow(project, plan.Windows()[2]); !errors.Is(err, domain.ErrWindowNotRunning) {
			t.Fatalf("got %v, want ErrWindowNotRunning", err)
		}
	})
}
