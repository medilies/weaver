package infra

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestLost(t *testing.T) {
	t.Run("finds a pane closed meanwhile", func(t *testing.T) {
		if !lost(&tmuxError{command: "capture-pane", err: exitedErr(t), stderr: "can't find pane: %9"}) {
			t.Fatal("want a pane tmux can't find lost")
		}
	})

	t.Run("finds a window closed meanwhile", func(t *testing.T) {
		if !lost(&tmuxError{command: "list-panes", err: exitedErr(t), stderr: "can't find window: @4"}) {
			t.Fatal("want a window tmux can't find lost")
		}
	})

	t.Run("doesn't take a missing session for a closed pane", func(t *testing.T) {
		if lost(&tmuxError{command: "list-panes", err: exitedErr(t), stderr: "can't find session: shop"}) {
			t.Fatal("want a missing session left to absent")
		}
	})

	t.Run("doesn't take tmux failing to run for a closed pane", func(t *testing.T) {
		if lost(&tmuxError{command: "capture-pane", err: errors.New("exec: not found"), stderr: "can't find pane: %9"}) {
			t.Fatal("want only tmux's own answer read")
		}
	})
}

func TestReadAgainIfLost(t *testing.T) {
	window, err := domain.NewWindowName("api")
	if err != nil {
		t.Fatal(err)
	}
	pane := domain.NewPanePreview(0, 0, 80, 24, "", nil)
	preview, err := domain.NewWindowPreview(80, 24, pane)
	if err != nil {
		t.Fatal(err)
	}
	gone := &tmuxError{command: "capture-pane", err: exitedErr(t), stderr: "can't find pane: %9"}
	// reads answers with errs in turn, then preview.
	reads := func(errs ...error) (func() (domain.WindowPreview, error), *int) {
		n := 0
		return func() (domain.WindowPreview, error) {
			n++
			if n <= len(errs) {
				return domain.WindowPreview{}, errs[n-1]
			}
			return preview, nil
		}, &n
	}

	t.Run("reads the window again when a pane closed while it was read", func(t *testing.T) {
		read, n := reads(gone)
		got, err := readAgainIfLost(window, read)
		if err != nil || len(got.Panes()) != 1 || *n != 2 {
			t.Fatalf("got %v after %d reads, want the second read's preview", err, *n)
		}
	})

	t.Run("says the window closed when it is lost twice", func(t *testing.T) {
		read, _ := reads(gone, gone)
		if _, err := readAgainIfLost(window, read); !errors.Is(err, domain.ErrWindowNotRunning) {
			t.Fatalf("got %v, want ErrWindowNotRunning", err)
		}
	})

	t.Run("reads once what fails otherwise, keeping the error", func(t *testing.T) {
		other := errors.New("tmux gone")
		read, n := reads(other)
		if _, err := readAgainIfLost(window, read); !errors.Is(err, other) || *n != 1 {
			t.Fatalf("got %v after %d reads, want the error of a single read", err, *n)
		}
	})

	t.Run("reads once what doesn't fail", func(t *testing.T) {
		read, n := reads()
		if _, err := readAgainIfLost(window, read); err != nil || *n != 1 {
			t.Fatalf("got %v after %d reads, want one read", err, *n)
		}
	})
}

// exitedErr is the error of a command that ran and failed.
func exitedErr(t *testing.T) error {
	t.Helper()
	err := exec.Command("false").Run()
	if err == nil {
		t.Fatal("false succeeded")
	}
	return err
}
