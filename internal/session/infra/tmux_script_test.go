package infra

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestLaunchHook(t *testing.T) {
	t.Run("hooks the shells it knows, by their program's name", func(t *testing.T) {
		for shell, want := range map[string]hookedShell{
			"/bin/bash": bashHook, "/usr/bin/zsh": zshHook, "/opt/homebrew/bin/fish": fishHook,
			"/bin/sh": shHook, "/usr/bin/dash": shHook, "/usr/bin/nu": unhooked,
		} {
			if got := (launch{shell: shell}).hook(); got != want {
				t.Errorf("%s: got %v, want %v", shell, got, want)
			}
		}
	})

	t.Run("hooks nothing when a default-command starts the panes", func(t *testing.T) {
		if got := (launch{shell: "/bin/bash", command: "exec bash"}).hook(); got != unhooked {
			t.Fatalf("got %v, want unhooked", got)
		}
	})
}

// cleared is a pane opening that clears, with a before command, a note
// and a command.
func cleared(t *testing.T) opening {
	t.Helper()
	before, err := domain.NewCommand("export X=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return opening{before: []domain.Command{before}, clears: true, note: []string{"it's ready"}, last: "npm run dev", submit: true}
}

func TestLaunchScript(t *testing.T) {
	t.Run("ends with the shell's start, which capture recognises, whatever the shell", func(t *testing.T) {
		for _, shell := range []string{"/bin/bash", "/bin/zsh", "/bin/fish", "/bin/sh"} {
			script := launch{shell: shell, tmux: "/usr/bin/tmux", logs: "/logs/p"}.script(cleared(t), "/logs/p/w.1.log")
			if !wrapped(script[1:]) {
				t.Errorf("%s: the script doesn't end with the shell's start:\n%s", shell, script[3])
			}
		}
	})

	t.Run("is a valid sh script, whatever the shell", func(t *testing.T) {
		for _, shell := range []string{"/bin/bash", "/bin/zsh", "/bin/fish", "/bin/sh"} {
			script := launch{shell: shell, tmux: "/usr/bin/tmux"}.script(cleared(t), "/logs/p/w.1.log")
			if out, err := exec.Command("sh", "-n", "-c", script[3]).CombinedOutput(); err != nil {
				t.Errorf("%s: %v: %s", shell, err, out)
			}
		}
	})

	t.Run("hooks into bash with a valid rc file", func(t *testing.T) {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skip("bash is missing")
		}
		hook := launch{shell: "/bin/bash", tmux: "/usr/bin/tmux"}.posixHook(cleared(t), "/logs/p/w.1.log", "")
		if out, err := exec.Command("bash", "-n", "-c", bashLogin+hook).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	})

	t.Run("types nothing into a hooked pane", func(t *testing.T) {
		if typed := (launch{shell: "/bin/bash"}).typed(cleared(t), "%1", ""); len(typed) != 0 {
			t.Fatalf("got %v typed, want nothing", typed)
		}
	})

	t.Run("types the before commands, then closes the log as it clears, for a shell it can't hook", func(t *testing.T) {
		typed := launch{shell: "/usr/bin/nu", tmux: "/usr/bin/tmux"}.typed(cleared(t), "%1", "/logs/p/w.1.log")
		var texts []string
		for _, line := range typed {
			texts = append(texts, line.text)
		}
		want := []string{"export X=1", " '/usr/bin/tmux' pipe-pane -t %1; clear; printf '%s\\n' 'it'\\''s ready'", "npm run dev"}
		if strings.Join(texts, "\n") != strings.Join(want, "\n") {
			t.Fatalf("got %q, want %q", texts, want)
		}
	})
}
