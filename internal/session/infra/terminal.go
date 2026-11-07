package infra

import (
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
