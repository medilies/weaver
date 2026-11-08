//go:build linux

package infra_test

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/medilies/weaver/internal/session/infra"
)

// newTerminal opens a pseudo-terminal and returns its terminal side,
// whose size a new pty leaves at 0x0 until something sets it.
func newTerminal(t *testing.T) *os.File {
	t.Helper()
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("a pseudo-terminal is required in CI: %v", err)
		}
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })
	fd := int(ptmx.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatalf("unlocking the pty: %v", err)
	}
	peer, err := unix.IoctlRetInt(fd, unix.TIOCGPTPEER)
	if err != nil {
		t.Fatalf("opening the terminal: %v", err)
	}
	tty := os.NewFile(uintptr(peer), "pty")
	t.Cleanup(func() { _ = tty.Close() })
	return tty
}

// ofSize sets tty's size and returns it.
func ofSize(t *testing.T, tty *os.File, cols, rows uint16) *os.File {
	t.Helper()
	if err := unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows}); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	return tty
}

// withStdio makes stdout, stdin and stderr the given files for the test.
func withStdio(t *testing.T, stdout, stdin, stderr *os.File) {
	t.Helper()
	was := [3]*os.File{os.Stdout, os.Stdin, os.Stderr}
	os.Stdout, os.Stdin, os.Stderr = stdout, stdin, stderr
	t.Cleanup(func() { os.Stdout, os.Stdin, os.Stderr = was[0], was[1], was[2] })
}

func TestTerminalSize(t *testing.T) {
	t.Run("reports no size for a terminal whose size was never set", func(t *testing.T) {
		tty := newTerminal(t)
		withStdio(t, tty, tty, tty)

		if cols, rows, ok := infra.TerminalSize(); ok {
			t.Fatalf("got %dx%d, want no size", cols, rows)
		}
	})

	t.Run("passes over a 0x0 terminal to the next one", func(t *testing.T) {
		withStdio(t, newTerminal(t), ofSize(t, newTerminal(t), 120, 40), newTerminal(t))

		if cols, rows, ok := infra.TerminalSize(); !ok || cols != 120 || rows != 40 {
			t.Fatalf("got %dx%d (%v), want 120x40", cols, rows, ok)
		}
	})
}
