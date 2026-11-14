package infra

import (
	"errors"
	"os/exec"
	"testing"
)

func TestLost(t *testing.T) {
	exited := func(t *testing.T) error {
		t.Helper()
		err := exec.Command("false").Run()
		if err == nil {
			t.Fatal("false succeeded")
		}
		return err
	}

	t.Run("finds a pane closed meanwhile", func(t *testing.T) {
		if !lost(&tmuxError{command: "capture-pane", err: exited(t), stderr: "can't find pane: %9"}) {
			t.Fatal("want a pane tmux can't find lost")
		}
	})

	t.Run("finds a window closed meanwhile", func(t *testing.T) {
		if !lost(&tmuxError{command: "list-panes", err: exited(t), stderr: "can't find window: @4"}) {
			t.Fatal("want a window tmux can't find lost")
		}
	})

	t.Run("doesn't take a missing session for a closed pane", func(t *testing.T) {
		if lost(&tmuxError{command: "list-panes", err: exited(t), stderr: "can't find session: shop"}) {
			t.Fatal("want a missing session left to absent")
		}
	})

	t.Run("doesn't take tmux failing to run for a closed pane", func(t *testing.T) {
		if lost(&tmuxError{command: "capture-pane", err: errors.New("exec: not found"), stderr: "can't find pane: %9"}) {
			t.Fatal("want only tmux's own answer read")
		}
	})
}
