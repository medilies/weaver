package infra

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// TerminalSize is the size of the terminal weaver runs in, read from the
// first of stdout, stdin and stderr that is one. Inside tmux, that is the
// current pane, as the shell scripts' `tput cols` and `tput lines` read.
// A terminal whose size was never set, as under script(1), reports 0x0,
// which tmux refuses: none is reported then.
func TerminalSize() (cols, rows int, ok bool) {
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		fd := int(f.Fd())
		if !term.IsTerminal(fd) {
			continue
		}
		if cols, rows, err := term.GetSize(fd); err == nil && cols > 0 && rows > 0 {
			return cols, rows, true
		}
	}
	return 0, 0, false
}

// LineAcknowledger is app.Acknowledger in a plain terminal: it prints a
// message and waits for Enter.
type LineAcknowledger struct {
	in  io.Reader
	out io.Writer
}

// NewLineAcknowledger reads Enter from in and prints to out.
func NewLineAcknowledger(in io.Reader, out io.Writer) LineAcknowledger {
	return LineAcknowledger{in: in, out: out}
}

// Acknowledge implements app.Acknowledger. Without a line to read, as
// when in is closed, it returns at once.
func (a LineAcknowledger) Acknowledge(message string) error {
	if _, err := fmt.Fprintf(a.out, "%s\n\nPress Enter to go on. ", message); err != nil {
		return err
	}
	_, err := bufio.NewReader(a.in).ReadString('\n')
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
