package infra

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"unicode"

	"github.com/medilies/weaver/internal/session/domain"
)

// failureLines is how many of a failed command's last error lines its
// error repeats, for the browser, whose redraw hides the terminal.
const failureLines = 3

// ShellRunner runs commands with sh, in the terminal weave runs in.
type ShellRunner struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

// NewShellRunner constructs a ShellRunner reading from stdin and printing
// to stdout and stderr.
func NewShellRunner(stdin io.Reader, stdout, stderr io.Writer) ShellRunner {
	return ShellRunner{stdin: stdin, stdout: stdout, stderr: stderr}
}

// Run implements domain.CommandRunner with sh -c, so each command is a
// shell of its own: a cd or export doesn't reach the next. Ctrl-C stops
// the command, not weave, which reports it as failed.
func (r ShellRunner) Run(cmd domain.Command, dir string, env domain.Env) error {
	var tail lastLines
	run := exec.Command("sh", "-c", cmd.String())
	run.Dir = dir
	run.Env = os.Environ()
	for _, v := range env.Vars() {
		run.Env = append(run.Env, v.Name()+"="+v.Value())
	}
	run.Stdin, run.Stdout, run.Stderr = r.stdin, r.stdout, io.MultiWriter(r.stderr, &tail)
	// The terminal sends Ctrl-C to the command and to weave: weave waits
	// for the command to end instead of dying with it.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	if err := run.Run(); err != nil {
		if lines := tail.last(failureLines); len(lines) > 0 {
			return fmt.Errorf("%w (%v): %s", domain.ErrCommandFailed, err, strings.Join(lines, " / "))
		}
		return fmt.Errorf("%w (%v)", domain.ErrCommandFailed, err)
	}
	return nil
}

// lastLines keeps the end of what is written to it.
type lastLines struct {
	kept []byte
}

// lastLinesKept bounds what lastLines keeps, from the end.
const lastLinesKept = 4096

func (l *lastLines) Write(p []byte) (int, error) {
	l.kept = append(l.kept, p...)
	if extra := len(l.kept) - lastLinesKept; extra > 0 {
		l.kept = l.kept[extra:]
	}
	return len(p), nil
}

// escapes are terminal escape sequences, as colours, kept out of errors.
var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// last is up to n of the last non-blank lines written, trimmed, in order,
// as the terminal last showed them: without colours, and a line redrawn
// after a carriage return as redrawn.
func (l *lastLines) last(n int) []string {
	var lines []string
	for line := range strings.SplitSeq(escapes.ReplaceAllString(string(l.kept), ""), "\n") {
		if i := strings.LastIndex(strings.TrimRight(line, "\r"), "\r"); i >= 0 {
			line = line[i+1:]
		}
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, line)
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines[max(len(lines)-n, 0):]
}
